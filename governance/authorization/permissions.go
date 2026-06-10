// Package runtime enforces agent permission contracts at execution time.
// PermissionManager authorises tool calls, file access, executable invocations, and
// network requests against permissions declared in the agent manifest, applying a
// three-level policy (Allow / Ask / Deny) with Human-in-the-Loop approval flows
// and configurable policy.GrantScope (OneTime, Session, Persistent).
package authorization

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

const permissionMatchAll = "**"

// AgentFilePermissionSet stores glob allow/deny rules.
type AgentFilePermissionSet struct {
	AllowPatterns     []string
	DenyPatterns      []string
	Default           string
	RequireApproval   bool
	DocumentationOnly bool
}

// AgentFileMatrix scopes write/edit operations.
type AgentFileMatrix struct {
	Write AgentFilePermissionSet
	Edit  AgentFilePermissionSet
}

// ToolPermissions describes the permissions a tool requires.
type ToolPermissions struct {
	Permissions *permissions.PermissionSet
}

func (t ToolPermissions) Validate() error {
	if t.Permissions == nil {
		return errors.New("tool permissions missing")
	}
	return t.Permissions.Validate()
}

// Tool is the governance-owned view of a capability tool.
type Tool interface {
	Name() string
	Permissions() ToolPermissions
	Tags() []string
}

// globRegexCache caches compiled glob-to-regex patterns using a bounded LRU.
// The process-global sync.Map was replaced to prevent memory exhaustion from
// adversarial or deeply-nested glob patterns.
var globRegexCache = newCompiledGlobCache(256)

// PermissionManager enforces the declared permission set for runtime actions.
type PermissionManager struct {
	basePath         string
	declared         *permissions.PermissionSet
	audit            policy.AuditLogger
	hitl             HITLProvider
	runtime          governanceports.SandboxRuntime
	grants           map[string]*PermissionGrant
	mu               sync.RWMutex
	grantClock       func() time.Time
	netPolicy        []governanceports.SandboxNetworkRule
	defaultPolicy    string // governs undeclared tool permissions; default is Ask
	eventLogger      func(context.Context, permissions.PermissionDescriptor, string, string, map[string]interface{})
	runtimePolicyErr error
	taskGrants       map[string]taskGrant
	hitlRateLimits   map[string]*hitlRateBucket
	fsPermCache      map[string]*permissions.FileSystemPermission
	execPermCache    map[string]*permissions.ExecutablePermission
	fsProtectedRoots []string
	fsExcludedRoots  []string
}

// NewPermissionManager creates an enforcement instance.
func NewPermissionManager(basePath string, declared *permissions.PermissionSet, audit policy.AuditLogger, hitl HITLProvider) (*PermissionManager, error) {
	if declared == nil {
		return nil, errors.New("permission manager requires permission set")
	}
	if err := declared.Validate(); err != nil {
		return nil, err
	}
	pm := &PermissionManager{
		basePath:       basePath,
		declared:       declared,
		audit:          audit,
		hitl:           hitl,
		grants:         make(map[string]*PermissionGrant),
		taskGrants:     make(map[string]taskGrant),
		hitlRateLimits: make(map[string]*hitlRateBucket),
		fsPermCache:    make(map[string]*permissions.FileSystemPermission),
		execPermCache:  make(map[string]*permissions.ExecutablePermission),
		grantClock:     time.Now,
	}
	pm.inflateScopes()
	return pm, nil
}

// AttachRuntime allows the manager to push policy updates to the sandbox.
func (m *PermissionManager) AttachRuntime(runtime governanceports.SandboxRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime = runtime
	m.applyRuntimePolicyLocked()
}

// SetDefaultPolicy configures how undeclared permissions are handled.
// agentspec.AgentPermissionAsk (default) routes to HITL; Allow bypasses; Deny hard-blocks.
func (m *PermissionManager) SetDefaultPolicy(level string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultPolicy = level
}

// SetEventLogger configures a callback for structured policy decision events.
func (m *PermissionManager) SetEventLogger(logger func(context.Context, permissions.PermissionDescriptor, string, string, map[string]interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventLogger = logger
}

// DefaultPolicy returns the configured default policy level, falling back to Ask.
func (m *PermissionManager) DefaultPolicy() string {
	return m.effectiveDefaultPolicy()
}

// effectiveDefaultPolicy returns the configured policy, falling back to Ask.
func (m *PermissionManager) effectiveDefaultPolicy() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultPolicy == "" {
		return "ask"
	}
	return m.defaultPolicy
}

func (m *PermissionManager) applyRuntimePolicyLocked() {
	if m == nil || m.runtime == nil {
		return
	}
	policy := m.currentSandboxPolicyLocked()
	m.runtimePolicyErr = m.runtime.ApplyPolicy(context.Background(), policy)
}

// Policy returns the merged sandbox policy currently known to the
// permission manager. Callers get a copy and can inspect it without racing.
func (m *PermissionManager) Policy() governanceports.SandboxPolicy {
	if m == nil {
		return governanceports.SandboxPolicy{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentSandboxPolicyLocked()
}

// RuntimePolicyError returns the last sandbox sync error, if any.
func (m *PermissionManager) RuntimePolicyError() error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtimePolicyErr
}

func (m *PermissionManager) currentSandboxPolicyLocked() governanceports.SandboxPolicy {
	if m == nil {
		return governanceports.SandboxPolicy{}
	}
	policy := governanceports.SandboxPolicy{}
	if m.runtime != nil {
		policy = m.runtime.Policy()
	}
	policy.NetworkRules = append([]governanceports.SandboxNetworkRule(nil), m.netPolicy...)
	return policy
}

// matchGlob supports both filepath.Match and the '**' recursive glob pattern
// so manifests can succinctly describe directories.
func matchGlob(pattern, value string) bool {
	if pattern == permissionMatchAll {
		return true
	}
	pattern = filepath.ToSlash(pattern)
	value = filepath.ToSlash(value)
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		if value == base {
			return true
		}
	}
	if !strings.Contains(pattern, "**") {
		ok, err := filepath.Match(pattern, value)
		if err != nil {
			return false
		}
		return ok
	}
	regexPattern := globToRegex(pattern)
	regex, err := globRegexCache.get(regexPattern)
	if err != nil {
		return false
	}
	return regex.MatchString(value)
}

// globToRegex converts '**' style globs into Go regular expressions so we can
// cheaply support recursive directory matching.
func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		switch ch {
		case '*':
			peek := ""
			if i+1 < len(runes) {
				peek = string(runes[i+1])
			}
			if peek == "*" {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '|', '^', '$', '[', ']', '{', '}', '\\':
			b.WriteRune('\\')
			b.WriteRune(ch)
		default:
			b.WriteRune(ch)
		}
	}
	b.WriteString("$")
	return b.String()
}

// PermissionRequirement declares a permission needed by a tool or plugin.
type PermissionRequirement struct {
	Type     permissions.PermissionType
	Action   string
	Resource string
}

// HITLProvider handles human approvals.
type HITLProvider interface {
	RequestPermission(ctx context.Context, req PermissionRequest) (*PermissionGrant, error)
}

// PermissionGrant captures approval metadata.
type PermissionGrant struct {
	ID          string
	Permission  permissions.PermissionDescriptor
	Scope       policy.GrantScope
	ExpiresAt   time.Time
	ApprovedBy  string
	Conditions  map[string]string
	GrantedAt   time.Time
	Description string
}

// Expired returns true when the grant is not usable anymore.
func (g *PermissionGrant) Expired(now time.Time) bool {
	if g == nil {
		return true
	}
	if g.ExpiresAt.IsZero() {
		return false
	}
	return now.After(g.ExpiresAt)
}

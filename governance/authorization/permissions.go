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

	"codeburg.org/lexbit/relurpify/governance/bounded"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
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
var globRegexCache = newCompiledGlobCache(256) //nolint:gochecknoglobals // bounded regex cache guarded by a mutex

// Cache caps for the permission manager's bounded maps. Every cap has an
// eviction counter on its cache; evictions are policy-forget events, not
// errors — the next evaluation recomputes and re-caches.
const (
	grantsCacheCap   = 1024
	hitlRateCacheCap = 4096
	fsPermCacheCap   = 2048
	execPermCacheCap = 2048
	hitlRateEntryTTL = time.Hour
)

// PermissionManager enforces the declared permission set for runtime actions.
type PermissionManager struct {
	basePath         string
	declared         *permissions.PermissionSet
	audit            policy.AuditLogger
	hitl             HITLProvider
	runtime          governanceports.SandboxRuntime
	grants           *bounded.Cache[string, *PermissionGrant]
	mu               sync.RWMutex
	grantClock       func() time.Time
	netPolicy        []governanceports.SandboxNetworkRule
	defaultDecision  permissions.Decision // governs undeclared tool permissions; default is Ask
	decisions        fwtelemetry.DecisionSink
	runtimePolicyErr error
	// taskGrants is bounded by its lifecycle: entries are deleted on
	// RevokeTaskGrant and never outlive their run.
	taskGrants       map[string]taskGrant
	hitlRateLimits   *bounded.Cache[string, *hitlRateBucket]
	fsPermCache      *bounded.Cache[string, *permissions.FileSystemPermission]
	execPermCache    *bounded.Cache[string, *permissions.ExecutablePermission]
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
		basePath:        basePath,
		declared:        declared,
		audit:           audit,
		hitl:            hitl,
		grants:          bounded.NewCache[string, *PermissionGrant](grantsCacheCap, 0, nil),
		taskGrants:      make(map[string]taskGrant),
		hitlRateLimits:  bounded.NewCache[string, *hitlRateBucket](hitlRateCacheCap, hitlRateEntryTTL, nil),
		fsPermCache:     bounded.NewCache[string, *permissions.FileSystemPermission](fsPermCacheCap, 0, nil),
		execPermCache:   bounded.NewCache[string, *permissions.ExecutablePermission](execPermCacheCap, 0, nil),
		grantClock:      time.Now,
		defaultDecision: permissions.DecisionAsk,
	}
	pm.inflateScopes()
	return pm, nil
}

// putGrant stores a grant, sweeping grant-level expiries once the registry
// passes half its cap so a long-lived manager never accumulates dead grants.
func (m *PermissionManager) putGrant(key string, grant *PermissionGrant) {
	if m.grants.Len() >= grantsCacheCap/2 {
		m.sweepExpiredGrants()
	}
	m.grants.Put(key, grant)
}

// sweepExpiredGrants deletes every grant whose own ExpiresAt has elapsed.
func (m *PermissionManager) sweepExpiredGrants() {
	now := m.grantClock()
	var stale []string
	m.grants.Range(func(key string, grant *PermissionGrant) bool {
		if grant.Expired(now) {
			stale = append(stale, key)
		}
		return true
	})
	for _, key := range stale {
		m.grants.Delete(key)
	}
}

// ReleaseSession deletes every grant issued under sessionID (typically the
// agent registration ID carried by the approving principal). Idempotent;
// returns the number of grants released. Session-scoped grants must not
// survive the session's close.
func (m *PermissionManager) ReleaseSession(sessionID string) int {
	if m == nil {
		return 0
	}
	var released []string
	m.grants.Range(func(key string, grant *PermissionGrant) bool {
		if grant.SessionID != "" && grant.SessionID == sessionID {
			released = append(released, key)
		}
		return true
	})
	for _, key := range released {
		m.grants.Delete(key)
	}
	return len(released)
}

// AttachRuntime allows the manager to push policy updates to the sandbox.
func (m *PermissionManager) AttachRuntime(ctx context.Context, runtime governanceports.SandboxRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime = runtime
	m.applyRuntimePolicyLocked(ctx)
}

// SetDefaultDecision configures how undeclared permissions are handled.
// DecisionAllow is rejected here (compile-time enforcement of the posture);
// DecisionDeny hard-blocks; the terminal default is DecisionAsk (HITL).
func (m *PermissionManager) SetDefaultDecision(level permissions.Decision) error {
	if _, err := permissions.ParseDecision(string(level)); err != nil {
		return err
	}
	if level == permissions.DecisionAllow {
		return errors.New("default decision allow is not permitted; use ask (HITL) or deny")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultDecision = level
	return nil
}

// SetDecisionSink configures the port that receives structured decision
// forensics for every policy evaluation (Decision 6, FR-5).
func (m *PermissionManager) SetDecisionSink(sink fwtelemetry.DecisionSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisions = sink
}

// DefaultPolicy returns the configured default decision as a string, falling
// back to ask. It is a projection for consumers that persist a string.
func (m *PermissionManager) DefaultPolicy() string {
	return string(m.effectiveDefaultDecision())
}

// effectiveDefaultDecision returns the configured decision, falling back to ask.
func (m *PermissionManager) effectiveDefaultDecision() permissions.Decision {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultDecision == "" {
		return permissions.DecisionAsk
	}
	return m.defaultDecision
}

func (m *PermissionManager) applyRuntimePolicyLocked(ctx context.Context) {
	if m == nil || m.runtime == nil {
		return
	}
	policy := m.currentSandboxPolicyLocked()
	m.runtimePolicyErr = m.runtime.ApplyPolicy(ctx, policy)
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
	if pattern == "" {
		return false
	}
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

// globToRegex converts glob patterns into Go regular expressions, supporting
// standard filepath.Match syntax plus '**' (doublestar) for recursive matching.
// '**/' matches zero or more directory levels; '**' alone matches everything.
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
				if i+2 < len(runes) && runes[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
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
	// SessionID identifies the approving session (the principal's agent
	// registration). Empty for grants issued outside a session context.
	// ReleaseSession deletes grants by this key.
	SessionID string
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

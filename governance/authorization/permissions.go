// Package runtime enforces agent permission contracts at execution time.
// PermissionManager authorises tool calls, file access, executable invocations, and
// network requests against permissions declared in the agent manifest, applying a
// three-level policy (Allow / Ask / Deny) with Human-in-the-Loop approval flows
// and configurable policy.GrantScope (OneTime, Session, Persistent).
package authorization

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
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

// governanceports.SandboxConfig exposes runtime knobs for a sandbox backend.
// isPrivateOrLoopbackHost checks if the host is a private/loopback address.
func isPrivateOrLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	if ip != nil {
		return isPrivateIP(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	return false
}

// hitlRateMax is the maximum HITL requests per key within hitlRateWindow before
// subsequent requests are rejected with a rate-limit error.
const hitlRateMax = 10

// hitlRateWindow is the sliding window duration for HITL rate limiting.
const hitlRateWindow = time.Minute

// globRegexCache caches compiled glob-to-regex patterns using a bounded LRU.
// The process-global sync.Map was replaced to prevent memory exhaustion from
// adversarial or deeply-nested glob patterns.
var globRegexCache = newCompiledGlobCache(256)

// hitlRateBucket tracks the number of HITL requests within a rolling time window.
type hitlRateBucket struct {
	count    int
	windowAt time.Time
}

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

type taskGrant struct {
	runID        string
	approvedTags map[string]struct{}
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

// SetFilesystemGuardRoots configures filesystem roots that must never be
// matched by manifest permissions, plus roots that are excluded from the
// default workspace glob unless explicitly declared.
func (m *PermissionManager) SetFilesystemGuardRoots(protectedRoots, excludedRoots []string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fsProtectedRoots = normalizeFilesystemRoots(protectedRoots)
	m.fsExcludedRoots = normalizeFilesystemRoots(excludedRoots)
	m.fsPermCache = make(map[string]*permissions.FileSystemPermission)
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

// inflateScopes rewrites any workspace placeholders inside the declared
// filesystem permissions so later matching can operate on concrete paths.
func (m *PermissionManager) inflateScopes() {
	if m == nil || m.declared == nil {
		return
	}
	ws := filepath.ToSlash(filepath.Clean(m.basePath))
	for i := range m.declared.FileSystem {
		m.declared.FileSystem[i].Path = expandWorkspacePlaceholder(ws, m.declared.FileSystem[i].Path)
	}
}

// expandWorkspacePlaceholder replaces instances of ${workspace} markers with
// the actual base path, keeping relative globs compatible with matchers.
func expandWorkspacePlaceholder(workspace, pattern string) string {
	if pattern == "" {
		return pattern
	}
	replacer := strings.NewReplacer(
		"${workspace}", workspace,
		"${WORKSPACE}", workspace,
		"{{workspace}}", workspace,
		"{{WORKSPACE}}", workspace,
	)
	resolved := filepath.ToSlash(replacer.Replace(pattern))
	if filepath.IsAbs(resolved) {
		return resolved
	}
	if workspace == "" {
		return filepath.ToSlash(resolved)
	}
	resolved = strings.TrimPrefix(resolved, "./")
	return filepath.ToSlash(filepath.Join(workspace, resolved))
}

// AuthorizeTool ensures the tool requirements fit the declared permissions.
// Undeclared permissions are handled according to the configured defaultPolicy:
// Ask (default) routes to HITL, Allow proceeds, Deny returns an error.
func (m *PermissionManager) AuthorizeTool(ctx context.Context, agentID string, tool any, args map[string]interface{}) error {
	if m == nil || tool == nil {
		return errors.New("permission manager or tool missing")
	}
	t, ok := tool.(Tool)
	if !ok {
		return errors.New("tool does not implement authorization.Tool")
	}
	if m.toolAllowedByTaskGrant(ctx, t) {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeHITL,
			Action:   fmt.Sprintf("tool:%s", t.Name()),
			Resource: agentID,
		}
		m.log(ctx, agentID, desc, "tool_allowed_task_grant", map[string]interface{}{"tags": t.Tags()})
		m.emitPolicyDecision(ctx, desc, "allow", "task grant matched tool tags", map[string]interface{}{"tags": t.Tags()})
		return nil
	}
	requirements := t.Permissions()
	if err := requirements.Validate(); err != nil {
		return fmt.Errorf("tool %s permission invalid: %w", t.Name(), err)
	}
	undeclared := m.collectUndeclared(requirements.Permissions)
	if len(undeclared) > 0 {
		switch m.effectiveDefaultPolicy() {
		case "deny":
			m.emitPolicyDecision(ctx, permissions.PermissionDescriptor{
				Type:     permissions.PermissionTypeHITL,
				Action:   fmt.Sprintf("tool:%s", t.Name()),
				Resource: agentID,
			}, "deny", "tool exceeds declared permissions", map[string]interface{}{"undeclared": undeclared})
			return fmt.Errorf("tool %s exceeds agent permissions: %s", t.Name(), strings.Join(undeclared, "; "))
		default: // "ask"
			m.emitPolicyDecision(ctx, permissions.PermissionDescriptor{
				Type:         permissions.PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", t.Name()),
				Resource:     agentID,
				RequiresHITL: true,
			}, "require_approval", "undeclared permissions require approval", map[string]interface{}{"undeclared": undeclared})
			if err := m.RequireApproval(ctx, agentID, permissions.PermissionDescriptor{
				Type:         permissions.PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", t.Name()),
				Resource:     agentID,
				RequiresHITL: true,
			}, fmt.Sprintf("tool %s requires: %s", t.Name(), strings.Join(undeclared, ", ")),
				policy.GrantScopeSession, policy.RiskLevelMedium, 0); err != nil {
				return err
			}
		}
	}
	desc := permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeHITL,
		Action:   fmt.Sprintf("tool:%s", t.Name()),
		Resource: agentID,
	}
	m.log(ctx, agentID, desc, "tool_allowed", nil)
	m.emitPolicyDecision(ctx, desc, "allow", "tool authorized", nil)
	return nil
}

func (m *PermissionManager) RegisterTaskGrant(runID string, approvedTags []string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("run id required")
	}
	if len(approvedTags) == 0 {
		return errors.New("approved tags required")
	}
	grant := taskGrant{
		runID:        runID,
		approvedTags: make(map[string]struct{}, len(approvedTags)),
	}
	for _, tag := range approvedTags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if tag == "*" {
			return errors.New("wildcard task grants are not allowed")
		}
		grant.approvedTags[tag] = struct{}{}
	}
	if len(grant.approvedTags) == 0 {
		return errors.New("approved tags required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taskGrants[runID] = grant
	return nil
}

func (m *PermissionManager) RevokeTaskGrant(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.taskGrants, strings.TrimSpace(runID))
}

// GrantPermission records a manual approval for a specific permission key.
func (m *PermissionManager) GrantPermission(desc permissions.PermissionDescriptor, approvedBy string, scope policy.GrantScope, duration time.Duration) {
	if m == nil {
		return
	}
	grant := GrantManual(desc, approvedBy, scope, duration)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grants[desc.Action+":"+desc.Resource] = grant
}

func (m *PermissionManager) toolAllowedByTaskGrant(ctx context.Context, tool Tool) bool {
	if m == nil || tool == nil {
		return false
	}
	taskCtx, ok := execution.TaskContextFrom(ctx)
	if !ok || strings.TrimSpace(taskCtx.ID) == "" {
		return false
	}
	m.mu.RLock()
	grant, ok := m.taskGrants[taskCtx.ID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	tags := tool.Tags()
	if len(tags) == 0 {
		return false
	}
	// Any approved tag on the tool is sufficient to authorise it under this grant.
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := grant.approvedTags[tag]; ok {
			return true
		}
	}
	return false
}

// CheckFileAccess validates filesystem access.
func (m *PermissionManager) CheckFileAccess(ctx context.Context, agentID string, action permissions.FileSystemAction, path string) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	clean, err := m.normalizePath(path)
	if err != nil {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeFilesystem,
			Action:   string(action),
			Resource: path,
		}, fmt.Sprintf("path escapes workspace: %v", err))
	}
	perm := m.findFilesystemPermission(action, clean)
	if perm == nil {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeFilesystem,
			Action:   string(action),
			Resource: clean,
		}
		switch m.effectiveDefaultPolicy() {
		case "deny":
			return m.deny(ctx, agentID, desc, "not declared")
		default: // AgentPermissionAsk (Allow is rejected at registration time)
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeFilesystem,
			Action:       string(action),
			Resource:     perm.Path,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeFilesystem,
		Action:   string(action),
		Resource: clean,
	}, "granted", map[string]interface{}{
		"pattern": perm.Path,
	})
	return nil
}

// CheckExecutable validates binary execution.
func (m *PermissionManager) CheckExecutable(ctx context.Context, agentID, binary string, args []string, env []string) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	perm := m.findExecutablePermission(binary)
	if perm == nil {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:binary:%s", binary),
			Resource: binary,
		}
		switch m.effectiveDefaultPolicy() {
		case "deny":
			return m.deny(ctx, agentID, desc, "binary not declared")
		default: // AgentPermissionAsk (Allow is rejected at registration time)
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if len(perm.Args) > 0 && !matchArgs(perm.Args, args) {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:args:%s", strings.Join(args, " ")),
			Resource: binary,
		}, "arguments rejected")
	}
	if len(perm.Env) > 0 && !matchEnv(perm.Env, env) {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeExecutable,
			Action:   "exec:env",
			Resource: binary,
		}, "environment rejected")
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeExecutable,
			Action:       fmt.Sprintf("exec:binary:%s", binary),
			Resource:     binary,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeExecutable,
		Action:   fmt.Sprintf("exec:%s", binary),
		Resource: binary,
	}, "granted", map[string]interface{}{
		"args": args,
		"env":  env,
	})
	return nil
}

// CheckNetwork validates network access.
func (m *PermissionManager) CheckNetwork(ctx context.Context, agentID string, direction string, protocol string, host string, port int) error {
	// Hard mandatory block: private, loopback, and link-local IPs are never
	// reachable regardless of agent configuration. This prevents SSRF to
	// cloud metadata services, localhost services, and internal networks.
	// The denylist is owned and enforced by the sandbox package.
	if isPrivateOrLoopbackHost(host) {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeNetwork,
			Action:   fmt.Sprintf("net:%s:%s:%s:%d", direction, protocol, host, port),
			Resource: host,
		}, "private, loopback, or link-local addresses are blocked")
	}
	perm := m.findNetworkPermission(direction, protocol, host, port)
	if perm == nil {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeNetwork,
			Action:   fmt.Sprintf("net:%s:%s:%s:%d", direction, protocol, host, port),
			Resource: host,
		}
		switch m.effectiveDefaultPolicy() {
		case "deny":
			return m.deny(ctx, agentID, desc, "network scope missing")
		default: // AgentPermissionAsk (Allow is rejected at registration time)
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeNetwork,
			Action:       fmt.Sprintf("net:%s:%s", direction, protocol),
			Resource:     fmt.Sprintf("%s:%d", host, port),
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeNetwork,
		Action:   fmt.Sprintf("net:%s", direction),
		Resource: fmt.Sprintf("%s:%d", host, port),
	}, "granted", nil)
	m.recordNetworkRule(direction, protocol, host, port)
	return nil
}

// recordNetworkRule stores approved network scopes and forwards them to the
// sandbox runtime so OS-level enforcement mirrors permission checks.
func (m *PermissionManager) recordNetworkRule(direction, protocol, host string, port int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rule := governanceports.SandboxNetworkRule{
		Direction: direction,
		Protocol:  protocol,
		Host:      host,
		Port:      port,
	}
	m.netPolicy = append(m.netPolicy, rule)
	m.applyRuntimePolicyLocked()
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

// CheckCapability verifies capability usage.
func (m *PermissionManager) CheckCapability(ctx context.Context, agentID string, capability string) error {
	if !m.hasCapability(capability) {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeCapability,
			Action:   fmt.Sprintf("cap:%s", capability),
			Resource: capability,
		}, "capability not declared")
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeCapability,
		Action:   fmt.Sprintf("cap:%s", capability),
		Resource: capability,
	}, "granted", nil)
	return nil
}

// CheckIPC validates IPC usage.
func (m *PermissionManager) CheckIPC(ctx context.Context, agentID string, kind string, target string) error {
	perm := m.findIPCPermission(kind, target)
	if perm == nil {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeIPC,
			Action:   fmt.Sprintf("ipc:%s", kind),
			Resource: target,
		}, "ipc scope missing")
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeIPC,
			Action:       fmt.Sprintf("ipc:%s", kind),
			Resource:     perm.Target,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeIPC,
		Action:   fmt.Sprintf("ipc:%s", kind),
		Resource: target,
	}, "granted", nil)
	return nil
}

// collectUndeclared returns human-readable descriptions of any permissions
// required by the tool that are not covered by the agent manifest.
func (m *PermissionManager) collectUndeclared(requirements *permissions.PermissionSet) []string {
	var missing []string
	for _, perm := range requirements.FileSystem {
		if m.findFilesystemPermission(perm.Action, perm.Path) == nil {
			missing = append(missing, fmt.Sprintf("fs %s %s", perm.Action, perm.Path))
		}
	}
	for _, exec := range requirements.Executables {
		if m.findExecutablePermission(exec.Binary) == nil {
			missing = append(missing, fmt.Sprintf("exec %s", exec.Binary))
		}
	}
	for _, net := range requirements.Network {
		if m.findNetworkPermission(net.Direction, net.Protocol, net.Host, net.Port) == nil {
			missing = append(missing, fmt.Sprintf("net %s %s", net.Direction, net.Host))
		}
	}
	return missing
}

// normalizePath sanitizes user input by resolving relative segments and
// symlinks, and preventing traversal outside the workspace base.
//
// The resolved path is guaranteed to be within m.basePath after all
// symlink resolution. Paths that escape the workspace (including via
// symlinks pointing outside) return an error.
func (m *PermissionManager) normalizePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path required")
	}
	// Join with basePath for relative paths so we can always check the
	// result against the workspace boundary.
	resolved := path
	if !filepath.IsAbs(resolved) {
		if m.basePath == "" {
			return filepath.ToSlash(filepath.Clean(resolved)), nil
		}
		resolved = filepath.Join(m.basePath, resolved)
	}
	resolved = filepath.Clean(resolved)
	// Resolve symlinks for the longest existing prefix, then append any
	// non-existent suffix. This handles symlink-to-directory where the
	// final file component does not yet exist (e.g. new file creation
	// through a symlinked directory).
	if canonical, err := resolveCanonicalPath(resolved); err == nil {
		resolved = canonical
	}
	// Verify the resolved path is within the workspace.
	base := filepath.Clean(m.basePath)
	if resolved != base && !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace %q", resolved, base)
	}
	return filepath.ToSlash(resolved), nil
}

func resolveCanonicalPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path required")
	}
	clean := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return filepath.Clean(resolved), nil
	}
	// Walk up from the path until we find an existing ancestor whose
	// symlinks can be resolved, then re-append the non-existent suffix.
	// This supports paths like /workspace/symlink_to_dir/newfile.go
	// where the symlink exists but the leaf file does not.
	suffix := []string{filepath.Base(clean)}
	parent := filepath.Dir(clean)
	for parent != "." && parent != "/" && parent != clean {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			// Re-append suffix from leaf to resolved ancestor
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
	// Nothing exists on this path — return the cleaned original
	return clean, nil
}

// findFilesystemPermission returns the first filesystem permission matching the
// requested action/path pair.
func (m *PermissionManager) findFilesystemPermission(action permissions.FileSystemAction, path string) *permissions.FileSystemPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	if m.isFilesystemProtectedPath(normalized) {
		return nil
	}
	cacheKey := string(action) + ":" + normalized
	m.mu.RLock()
	if perm, ok := m.fsPermCache[cacheKey]; ok {
		m.mu.RUnlock()
		return perm
	}
	m.mu.RUnlock()
	var matched *permissions.FileSystemPermission
	protectedRoots := m.filesystemGuardRootsSnapshot()
	for _, perm := range m.declared.FileSystem {
		if perm.Action != action {
			continue
		}
		if matchGlob(perm.Path, normalized) {
			if len(protectedRoots) > 0 && m.isFilesystemExcludedPath(normalized, protectedRoots) && !filesystemPatternTargetsExcludedRoot(perm.Path, protectedRoots) {
				continue
			}
			permCopy := perm
			matched = &permCopy
			break
		}
	}
	m.mu.Lock()
	m.fsPermCache[cacheKey] = matched
	m.mu.Unlock()
	return matched
}

func normalizeFilesystemRoots(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if path == "" || path == "." {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func (m *PermissionManager) filesystemGuardRootsSnapshot() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.fsProtectedRoots) == 0 && len(m.fsExcludedRoots) == 0 {
		return nil
	}
	roots := make([]string, 0, len(m.fsProtectedRoots)+len(m.fsExcludedRoots))
	roots = append(roots, m.fsProtectedRoots...)
	roots = append(roots, m.fsExcludedRoots...)
	return roots
}

func (m *PermissionManager) isFilesystemProtectedPath(path string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, root := range m.fsProtectedRoots {
		if root != "" && pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func (m *PermissionManager) isFilesystemExcludedPath(path string, roots []string) bool {
	for _, root := range roots {
		if root != "" && pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func filesystemPatternTargetsExcludedRoot(pattern string, roots []string) bool {
	pattern = filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern)))
	if pattern == "" {
		return false
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		root = filepath.ToSlash(filepath.Clean(root))
		if pattern == root || strings.HasPrefix(pattern, root+"/") {
			return true
		}
	}
	return false
}

func pathWithinRoot(target, root string) bool {
	target = filepath.ToSlash(filepath.Clean(target))
	root = filepath.ToSlash(filepath.Clean(root))
	if root == "" {
		return false
	}
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+"/")
}

// findExecutablePermission locates the manifest entry authorizing a binary.
func (m *PermissionManager) findExecutablePermission(binary string) *permissions.ExecutablePermission {
	if m == nil || m.declared == nil {
		return nil
	}
	cacheKey := strings.TrimSpace(binary)
	m.mu.RLock()
	if perm, ok := m.execPermCache[cacheKey]; ok {
		m.mu.RUnlock()
		return perm
	}
	m.mu.RUnlock()
	var matched *permissions.ExecutablePermission
	for _, perm := range m.declared.Executables {
		if perm.Binary == binary {
			permCopy := perm
			matched = &permCopy
			break
		}
	}
	m.mu.Lock()
	m.execPermCache[cacheKey] = matched
	m.mu.Unlock()
	return matched
}

// findNetworkPermission resolves whether the host/port pair is authorized for
// the given direction/protocol combination.
func (m *PermissionManager) findNetworkPermission(direction, protocol, host string, port int) *permissions.NetworkPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	target := fmt.Sprintf("%s:%d", host, port)
	for _, perm := range m.declared.Network {
		if perm.Direction != direction || perm.Protocol != protocol {
			continue
		}
		if perm.Direction == "egress" {
			if perm.Port != 0 && perm.Port != port {
				continue
			}
			if perm.Host == host || perm.Host == permissionMatchAll || matchGlob(perm.Host, host) {
				return &perm
			}
		} else if perm.Direction == "ingress" {
			if perm.Port == port || perm.Port == 0 {
				return &perm
			}
		} else if perm.Direction == "dns" && perm.Host == "" {
			return &perm
		}
		if perm.Host == target {
			return &perm
		}
	}
	return nil
}

// findIPCPermission determines if the IPC target was declared in the manifest.
func (m *PermissionManager) findIPCPermission(kind, target string) *permissions.IPCPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	for _, perm := range m.declared.IPC {
		if perm.Kind == kind && (perm.Target == target || perm.Target == permissionMatchAll) {
			return &perm
		}
	}
	return nil
}

// hasCapability checks whether a Linux capability was granted to the agent.
func (m *PermissionManager) hasCapability(cap string) bool {
	if m == nil || m.declared == nil {
		return false
	}
	for _, perm := range m.declared.Capabilities {
		if perm.Capability == cap {
			return true
		}
	}
	return false
}

// ensureGrant obtains a HITL approval when a permission requires human review.
func (m *PermissionManager) ensureGrant(ctx context.Context, agentID string, desc permissions.PermissionDescriptor) error {
	key := desc.Action + ":" + desc.Resource
	m.mu.Lock()
	if grant, ok := m.grants[key]; ok {
		if !grant.Expired(m.grantClock()) {
			m.mu.Unlock()
			return nil
		}
		delete(m.grants, key)
	}
	m.mu.Unlock()
	if m.hitl == nil {
		return m.deny(ctx, agentID, desc, "hitl approval required")
	}
	grant, err := m.hitl.RequestPermission(ctx, PermissionRequest{
		Permission:    desc,
		Justification: "runtime request",
		Scope:         policy.GrantScopeSession,
		Risk:          policy.RiskLevelMedium,
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.grants[key] = grant
	m.mu.Unlock()
	return nil
}

// checkHITLRateLimit returns an error if the per-key HITL request rate exceeds
// hitlRateMax within hitlRateWindow. Must be called without m.mu held.
func (m *PermissionManager) checkHITLRateLimit(key string) error {
	now := m.grantClock()
	m.mu.Lock()
	defer m.mu.Unlock()
	bucket, ok := m.hitlRateLimits[key]
	if !ok || now.Sub(bucket.windowAt) >= hitlRateWindow {
		m.hitlRateLimits[key] = &hitlRateBucket{count: 1, windowAt: now}
		return nil
	}
	bucket.count++
	if bucket.count > hitlRateMax {
		return fmt.Errorf("HITL rate limit exceeded for %s: max %d requests per %s", key, hitlRateMax, hitlRateWindow)
	}
	return nil
}

// RequireApproval requests HITL approval for an arbitrary runtime decision
// (tool gating, file matrix, bash policy) and caches the resulting grant.
func (m *PermissionManager) RequireApproval(ctx context.Context, agentID string, desc permissions.PermissionDescriptor, justification string, scope policy.GrantScope, risk policy.RiskLevel, duration time.Duration) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	desc.RequiresHITL = true
	key := desc.Action + ":" + desc.Resource
	m.mu.Lock()
	if grant, ok := m.grants[key]; ok {
		if !grant.Expired(m.grantClock()) {
			m.mu.Unlock()
			return nil
		}
		delete(m.grants, key)
	}
	m.mu.Unlock()
	if err := m.checkHITLRateLimit(key); err != nil {
		m.emitPolicyDecision(ctx, desc, "deny", err.Error(), nil)
		return err
	}
	if m.hitl == nil {
		return m.deny(ctx, agentID, desc, "hitl approval required")
	}
	if scope == "" {
		scope = policy.GrantScopeOneTime
	}
	if risk == "" {
		risk = policy.RiskLevelMedium
	}
	grant, err := m.hitl.RequestPermission(ctx, PermissionRequest{
		Permission:    desc,
		Justification: justification,
		Scope:         scope,
		Duration:      duration,
		Risk:          risk,
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.grants[key] = grant
	m.mu.Unlock()
	return nil
}

// deny records an audit event and returns a structured error describing why an
// action was blocked.
func (m *PermissionManager) deny(ctx context.Context, agentID string, desc permissions.PermissionDescriptor, reason string) error {
	m.log(ctx, agentID, desc, "denied", map[string]interface{}{
		"reason": reason,
	})
	m.emitPolicyDecision(ctx, desc, "deny", reason, nil)
	return &permissions.PermissionDeniedError{
		Descriptor: desc,
		Message:    reason,
	}
}

func (m *PermissionManager) emitPolicyDecision(ctx context.Context, desc permissions.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
	if m == nil {
		return
	}
	m.mu.RLock()
	logger := m.eventLogger
	m.mu.RUnlock()
	if logger == nil {
		return
	}
	logger(ctx, desc, effect, reason, fields)
}

// sensitivePathPatterns are substrings that indicate a file path may contain
// sensitive data warranting redaction from audit records.
var sensitivePathPatterns = []string{
	".env",
	".ssh",
	"secret",
	"token",
	"credential",
	"password",
	"key.pem",
	"id_rsa",
	"id_ed25519",
}

// redactSensitivePath replaces path components matching sensitive patterns
// with "[REDACTED]" for audit-safe logging.
func redactSensitivePath(path string) string {
	lower := strings.ToLower(path)
	for _, pattern := range sensitivePathPatterns {
		if strings.Contains(lower, pattern) {
			// Return a placeholder instead of the full path to avoid
			// leaking the sensitive location or its context.
			return "[REDACTED_PATH]"
		}
	}
	return path
}

// log forwards permission decisions to the configured audit sink to provide a
// tamper-evident trail of runtime behavior.
func (m *PermissionManager) log(ctx context.Context, agentID string, desc permissions.PermissionDescriptor, result string, fields map[string]interface{}) {
	if m.audit == nil {
		return
	}
	record := policy.AuditRecord{
		Timestamp:   time.Now().UTC(),
		AgentID:     agentID,
		Action:      desc.Action,
		Type:        string(desc.Type),
		Permission:  redactSensitivePath(desc.Resource),
		Result:      result,
		Metadata:    capability.RedactMetadataMap(fields),
		Correlation: agentID,
	}
	_ = m.audit.Log(ctx, record)
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

// matchArgs compares declared argument patterns with a runtime invocation while
// supporting simple globbing for flags.
func matchArgs(patterns, args []string) bool {
	if len(patterns) == 0 {
		return true
	}
	if len(patterns) == 1 && patterns[0] == "*" {
		return true
	}
	hasTrailingWildcard := len(patterns) > 0 && patterns[len(patterns)-1] == "*"
	if !hasTrailingWildcard && len(patterns) != len(args) {
		return false
	}
	if hasTrailingWildcard && len(args) < len(patterns)-1 {
		return false
	}
	limit := len(patterns)
	if len(args) < limit {
		limit = len(args)
	}
	for i := 0; i < limit; i++ {
		pattern := patterns[i]
		if hasTrailingWildcard && i == len(patterns)-1 {
			break
		}
		if pattern == "*" {
			continue
		}
		if strings.HasPrefix(pattern, "--") && strings.HasSuffix(pattern, "*") {
			if !strings.HasPrefix(args[i], strings.TrimSuffix(pattern, "*")) {
				return false
			}
			continue
		}
		if pattern != args[i] {
			return false
		}
	}
	return true
}

// matchEnv verifies required environment variables match the expected values or
// contain wildcards where any value is acceptable.
func matchEnv(patterns, env []string) bool {
	if len(patterns) == 0 {
		return true
	}
	m := map[string]string{}
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	for _, pattern := range patterns {
		parts := strings.SplitN(pattern, "=", 2)
		if len(parts) != 2 {
			continue
		}
		val, ok := m[parts[0]]
		if !ok {
			return false
		}
		if parts[1] != "*" && parts[1] != val {
			return false
		}
	}
	return true
}

// CheckFilePermission implements permissions.FilePermissionChecker.
// It validates a file operation against the agent's file matrix.
func (m *PermissionManager) CheckFilePermission(ctx context.Context, agentID, basePath, action, absPath string, matrix AgentFileMatrix) error {
	rel := absPath
	if basePath != "" {
		if r, err := filepath.Rel(basePath, absPath); err == nil {
			rel = r
		}
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(rel, "./") {
		rel = strings.TrimPrefix(rel, "./")
	}
	perm := matrix.Write
	if action == "edit" {
		perm = matrix.Edit
	}
	if perm.DocumentationOnly && !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return fmt.Errorf("file %s blocked: documentation_only enabled", rel)
	}
	decision, _ := DecideByPatterns(rel, perm.AllowPatterns, perm.DenyPatterns, permissions.AgentPermissionLevel(perm.Default))
	if perm.RequireApproval {
		decision = permissions.AgentPermissionAsk
	}
	switch decision {
	case permissions.AgentPermissionAllow:
		return nil
	case permissions.AgentPermissionDeny:
		return fmt.Errorf("file %s blocked: denied by file_permissions", rel)
	case permissions.AgentPermissionAsk:
		if m == nil {
			return fmt.Errorf("file %s blocked: approval required but permission manager missing", rel)
		}
		return m.RequireApproval(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeHITL,
			Action:       fmt.Sprintf("file_matrix:%s", action),
			Resource:     rel,
			RequiresHITL: true,
		}, "file permission matrix", policy.GrantScopeOneTime, policy.RiskLevelMedium, 0)
	default:
		return nil
	}
}

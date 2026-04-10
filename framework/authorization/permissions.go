// Package runtime enforces agent permission contracts at execution time.
// PermissionManager authorises tool calls, file access, executable invocations, and
// network requests against permissions declared in the agent manifest, applying a
// three-level policy (Allow / Ask / Deny) with Human-in-the-Loop approval flows
// and configurable GrantScope (OneTime, Session, Persistent).
package authorization

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

const permissionMatchAll = "**"

// hitlRateMax is the maximum HITL requests per key within hitlRateWindow before
// subsequent requests are rejected with a rate-limit error.
const hitlRateMax = 10

// hitlRateWindow is the sliding window duration for HITL rate limiting.
const hitlRateWindow = time.Minute

// globRegexCache caches compiled glob-to-regex patterns to avoid recompiling on every match.
var globRegexCache sync.Map // map[string]*regexp.Regexp

// hitlRateBucket tracks the number of HITL requests within a rolling time window.
type hitlRateBucket struct {
	count    int
	windowAt time.Time
}

// PermissionManager enforces the declared permission set for runtime actions.
type PermissionManager struct {
	basePath         string
	declared         *core.PermissionSet
	audit            core.AuditLogger
	hitl             HITLProvider
	runtime          sandbox.SandboxRuntime
	grants           map[string]*PermissionGrant
	mu               sync.RWMutex
	grantClock       func() time.Time
	netPolicy        []sandbox.NetworkRule
	defaultPolicy    core.AgentPermissionLevel // governs undeclared tool permissions; default is Ask
	eventLogger      func(context.Context, core.PermissionDescriptor, string, string, map[string]interface{})
	runtimePolicyErr error
	taskGrants       map[string]taskGrant
	hitlRateLimits   map[string]*hitlRateBucket
	fsPermCache      map[string]*core.FileSystemPermission
	execPermCache    map[string]*core.ExecutablePermission
}

type taskGrant struct {
	runID        string
	approvedTags map[string]struct{}
}

// NewPermissionManager creates an enforcement instance.
func NewPermissionManager(basePath string, declared *core.PermissionSet, audit core.AuditLogger, hitl HITLProvider) (*PermissionManager, error) {
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
		fsPermCache:    make(map[string]*core.FileSystemPermission),
		execPermCache:  make(map[string]*core.ExecutablePermission),
		grantClock:     time.Now,
	}
	pm.inflateScopes()
	return pm, nil
}

// AttachRuntime allows the manager to push policy updates to the sandbox.
func (m *PermissionManager) AttachRuntime(runtime sandbox.SandboxRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime = runtime
	m.applyRuntimePolicyLocked()
}

// SetDefaultPolicy configures how undeclared permissions are handled.
// core.AgentPermissionAsk (default) routes to HITL; Allow bypasses; Deny hard-blocks.
func (m *PermissionManager) SetDefaultPolicy(level core.AgentPermissionLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultPolicy = level
}

// SetEventLogger configures a callback for structured policy decision events.
func (m *PermissionManager) SetEventLogger(logger func(context.Context, core.PermissionDescriptor, string, string, map[string]interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventLogger = logger
}

// DefaultPolicy returns the configured default policy level, falling back to Ask.
func (m *PermissionManager) DefaultPolicy() core.AgentPermissionLevel {
	return m.effectiveDefaultPolicy()
}

// effectiveDefaultPolicy returns the configured policy, falling back to Ask.
func (m *PermissionManager) effectiveDefaultPolicy() core.AgentPermissionLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultPolicy == "" {
		return core.AgentPermissionAsk
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
func (m *PermissionManager) AuthorizeTool(ctx context.Context, agentID string, tool core.Tool, args map[string]interface{}) error {
	if m == nil || tool == nil {
		return errors.New("permission manager or tool missing")
	}
	if m.toolAllowedByTaskGrant(ctx, tool) {
		desc := core.PermissionDescriptor{
			Type:     core.PermissionTypeHITL,
			Action:   fmt.Sprintf("tool:%s", tool.Name()),
			Resource: agentID,
		}
		m.log(ctx, agentID, desc, "tool_allowed_task_grant", map[string]interface{}{"tags": tool.Tags()})
		m.emitPolicyDecision(ctx, desc, "allow", "task grant matched tool tags", map[string]interface{}{"tags": tool.Tags()})
		return nil
	}
	requirements := tool.Permissions()
	if err := requirements.Validate(); err != nil {
		return fmt.Errorf("tool %s permission invalid: %w", tool.Name(), err)
	}
	undeclared := m.collectUndeclared(requirements.Permissions)
	if len(undeclared) > 0 {
		switch m.effectiveDefaultPolicy() {
		case core.AgentPermissionDeny:
			m.emitPolicyDecision(ctx, core.PermissionDescriptor{
				Type:     core.PermissionTypeHITL,
				Action:   fmt.Sprintf("tool:%s", tool.Name()),
				Resource: agentID,
			}, "deny", "tool exceeds declared permissions", map[string]interface{}{"undeclared": undeclared})
			return fmt.Errorf("tool %s exceeds agent permissions: %s", tool.Name(), strings.Join(undeclared, "; "))
		case core.AgentPermissionAllow:
			m.emitPolicyDecision(ctx, core.PermissionDescriptor{
				Type:     core.PermissionTypeHITL,
				Action:   fmt.Sprintf("tool:%s", tool.Name()),
				Resource: agentID,
			}, "allow", "undeclared permissions allowed by default policy", map[string]interface{}{"undeclared": undeclared})
			// undeclared permissions are explicitly allowed — proceed
		default: // core.AgentPermissionAsk
			m.emitPolicyDecision(ctx, core.PermissionDescriptor{
				Type:         core.PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", tool.Name()),
				Resource:     agentID,
				RequiresHITL: true,
			}, "require_approval", "undeclared permissions require approval", map[string]interface{}{"undeclared": undeclared})
			if err := m.RequireApproval(ctx, agentID, core.PermissionDescriptor{
				Type:         core.PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", tool.Name()),
				Resource:     agentID,
				RequiresHITL: true,
			}, fmt.Sprintf("tool %s requires: %s", tool.Name(), strings.Join(undeclared, ", ")),
				GrantScopeSession, RiskLevelMedium, 0); err != nil {
				return err
			}
		}
	}
	desc := core.PermissionDescriptor{
		Type:     core.PermissionTypeHITL,
		Action:   fmt.Sprintf("tool:%s", tool.Name()),
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
func (m *PermissionManager) GrantPermission(desc core.PermissionDescriptor, approvedBy string, scope GrantScope, duration time.Duration) {
	if m == nil {
		return
	}
	grant := GrantManual(desc, approvedBy, scope, duration)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grants[desc.Action+":"+desc.Resource] = grant
}

func (m *PermissionManager) toolAllowedByTaskGrant(ctx context.Context, tool core.Tool) bool {
	if m == nil || tool == nil {
		return false
	}
	taskCtx, ok := core.TaskContextFrom(ctx)
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
func (m *PermissionManager) CheckFileAccess(ctx context.Context, agentID string, action core.FileSystemAction, path string) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	clean, err := m.normalizePath(path)
	if err != nil {
		return err
	}
	perm := m.findFilesystemPermission(action, clean)
	if perm == nil {
		desc := core.PermissionDescriptor{
			Type:     core.PermissionTypeFilesystem,
			Action:   string(action),
			Resource: clean,
		}
		switch m.effectiveDefaultPolicy() {
		case core.AgentPermissionAllow:
			m.log(ctx, agentID, desc, "granted (default allow)", nil)
			return nil
		case core.AgentPermissionDeny:
			return m.deny(ctx, agentID, desc, "not declared")
		default: // core.AgentPermissionAsk
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, core.PermissionDescriptor{
			Type:         core.PermissionTypeFilesystem,
			Action:       string(action),
			Resource:     perm.Path,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, core.PermissionDescriptor{
		Type:     core.PermissionTypeFilesystem,
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
		desc := core.PermissionDescriptor{
			Type:     core.PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:binary:%s", binary),
			Resource: binary,
		}
		switch m.effectiveDefaultPolicy() {
		case core.AgentPermissionAllow:
			m.log(ctx, agentID, desc, "granted (default allow)", nil)
			return nil
		case core.AgentPermissionDeny:
			return m.deny(ctx, agentID, desc, "binary not declared")
		default: // core.AgentPermissionAsk
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if len(perm.Args) > 0 && !matchArgs(perm.Args, args) {
		return m.deny(ctx, agentID, core.PermissionDescriptor{
			Type:     core.PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:args:%s", strings.Join(args, " ")),
			Resource: binary,
		}, "arguments rejected")
	}
	if len(perm.Env) > 0 && !matchEnv(perm.Env, env) {
		return m.deny(ctx, agentID, core.PermissionDescriptor{
			Type:     core.PermissionTypeExecutable,
			Action:   "exec:env",
			Resource: binary,
		}, "environment rejected")
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, core.PermissionDescriptor{
			Type:         core.PermissionTypeExecutable,
			Action:       fmt.Sprintf("exec:binary:%s", binary),
			Resource:     binary,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, core.PermissionDescriptor{
		Type:     core.PermissionTypeExecutable,
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
	perm := m.findNetworkPermission(direction, protocol, host, port)
	if perm == nil {
		desc := core.PermissionDescriptor{
			Type:     core.PermissionTypeNetwork,
			Action:   fmt.Sprintf("net:%s:%s:%s:%d", direction, protocol, host, port),
			Resource: host,
		}
		switch m.effectiveDefaultPolicy() {
		case core.AgentPermissionAllow:
			m.log(ctx, agentID, desc, "granted (default allow)", nil)
			return nil
		case core.AgentPermissionDeny:
			return m.deny(ctx, agentID, desc, "network scope missing")
		default: // core.AgentPermissionAsk
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, core.PermissionDescriptor{
			Type:         core.PermissionTypeNetwork,
			Action:       fmt.Sprintf("net:%s:%s", direction, protocol),
			Resource:     fmt.Sprintf("%s:%d", host, port),
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, core.PermissionDescriptor{
		Type:     core.PermissionTypeNetwork,
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
	rule := sandbox.NetworkRule{
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
func (m *PermissionManager) Policy() sandbox.SandboxPolicy {
	if m == nil {
		return sandbox.SandboxPolicy{}
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

func (m *PermissionManager) currentSandboxPolicyLocked() sandbox.SandboxPolicy {
	if m == nil {
		return sandbox.SandboxPolicy{}
	}
	policy := sandbox.SandboxPolicy{}
	if m.runtime != nil {
		policy = m.runtime.Policy()
	}
	policy.NetworkRules = append([]sandbox.NetworkRule(nil), m.netPolicy...)
	return policy
}

// CheckCapability verifies capability usage.
func (m *PermissionManager) CheckCapability(ctx context.Context, agentID string, capability string) error {
	if !m.hasCapability(capability) {
		return m.deny(ctx, agentID, core.PermissionDescriptor{
			Type:     core.PermissionTypeCapability,
			Action:   fmt.Sprintf("cap:%s", capability),
			Resource: capability,
		}, "capability not declared")
	}
	m.log(ctx, agentID, core.PermissionDescriptor{
		Type:     core.PermissionTypeCapability,
		Action:   fmt.Sprintf("cap:%s", capability),
		Resource: capability,
	}, "granted", nil)
	return nil
}

// CheckIPC validates IPC usage.
func (m *PermissionManager) CheckIPC(ctx context.Context, agentID string, kind string, target string) error {
	perm := m.findIPCPermission(kind, target)
	if perm == nil {
		return m.deny(ctx, agentID, core.PermissionDescriptor{
			Type:     core.PermissionTypeIPC,
			Action:   fmt.Sprintf("ipc:%s", kind),
			Resource: target,
		}, "ipc scope missing")
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, core.PermissionDescriptor{
			Type:         core.PermissionTypeIPC,
			Action:       fmt.Sprintf("ipc:%s", kind),
			Resource:     perm.Target,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, core.PermissionDescriptor{
		Type:     core.PermissionTypeIPC,
		Action:   fmt.Sprintf("ipc:%s", kind),
		Resource: target,
	}, "granted", nil)
	return nil
}

// collectUndeclared returns human-readable descriptions of any permissions
// required by the tool that are not covered by the agent manifest.
func (m *PermissionManager) collectUndeclared(requirements *core.PermissionSet) []string {
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
// preventing traversal outside the workspace base.
func (m *PermissionManager) normalizePath(path string) (string, error) {
	clean := filepath.Clean(path)
	clean = filepath.ToSlash(clean)
	if strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("path traversal detected: %s", path)
	}
	if strings.HasPrefix(clean, "..") && clean != ".." {
		return "", fmt.Errorf("path traversal detected: %s", path)
	}
	if filepath.IsAbs(clean) {
		return clean, nil
	}
	if m.basePath == "" {
		return clean, nil
	}
	return filepath.ToSlash(filepath.Join(m.basePath, clean)), nil
}

func resolveCanonicalPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path required")
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved), nil
	}
	current := path
	suffix := make([]string, 0, 4)
	for {
		parent := filepath.Dir(current)
		if parent == current {
			current = path
			suffix = suffix[:0]
			break
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
	resolvedAncestor, err := filepath.EvalSymlinks(current)
	if err != nil {
		resolvedAncestor = current
	}
	resolved := resolvedAncestor
	for _, part := range suffix {
		resolved = filepath.Join(resolved, part)
	}
	return filepath.Clean(resolved), nil
}

// findFilesystemPermission returns the first filesystem permission matching the
// requested action/path pair.
func (m *PermissionManager) findFilesystemPermission(action core.FileSystemAction, path string) *core.FileSystemPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	cacheKey := string(action) + ":" + normalized
	m.mu.RLock()
	if perm, ok := m.fsPermCache[cacheKey]; ok {
		m.mu.RUnlock()
		return perm
	}
	m.mu.RUnlock()
	var matched *core.FileSystemPermission
	for _, perm := range m.declared.FileSystem {
		if perm.Action != action {
			continue
		}
		if matchGlob(perm.Path, normalized) {
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

// findExecutablePermission locates the manifest entry authorizing a binary.
func (m *PermissionManager) findExecutablePermission(binary string) *core.ExecutablePermission {
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
	var matched *core.ExecutablePermission
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
func (m *PermissionManager) findNetworkPermission(direction, protocol, host string, port int) *core.NetworkPermission {
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
func (m *PermissionManager) findIPCPermission(kind, target string) *core.IPCPermission {
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
func (m *PermissionManager) ensureGrant(ctx context.Context, agentID string, desc core.PermissionDescriptor) error {
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
		Scope:         GrantScopeSession,
		Risk:          RiskLevelMedium,
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
func (m *PermissionManager) RequireApproval(ctx context.Context, agentID string, desc core.PermissionDescriptor, justification string, scope GrantScope, risk RiskLevel, duration time.Duration) error {
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
		scope = GrantScopeOneTime
	}
	if risk == "" {
		risk = RiskLevelMedium
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
func (m *PermissionManager) deny(ctx context.Context, agentID string, desc core.PermissionDescriptor, reason string) error {
	m.log(ctx, agentID, desc, "denied", map[string]interface{}{
		"reason": reason,
	})
	m.emitPolicyDecision(ctx, desc, "deny", reason, nil)
	return &core.PermissionDeniedError{
		Descriptor: desc,
		Message:    reason,
	}
}

func (m *PermissionManager) emitPolicyDecision(ctx context.Context, desc core.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
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

// log forwards permission decisions to the configured audit sink to provide a
// tamper-evident trail of runtime behavior.
func (m *PermissionManager) log(ctx context.Context, agentID string, desc core.PermissionDescriptor, result string, fields map[string]interface{}) {
	if m.audit == nil {
		return
	}
	record := core.AuditRecord{
		Timestamp:   time.Now().UTC(),
		AgentID:     agentID,
		Action:      desc.Action,
		Type:        string(desc.Type),
		Permission:  desc.Resource,
		Result:      result,
		Metadata:    core.RedactMetadataMap(fields),
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
	var regex *regexp.Regexp
	if cached, ok := globRegexCache.Load(regexPattern); ok {
		regex = cached.(*regexp.Regexp)
	} else {
		compiled, err := regexp.Compile(regexPattern)
		if err != nil {
			return false
		}
		globRegexCache.Store(regexPattern, compiled)
		regex = compiled
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
	Type     core.PermissionType
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
	Permission  core.PermissionDescriptor
	Scope       GrantScope
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

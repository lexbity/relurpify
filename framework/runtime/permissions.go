// Package runtime enforces agent permission contracts at execution time.
// PermissionManager authorises tool calls, file access, executable invocations, and
// network requests against permissions declared in the agent manifest, applying a
// three-level policy (Allow / Ask / Deny) with Human-in-the-Loop approval flows
// and configurable GrantScope (OneTime, Session, Persistent).
package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const permissionMatchAll = "**"

// PermissionManager enforces the declared permission set for runtime actions.
type PermissionManager struct {
	basePath      string
	declared      *PermissionSet
	audit         AuditLogger
	hitl          HITLProvider
	runtime       SandboxRuntime
	grants        map[string]*PermissionGrant
	mu            sync.RWMutex
	grantClock    func() time.Time
	netPolicy     []NetworkRule
	defaultPolicy AgentPermissionLevel // governs undeclared tool permissions; default is Ask
}

// NewPermissionManager creates an enforcement instance.
func NewPermissionManager(basePath string, declared *PermissionSet, audit AuditLogger, hitl HITLProvider) (*PermissionManager, error) {
	if declared == nil {
		return nil, errors.New("permission manager requires permission set")
	}
	if err := declared.Validate(); err != nil {
		return nil, err
	}
	pm := &PermissionManager{
		basePath:   basePath,
		declared:   declared,
		audit:      audit,
		hitl:       hitl,
		grants:     make(map[string]*PermissionGrant),
		grantClock: time.Now,
	}
	pm.inflateScopes()
	return pm, nil
}

// AttachRuntime allows the manager to push policy updates to the sandbox.
func (m *PermissionManager) AttachRuntime(runtime SandboxRuntime) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime = runtime
	if len(m.netPolicy) > 0 {
		_ = runtime.EnforcePolicy(SandboxPolicy{NetworkRules: m.netPolicy})
	}
}

// SetDefaultPolicy configures how undeclared permissions are handled.
// AgentPermissionAsk (default) routes to HITL; Allow bypasses; Deny hard-blocks.
func (m *PermissionManager) SetDefaultPolicy(level AgentPermissionLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultPolicy = level
}

// effectiveDefaultPolicy returns the configured policy, falling back to Ask.
func (m *PermissionManager) effectiveDefaultPolicy() AgentPermissionLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultPolicy == "" {
		return AgentPermissionAsk
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
func (m *PermissionManager) AuthorizeTool(ctx context.Context, agentID string, tool Tool, args map[string]interface{}) error {
	if m == nil || tool == nil {
		return errors.New("permission manager or tool missing")
	}
	requirements := tool.Permissions()
	if err := requirements.Validate(); err != nil {
		return fmt.Errorf("tool %s permission invalid: %w", tool.Name(), err)
	}
	undeclared := m.collectUndeclared(requirements.Permissions)
	if len(undeclared) > 0 {
		switch m.effectiveDefaultPolicy() {
		case AgentPermissionDeny:
			return fmt.Errorf("tool %s exceeds agent permissions: %s", tool.Name(), strings.Join(undeclared, "; "))
		case AgentPermissionAllow:
			// undeclared permissions are explicitly allowed — proceed
		default: // AgentPermissionAsk
			if err := m.RequireApproval(ctx, agentID, PermissionDescriptor{
				Type:         PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", tool.Name()),
				Resource:     agentID,
				RequiresHITL: true,
			}, fmt.Sprintf("tool %s requires: %s", tool.Name(), strings.Join(undeclared, ", ")),
				GrantScopeSession, RiskLevelMedium, 0); err != nil {
				return err
			}
		}
	}
	desc := PermissionDescriptor{
		Type:     PermissionTypeHITL,
		Action:   fmt.Sprintf("tool:%s", tool.Name()),
		Resource: agentID,
	}
	m.log(ctx, agentID, desc, "tool_allowed", nil)
	return nil
}

// CheckFileAccess validates filesystem access.
func (m *PermissionManager) CheckFileAccess(ctx context.Context, agentID string, action FileSystemAction, path string) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	clean, err := m.normalizePath(path)
	if err != nil {
		return err
	}
	perm := m.findFilesystemPermission(action, clean)
	if perm == nil {
		desc := PermissionDescriptor{
			Type:     PermissionTypeFilesystem,
			Action:   string(action),
			Resource: clean,
		}
		switch m.effectiveDefaultPolicy() {
		case AgentPermissionAllow:
			m.log(ctx, agentID, desc, "granted (default allow)", nil)
			return nil
		case AgentPermissionDeny:
			return m.deny(ctx, agentID, desc, "not declared")
		default: // AgentPermissionAsk
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, PermissionDescriptor{
			Type:         PermissionTypeFilesystem,
			Action:       string(action),
			Resource:     perm.Path,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, PermissionDescriptor{
		Type:     PermissionTypeFilesystem,
		Action:   string(action),
		Resource: clean,
	}, "granted", map[string]interface{}{
		"pattern": perm.Path,
	})
	return nil
}

// CheckExecutable validates binary execution.
func (m *PermissionManager) CheckExecutable(ctx context.Context, agentID, binary string, args []string, env []string) error {
	perm := m.findExecutablePermission(binary)
	if perm == nil {
		desc := PermissionDescriptor{
			Type:     PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:binary:%s", binary),
			Resource: binary,
		}
		switch m.effectiveDefaultPolicy() {
		case AgentPermissionAllow:
			m.log(ctx, agentID, desc, "granted (default allow)", nil)
			return nil
		case AgentPermissionDeny:
			return m.deny(ctx, agentID, desc, "binary not declared")
		default: // AgentPermissionAsk
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if len(perm.Args) > 0 && !matchArgs(perm.Args, args) {
		return m.deny(ctx, agentID, PermissionDescriptor{
			Type:     PermissionTypeExecutable,
			Action:   fmt.Sprintf("exec:args:%s", strings.Join(args, " ")),
			Resource: binary,
		}, "arguments rejected")
	}
	if len(perm.Env) > 0 && !matchEnv(perm.Env, env) {
		return m.deny(ctx, agentID, PermissionDescriptor{
			Type:     PermissionTypeExecutable,
			Action:   "exec:env",
			Resource: binary,
		}, "environment rejected")
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, PermissionDescriptor{
			Type:         PermissionTypeExecutable,
			Action:       fmt.Sprintf("exec:binary:%s", binary),
			Resource:     binary,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, PermissionDescriptor{
		Type:     PermissionTypeExecutable,
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
		desc := PermissionDescriptor{
			Type:     PermissionTypeNetwork,
			Action:   fmt.Sprintf("net:%s:%s:%s:%d", direction, protocol, host, port),
			Resource: host,
		}
		switch m.effectiveDefaultPolicy() {
		case AgentPermissionAllow:
			m.log(ctx, agentID, desc, "granted (default allow)", nil)
			return nil
		case AgentPermissionDeny:
			return m.deny(ctx, agentID, desc, "network scope missing")
		default: // AgentPermissionAsk
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, PermissionDescriptor{
			Type:         PermissionTypeNetwork,
			Action:       fmt.Sprintf("net:%s:%s", direction, protocol),
			Resource:     fmt.Sprintf("%s:%d", host, port),
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, PermissionDescriptor{
		Type:     PermissionTypeNetwork,
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
	rule := NetworkRule{
		Direction: direction,
		Protocol:  protocol,
		Host:      host,
		Port:      port,
	}
	m.netPolicy = append(m.netPolicy, rule)
	if m.runtime != nil {
		_ = m.runtime.EnforcePolicy(SandboxPolicy{
			NetworkRules: append([]NetworkRule(nil), m.netPolicy...),
		})
	}
}

// CheckCapability verifies capability usage.
func (m *PermissionManager) CheckCapability(ctx context.Context, agentID string, capability string) error {
	if !m.hasCapability(capability) {
		return m.deny(ctx, agentID, PermissionDescriptor{
			Type:     PermissionTypeCapability,
			Action:   fmt.Sprintf("cap:%s", capability),
			Resource: capability,
		}, "capability not declared")
	}
	m.log(ctx, agentID, PermissionDescriptor{
		Type:     PermissionTypeCapability,
		Action:   fmt.Sprintf("cap:%s", capability),
		Resource: capability,
	}, "granted", nil)
	return nil
}

// CheckIPC validates IPC usage.
func (m *PermissionManager) CheckIPC(ctx context.Context, agentID string, kind string, target string) error {
	perm := m.findIPCPermission(kind, target)
	if perm == nil {
		return m.deny(ctx, agentID, PermissionDescriptor{
			Type:     PermissionTypeIPC,
			Action:   fmt.Sprintf("ipc:%s", kind),
			Resource: target,
		}, "ipc scope missing")
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, PermissionDescriptor{
			Type:         PermissionTypeIPC,
			Action:       fmt.Sprintf("ipc:%s", kind),
			Resource:     perm.Target,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, PermissionDescriptor{
		Type:     PermissionTypeIPC,
		Action:   fmt.Sprintf("ipc:%s", kind),
		Resource: target,
	}, "granted", nil)
	return nil
}

// collectUndeclared returns human-readable descriptions of any permissions
// required by the tool that are not covered by the agent manifest.
func (m *PermissionManager) collectUndeclared(requirements *PermissionSet) []string {
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

// findFilesystemPermission returns the first filesystem permission matching the
// requested action/path pair.
func (m *PermissionManager) findFilesystemPermission(action FileSystemAction, path string) *FileSystemPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	for _, perm := range m.declared.FileSystem {
		if perm.Action != action {
			continue
		}
		if matchGlob(perm.Path, normalized) {
			return &perm
		}
	}
	return nil
}

// findExecutablePermission locates the manifest entry authorizing a binary.
func (m *PermissionManager) findExecutablePermission(binary string) *ExecutablePermission {
	if m == nil || m.declared == nil {
		return nil
	}
	for _, perm := range m.declared.Executables {
		if perm.Binary == binary {
			return &perm
		}
	}
	return nil
}

// findNetworkPermission resolves whether the host/port pair is authorized for
// the given direction/protocol combination.
func (m *PermissionManager) findNetworkPermission(direction, protocol, host string, port int) *NetworkPermission {
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
func (m *PermissionManager) findIPCPermission(kind, target string) *IPCPermission {
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
func (m *PermissionManager) ensureGrant(ctx context.Context, agentID string, desc PermissionDescriptor) error {
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

// RequireApproval requests HITL approval for an arbitrary runtime decision
// (tool gating, file matrix, bash policy) and caches the resulting grant.
func (m *PermissionManager) RequireApproval(ctx context.Context, agentID string, desc PermissionDescriptor, justification string, scope GrantScope, risk RiskLevel, duration time.Duration) error {
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
func (m *PermissionManager) deny(ctx context.Context, agentID string, desc PermissionDescriptor, reason string) error {
	m.log(ctx, agentID, desc, "denied", map[string]interface{}{
		"reason": reason,
	})
	return &PermissionDeniedError{
		Descriptor: desc,
		Message:    reason,
	}
}

// log forwards permission decisions to the configured audit sink to provide a
// tamper-evident trail of runtime behavior.
func (m *PermissionManager) log(ctx context.Context, agentID string, desc PermissionDescriptor, result string, fields map[string]interface{}) {
	if m.audit == nil {
		return
	}
	record := AuditRecord{
		Timestamp:   time.Now().UTC(),
		AgentID:     agentID,
		Action:      desc.Action,
		Type:        string(desc.Type),
		Permission:  desc.Resource,
		Result:      result,
		Metadata:    RedactMetadataMap(fields),
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
	regex, err := regexp.Compile(regexPattern)
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
	Type     PermissionType
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
	Permission  PermissionDescriptor
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

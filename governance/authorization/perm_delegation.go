package authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

// hitlRateMax is the maximum HITL requests per key within hitlRateWindow before
// subsequent requests are rejected with a rate-limit error.
const hitlRateMax = 10

// hitlRateWindow is the sliding window duration for HITL rate limiting.
const hitlRateWindow = time.Minute

// hitlRateBucket tracks the number of HITL requests within a rolling time window.
type hitlRateBucket struct {
	count    int
	windowAt time.Time
}

type taskGrant struct {
	runID        string
	approvedTags map[string]struct{}
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
	principal := governanceports.PrincipalFromContext(ctx)
	if strings.TrimSpace(principal.AgentID) == "" {
		return false
	}
	m.mu.RLock()
	grant, ok := m.taskGrants[principal.AgentID]
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
	m.log(ctx, agentID, desc, "denied", map[string]any{
		"reason": reason,
	})
	m.emitPolicyDecision(ctx, desc, "deny", reason, nil)
	return &permissions.PermissionDeniedError{
		Descriptor: desc,
		Message:    reason,
	}
}

func (m *PermissionManager) emitPolicyDecision(ctx context.Context, desc permissions.PermissionDescriptor, effect, reason string, fields map[string]any) {
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
			// Return a redacted sentinel instead of the full path to avoid
			// leaking the sensitive location or its context.
			return "[REDACTED_PATH]"
		}
	}
	return path
}

// log forwards permission decisions to the configured audit sink to provide a
// tamper-evident trail of runtime behavior.
func (m *PermissionManager) log(ctx context.Context, agentID string, desc permissions.PermissionDescriptor, result string, fields map[string]any) {
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
		Metadata:    redactMetadataMap(fields),
		Correlation: agentID,
	}
	_ = m.audit.Log(ctx, record)
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

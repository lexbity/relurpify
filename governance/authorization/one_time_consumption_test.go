package authorization

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/policy"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// countingApprovalProvider is a HITLProvider that counts every request and
// answers per script (approve by default, deny on an explicit error list).
type countingApprovalProvider struct {
	mu      sync.Mutex
	hits    int
	deny    map[string]string // action → denial reason
	approve map[string]policy.GrantScope
}

func (p *countingApprovalProvider) RequestPermission(_ context.Context, req PermissionRequest) (*PermissionGrant, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hits++
	if reason, ok := p.deny[req.Permission.Action]; ok {
		return nil, fmt.Errorf("denied: %s", reason)
	}
	scope := req.Scope
	if s, ok := p.approve[req.Permission.Action]; ok {
		scope = s
	}
	return &PermissionGrant{
		ID:         fmt.Sprintf("grant-%d", p.hits),
		Permission: req.Permission,
		Scope:      scope,
		ApprovedBy: "test",
		GrantedAt:  time.Now().UTC(),
	}, nil
}

func (p *countingApprovalProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hits
}

// TestOneTimeConsumedOnUse is AC-16: a one-time grant authorizes exactly one
// enforcement. The check that obtains the approval caches it; the next
// identical check consumes the cached one-time grant (no new provider hit);
// the check after that requires re-approval. Denial paths never consume.
func TestOneTimeConsumedOnUse(t *testing.T) {
	provider := &countingApprovalProvider{approve: map[string]policy.GrantScope{}}
	pm, err := NewPermissionManager(t.TempDir(), &ucperms.PermissionSet{}, nil, provider)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}

	desc := ucperms.PermissionDescriptor{
		Type:         ucperms.PermissionTypeFilesystem,
		Action:       "write",
		Resource:     "/workspace/file.txt",
		RequiresHITL: true,
	}

	// 1. First check obtains a fresh approval (one provider hit).
	if err := pm.RequireApproval(context.Background(), "agent-1", desc, "step one", policy.GrantScopeOneTime, policy.RiskLevelMedium, 0); err != nil {
		t.Fatalf("first check: %v", err)
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("expected 1 provider hit after first check, got %d", got)
	}

	// 2. Second identical check consumes the cached one-time grant: authorized
	// without a new provider hit.
	if err := pm.RequireApproval(context.Background(), "agent-1", desc, "step two", policy.GrantScopeOneTime, policy.RiskLevelMedium, 0); err != nil {
		t.Fatalf("second check: %v", err)
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("one-time grant must be consumed on use (no new provider hit), got %d", got)
	}

	// 3. Third identical check requires re-approval.
	if err := pm.RequireApproval(context.Background(), "agent-1", desc, "step three", policy.GrantScopeOneTime, policy.RiskLevelMedium, 0); err != nil {
		t.Fatalf("third check: %v", err)
	}
	if got := provider.count(); got != 2 {
		t.Fatalf("third check must re-approve, expected 2 provider hits, got %d", got)
	}
}

// TestOneTimeDenyDoesNotConsume is the R-6 deny invariant: a denied approval
// leaves the grant cache intact (nothing to consume) and every identical
// check re-asks.
func TestOneTimeDenyDoesNotConsume(t *testing.T) {
	provider := &countingApprovalProvider{deny: map[string]string{"write": "policy blocks write"}}
	pm, err := NewPermissionManager(t.TempDir(), &ucperms.PermissionSet{}, nil, provider)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	desc := ucperms.PermissionDescriptor{Type: ucperms.PermissionTypeFilesystem, Action: "write", Resource: "/workspace/file.txt", RequiresHITL: true}

	for i := 0; i < 2; i++ {
		if err := pm.RequireApproval(context.Background(), "agent-1", desc, "denied check", policy.GrantScopeOneTime, policy.RiskLevelMedium, 0); err == nil {
			t.Fatalf("check %d must be denied", i)
		}
	}
	if got := provider.count(); got != 2 {
		t.Fatalf("every denied check must reach the provider, got %d hits", got)
	}
	if pm.grants.Len() != 0 {
		t.Fatalf("denied approvals must not cache grants, got %d cached", pm.grants.Len())
	}
}

// TestSessionScopeGrantSurvivesChecks proves session-scoped grants are reused
// (not consumed), so legitimately repeatable operations keep their approval
// (D14 vocabulary: one-time = one use, session = repeatable).
func TestSessionScopeGrantSurvivesChecks(t *testing.T) {
	provider := &countingApprovalProvider{approve: map[string]policy.GrantScope{}}
	pm, err := NewPermissionManager(t.TempDir(), &ucperms.PermissionSet{}, nil, provider)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	desc := ucperms.PermissionDescriptor{Type: ucperms.PermissionTypeFilesystem, Action: "write", Resource: "/workspace/file.txt", RequiresHITL: true}

	for i := 0; i < 3; i++ {
		if err := pm.RequireApproval(context.Background(), "agent-1", desc, "session check", policy.GrantScopeSession, policy.RiskLevelMedium, 0); err != nil {
			t.Fatalf("session check %d: %v", i, err)
		}
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("session grant must be reused across checks, expected 1 provider hit, got %d", got)
	}
}

// TestEnsureGrantOneTimeConsumedOnUse covers the ensureGrant-served enforcement
// paths (perm_file.go/perm_exec.go/perm_net.go): a cached one-time grant
// authorizes one check and is consumed; the next enforcement re-asks.
func TestEnsureGrantOneTimeConsumedOnUse(t *testing.T) {
	provider := &countingApprovalProvider{approve: map[string]policy.GrantScope{}}
	pm, err := NewPermissionManager(t.TempDir(), &ucperms.PermissionSet{}, nil, provider)
	if err != nil {
		t.Fatalf("NewPermissionManager: %v", err)
	}
	desc := ucperms.PermissionDescriptor{Type: ucperms.PermissionTypeFilesystem, Action: "write", Resource: "/workspace/file.txt", RequiresHITL: true}

	// Seed a cached one-time grant as a prior HITL approval would.
	pm.GrantPermission(desc, "user", policy.GrantScopeOneTime, 0)

	// First enforcement consumes the one-time grant: allowed without a
	// provider hit (the grant did the authorizing).
	if err := pm.ensureGrant(context.Background(), "agent-1", desc); err != nil {
		t.Fatalf("first enforcement: %v", err)
	}
	if got := provider.count(); got != 0 {
		t.Fatalf("one-time grant must authorize the first check without the provider, got %d hits", got)
	}
	// Second enforcement finds the grant consumed and re-asks.
	if err := pm.ensureGrant(context.Background(), "agent-1", desc); err != nil {
		t.Fatalf("second enforcement: %v", err)
	}
	if got := provider.count(); got != 1 {
		t.Fatalf("expected 1 provider hit after consume, got %d", got)
	}
}

package authorization

import (
	"context"
	"testing"
	"time"

	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// TestPermissionManagerCachesEnforceCaps: inserting cap+10 entries into each
// bounded cache leaves exactly cap entries and increments the eviction
// counters (NFR-3).
func TestPermissionManagerCachesEnforceCaps(t *testing.T) {
	m := testPermManager(t, t.TempDir())

	for i := 0; i < grantsCacheCap+10; i++ {
		m.putGrant(grantKey("action", intString(i)), &PermissionGrant{ID: intString(i)})
	}
	if m.grants.Len() != grantsCacheCap {
		t.Fatalf("grants Len() = %d, want %d", m.grants.Len(), grantsCacheCap)
	}
	if m.grants.Evicted() != 10 {
		t.Fatalf("grants Evicted() = %d, want 10", m.grants.Evicted())
	}

	for i := 0; i < hitlRateCacheCap+10; i++ {
		m.hitlRateLimits.Put(intString(i), &hitlRateBucket{count: 1, windowAt: time.Now()})
	}
	if m.hitlRateLimits.Len() != hitlRateCacheCap {
		t.Fatalf("hitlRateLimits Len() = %d, want %d", m.hitlRateLimits.Len(), hitlRateCacheCap)
	}
	if m.hitlRateLimits.Evicted() != 10 {
		t.Fatalf("hitlRateLimits Evicted() = %d, want 10", m.hitlRateLimits.Evicted())
	}

	for i := 0; i < fsPermCacheCap+10; i++ {
		m.fsPermCache.Put(intString(i), nil)
	}
	if m.fsPermCache.Len() != fsPermCacheCap {
		t.Fatalf("fsPermCache Len() = %d, want %d", m.fsPermCache.Len(), fsPermCacheCap)
	}

	for i := 0; i < execPermCacheCap+10; i++ {
		m.execPermCache.Put(intString(i), nil)
	}
	if m.execPermCache.Len() != execPermCacheCap {
		t.Fatalf("execPermCache Len() = %d, want %d", m.execPermCache.Len(), execPermCacheCap)
	}
}

// TestPermissionManagerGrantSweep: sweep-on-insert past half the cap removes
// grants whose own ExpiresAt has elapsed while keeping live ones.
func TestPermissionManagerGrantSweep(t *testing.T) {
	m := testPermManager(t, t.TempDir())
	now := time.Now()
	m.grantClock = func() time.Time { return now }

	stale := &PermissionGrant{ID: "stale", ExpiresAt: now.Add(-time.Minute)}
	fresh := &PermissionGrant{ID: "fresh", ExpiresAt: now.Add(time.Hour)}
	m.putGrant("old:stale", stale)
	for i := 0; i < grantsCacheCap/2; i++ {
		m.putGrant("bulk:"+intString(i), fresh)
	}
	// The next putGrant crosses the sweep threshold and drops the stale grant.
	m.putGrant("after:sweep", fresh)
	if _, ok := m.grants.Get("old:stale"); ok {
		t.Fatal("expired grant survived the sweep")
	}
	if _, ok := m.grants.Get("after:sweep"); !ok {
		t.Fatal("post-sweep grant missing")
	}
}

// TestPermissionManagerReleaseSession: grants issued under a session ID are
// deleted by ReleaseSession; other sessions' grants survive. Idempotent.
func TestPermissionManagerReleaseSession(t *testing.T) {
	m := testPermManager(t, t.TempDir())
	m.putGrant("file:read:a", &PermissionGrant{ID: "a", SessionID: "agent-1"})
	m.putGrant("file:read:b", &PermissionGrant{ID: "b", SessionID: "agent-2"})
	m.putGrant("file:read:unscoped", &PermissionGrant{ID: "u"})

	if released := m.ReleaseSession("agent-1"); released != 1 {
		t.Fatalf("ReleaseSession released %d, want 1", released)
	}
	if _, ok := m.grants.Get("file:read:a"); ok {
		t.Fatal("session grant survived ReleaseSession")
	}
	if _, ok := m.grants.Get("file:read:b"); !ok {
		t.Fatal("other session's grant was released")
	}
	if _, ok := m.grants.Get("file:read:unscoped"); !ok {
		t.Fatal("unscoped grant was released")
	}
	if released := m.ReleaseSession("agent-1"); released != 0 {
		t.Fatalf("ReleaseSession is not idempotent: %d", released)
	}
}

// TestPermissionManagerHITLRateCacheTTL: rate-limit buckets expire after the
// cache TTL, so stale windows cannot pin a key forever.
func TestPermissionManagerHITLRateCacheTTL(t *testing.T) {
	m := testPermManager(t, t.TempDir())
	now := time.Now()
	m.hitlRateLimits.SetClock(func() time.Time { return now })
	m.hitlRateLimits.Put("k", &hitlRateBucket{count: 9, windowAt: now})
	now = now.Add(2 * hitlRateEntryTTL)
	if _, ok := m.hitlRateLimits.Get("k"); ok {
		t.Fatal("expired rate bucket still readable")
	}
	if m.hitlRateLimits.Expired() != 1 {
		t.Fatalf("Expired() = %d, want 1", m.hitlRateLimits.Expired())
	}
}

// TestSessionGrantCarriesSessionID: grants recorded through RequireApproval
// inherit the approving principal's session identity.
func TestSessionGrantCarriesSessionID(t *testing.T) {
	m := testPermManager(t, t.TempDir())
	m.hitl = &autoApproveHITL{}

	ctx := withPrincipalSession(t, "agent-session-7")
	desc := ucperms.PermissionDescriptor{
		Type:     ucperms.PermissionTypeFilesystem,
		Action:   "file:test:grant",
		Resource: "/tmp/x",
	}
	if err := m.RequireApproval(ctx, "agent-session-7", desc, "test", policy.GrantScopeSession, policy.RiskLevelLow, time.Hour); err != nil {
		t.Fatalf("RequireApproval: %v", err)
	}
	grant, ok := m.grants.Get(grantKeyOf(desc))
	if !ok {
		t.Fatal("grant not stored")
	}
	if grant.SessionID != "agent-session-7" {
		t.Fatalf("grant.SessionID = %q, want agent-session-7", grant.SessionID)
	}
}

func grantKey(action, resource string) string { return action + ":" + resource }

func grantKeyOf(desc ucperms.PermissionDescriptor) string {
	return desc.Action + ":" + desc.Resource
}

func intString(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

type autoApproveHITL struct{}

func (autoApproveHITL) RequestPermission(_ context.Context, req PermissionRequest) (*PermissionGrant, error) {
	return &PermissionGrant{
		ID:          req.ID + ":test",
		Permission:  req.Permission,
		Scope:       req.Scope,
		ApprovedBy:  "test",
		GrantedAt:   time.Now(),
		Description: req.Justification,
	}, nil
}

func withPrincipalSession(t *testing.T, agentID string) context.Context {
	t.Helper()
	return governanceports.ContextWithPrincipal(context.Background(), governanceports.Principal{AgentID: agentID})
}

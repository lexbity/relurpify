package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	nexusadmin "github.com/lexcodex/relurpify/app/nexus/admin"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	"github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/framework/core"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	"github.com/stretchr/testify/require"
)

func TestRunStatusPrintsGatewayState(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("gateway:\n  bind: ':8090'\n  path: /gateway\nchannels:\n  webchat:\n    enabled: true\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "relurpify_cfg"), 0o755))

	log, err := db.NewSQLiteEventLog(filepath.Join(dir, "relurpify_cfg", "events.db"))
	require.NoError(t, err)
	defer log.Close()
	_, err = log.Append(context.Background(), "local", []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventSystemStarted,
		Partition: "local",
	}})
	require.NoError(t, err)
	_, err = log.Append(context.Background(), "local", []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventSessionCreated,
		Actor:     core.EventActor{Kind: "agent", ID: "agent-session"},
		Partition: "local",
	}})
	require.NoError(t, err)
	_, err = log.Append(context.Background(), "local", []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   []byte(`{"channel":"webchat"}`),
		Partition: "local",
	}})
	require.NoError(t, err)

	var out bytes.Buffer
	require.NoError(t, runStatus(context.Background(), &out, dir, configPath))
	require.Contains(t, out.String(), "Last seq: 3")
	require.Contains(t, out.String(), "Configured channels: 1")
	require.Contains(t, out.String(), "Active sessions: 1")
	require.Contains(t, out.String(), "Observed channels: 1")
	require.Contains(t, out.String(), "Paired nodes: 0")
	require.Contains(t, out.String(), "Pending pairings: 0")
}

func TestNodePairAndApproveCommandsPersistState(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("gateway:\n  bind: ':8090'\n  path: /gateway\nnodes:\n  auto_approve_local: false\n  pairing_code_ttl: 1h\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "relurpify_cfg"), 0o755))

	workspace := dir
	config := configPath

	pairCmd := newNodePairCmd(&workspace, &config)
	var pairOut bytes.Buffer
	pairCmd.SetOut(&pairOut)
	pairCmd.SetArgs([]string{"--device-id", "node-1"})
	require.NoError(t, pairCmd.ExecuteContext(context.Background()))
	require.Contains(t, pairOut.String(), "Pairing requested for node-1 with code ")

	code := pairOut.String()[len("Pairing requested for node-1 with code ") : len(pairOut.String())-1]

	approveCmd := newNodeApproveCmd(&workspace, &config)
	var approveOut bytes.Buffer
	approveCmd.SetOut(&approveOut)
	approveCmd.SetArgs([]string{"--", code})
	require.NoError(t, approveCmd.ExecuteContext(context.Background()))
	require.Contains(t, approveOut.String(), "approved")

	var statusOut bytes.Buffer
	require.NoError(t, runStatus(context.Background(), &statusOut, dir, configPath))
	require.Contains(t, statusOut.String(), "Paired nodes: 1")
	require.Contains(t, statusOut.String(), "Pending pairings: 0")
}

type compactionSpy struct {
	mu      sync.Mutex
	cutoffs []time.Time
	calls   chan time.Time
}

func (s *compactionSpy) CompactBefore(_ context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	s.cutoffs = append(s.cutoffs, cutoff)
	s.mu.Unlock()
	if s.calls != nil {
		s.calls <- cutoff
	}
	return 0, nil
}

func (s *compactionSpy) snapshot() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.cutoffs...)
}

func TestCompactEventLogUsesRetentionWindow(t *testing.T) {
	base := time.Date(2026, 3, 9, 12, 30, 0, 0, time.UTC)
	spy := &compactionSpy{}

	_, err := compactEventLog(context.Background(), spy, 30, func() time.Time { return base })
	require.NoError(t, err)

	calls := spy.snapshot()
	require.Len(t, calls, 1)
	require.True(t, calls[0].Equal(base.AddDate(0, 0, -30)))
}

func TestStartEventLogCompactorRunsImmediatelyAndOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base := time.Date(2026, 3, 9, 12, 30, 0, 0, time.UTC)
	spy := &compactionSpy{calls: make(chan time.Time, 4)}

	require.NoError(t, startEventLogCompactor(ctx, spy, 7, 10*time.Millisecond, func() time.Time { return base }))

	select {
	case <-spy.calls:
	case <-time.After(time.Second):
		t.Fatal("expected immediate compaction")
	}

	select {
	case <-spy.calls:
	case <-time.After(time.Second):
		t.Fatal("expected periodic compaction")
	}

	calls := spy.snapshot()
	require.GreaterOrEqual(t, len(calls), 2)
	for _, cutoff := range calls[:2] {
		require.True(t, cutoff.Equal(base.AddDate(0, 0, -7)))
	}
}

func TestStaticGatewayPrincipalResolver(t *testing.T) {
	resolver := gatewayPrincipalResolver(nexuscfg.GatewayAuthConfig{
		Enabled: true,
		Tokens: []nexuscfg.GatewayTokenAuth{{
			Token:       "token-1",
			TenantID:    "tenant-1",
			Role:        "agent",
			SubjectKind: string(core.SubjectKindServiceAccount),
			SubjectID:   "svc-1",
			Scopes:      []string{"session:send"},
		}},
	}, nil, nil)
	require.NotNil(t, resolver)

	principal, err := resolver(context.Background(), "token-1")
	require.NoError(t, err)
	require.True(t, principal.Authenticated)
	require.NotNil(t, principal.Principal)
	require.Equal(t, fwgateway.ConnectionPrincipal{
		Role:          "agent",
		Authenticated: true,
		Principal: &core.AuthenticatedPrincipal{
			TenantID:      "tenant-1",
			AuthMethod:    core.AuthMethodBearerToken,
			Authenticated: true,
			Scopes:        []string{"session:send"},
			Subject: core.SubjectRef{
				TenantID: "tenant-1",
				Kind:     core.SubjectKindServiceAccount,
				ID:       "svc-1",
			},
		},
		Actor: core.EventActor{
			Kind:        "agent",
			ID:          "svc-1",
			TenantID:    "tenant-1",
			SubjectKind: core.SubjectKindServiceAccount,
		},
	}, principal)
}

func TestStaticGatewayPrincipalResolverRejectsUnknownToken(t *testing.T) {
	resolver := gatewayPrincipalResolver(nexuscfg.GatewayAuthConfig{
		Enabled: true,
		Tokens: []nexuscfg.GatewayTokenAuth{{
			Token:       "token-1",
			TenantID:    "tenant-1",
			Role:        "agent",
			SubjectKind: string(core.SubjectKindServiceAccount),
			SubjectID:   "svc-1",
		}},
	}, nil, nil)

	_, err := resolver(context.Background(), "token-2")
	require.ErrorContains(t, err, "unknown bearer token")
}

func TestGatewayPrincipalResolverAcceptsIssuedToken(t *testing.T) {
	store, err := db.NewSQLiteAdminTokenStore(filepath.Join(t.TempDir(), "admin_tokens.db"))
	require.NoError(t, err)
	defer store.Close()
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()
	require.NoError(t, identityStore.UpsertTenant(context.Background(), core.TenantRecord{
		ID:        "tenant-issued",
		CreatedAt: time.Now().UTC(),
	}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), core.SubjectRecord{
		TenantID:  "tenant-issued",
		Kind:      core.SubjectKindUser,
		ID:        "subject-1",
		CreatedAt: time.Now().UTC(),
	}))

	require.NoError(t, store.CreateToken(context.Background(), core.AdminTokenRecord{
		ID:          "tok-1",
		Name:        "subject-1",
		TenantID:    "tenant-issued",
		SubjectKind: core.SubjectKindUser,
		SubjectID:   "subject-1",
		TokenHash:   nexusadmin.HashToken("issued-token"),
		Scopes:      []string{"nexus:admin"},
		IssuedAt:    time.Now().UTC(),
	}))

	resolver := gatewayPrincipalResolver(nexuscfg.GatewayAuthConfig{Enabled: true}, store, identityStore)
	principal, err := resolver(context.Background(), "issued-token")
	require.NoError(t, err)
	require.Equal(t, "admin", principal.Role)
	require.NotNil(t, principal.Principal)
	require.Equal(t, "subject-1", principal.Principal.Subject.ID)
	require.Equal(t, "tenant-issued", principal.Principal.TenantID)
	require.Equal(t, core.SubjectKindUser, principal.Principal.Subject.Kind)
	require.Equal(t, []string{"nexus:admin"}, principal.Principal.Scopes)
}

func TestGatewayPrincipalResolverRejectsIssuedTokenWithoutStoredSubject(t *testing.T) {
	store, err := db.NewSQLiteAdminTokenStore(filepath.Join(t.TempDir(), "admin_tokens.db"))
	require.NoError(t, err)
	defer store.Close()
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()
	require.NoError(t, identityStore.UpsertTenant(context.Background(), core.TenantRecord{
		ID:        "tenant-issued",
		CreatedAt: time.Now().UTC(),
	}))

	require.NoError(t, store.CreateToken(context.Background(), core.AdminTokenRecord{
		ID:          "tok-1",
		Name:        "subject-1",
		TenantID:    "tenant-issued",
		SubjectKind: core.SubjectKindUser,
		SubjectID:   "subject-1",
		TokenHash:   nexusadmin.HashToken("issued-token"),
		Scopes:      []string{"nexus:admin"},
		IssuedAt:    time.Now().UTC(),
	}))

	resolver := gatewayPrincipalResolver(nexuscfg.GatewayAuthConfig{Enabled: true}, store, identityStore)
	_, err = resolver(context.Background(), "issued-token")
	require.ErrorContains(t, err, "token subject")
}

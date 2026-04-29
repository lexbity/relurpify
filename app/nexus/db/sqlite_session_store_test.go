package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSQLiteSessionStoreListBoundariesFiltersExpiredRows(t *testing.T) {
	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	store.BoundaryTTL = time.Hour
	store.now = func() time.Time {
		return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	}

	require.NoError(t, store.UpsertBoundary(context.Background(), "expired", &core.SessionBoundary{
		SessionID:      "expired",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		ActorID:        "user-1",
		ChannelID:      "telegram",
		PeerID:         "peer-1",
		TrustClass:     core.TrustClassWorkspaceTrusted,
		CreatedAt:      time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
		LastActivityAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
	}))
	require.NoError(t, store.UpsertBoundary(context.Background(), "active", &core.SessionBoundary{
		SessionID:      "active",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		ActorID:        "user-2",
		ChannelID:      "telegram",
		PeerID:         "peer-2",
		TrustClass:     core.TrustClassWorkspaceTrusted,
		CreatedAt:      time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC),
		LastActivityAt: time.Date(2026, 3, 1, 11, 30, 0, 0, time.UTC),
	}))

	boundaries, err := store.ListBoundaries(context.Background(), "local")
	require.NoError(t, err)
	require.Len(t, boundaries, 1)
	require.Equal(t, "active", boundaries[0].SessionID)

	expired, err := store.GetBoundary(context.Background(), "expired")
	require.NoError(t, err)
	require.Nil(t, expired)
}

func TestSQLiteSessionStoreDeleteExpiredBoundariesRemovesExpiredRows(t *testing.T) {
	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()
	store.BoundaryTTL = 0

	require.NoError(t, store.UpsertBoundary(context.Background(), "expired-1", &core.SessionBoundary{
		SessionID:      "expired-1",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		CreatedAt:      time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
		LastActivityAt: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
	}))
	require.NoError(t, store.UpsertBoundary(context.Background(), "expired-2", &core.SessionBoundary{
		SessionID:      "expired-2",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		CreatedAt:      time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		LastActivityAt: time.Date(2026, 3, 1, 10, 15, 0, 0, time.UTC),
	}))
	require.NoError(t, store.UpsertBoundary(context.Background(), "active", &core.SessionBoundary{
		SessionID:      "active",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		CreatedAt:      time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
		LastActivityAt: time.Date(2026, 3, 1, 11, 45, 0, 0, time.UTC),
	}))

	deleted, err := store.DeleteExpiredBoundaries(context.Background(), time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, 2, deleted)

	boundaries, err := store.ListBoundaries(context.Background(), "local")
	require.NoError(t, err)
	require.Len(t, boundaries, 1)
	require.Equal(t, "active", boundaries[0].SessionID)
}

func TestSQLiteSessionStorePersistsTenantOwnerAndBinding(t *testing.T) {
	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	require.NoError(t, store.UpsertBoundary(context.Background(), "bound", &core.SessionBoundary{
		SessionID:      "session-1",
		RoutingKey:     "bound",
		TenantID:       "tenant-1",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		ActorID:        "legacy-actor",
		Owner:          core.DelegationSubjectRef{TenantID: "tenant-1", Kind: string(core.SubjectKindUser), ID: "user-1"},
		ChannelID:      "discord",
		PeerID:         "conv-1",
		Binding:        &core.SessionBinding{Provider: "discord", ProviderID: "guild-1", AccountID: "guild-1", ChannelID: "channel-1", ConversationID: "conv-1", ThreadID: "thread-1", ExternalUserID: "discord-user-1"},
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      now,
		LastActivityAt: now,
	}))

	got, err := store.GetBoundary(context.Background(), "bound")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "bound", got.RoutingKey)
	require.Equal(t, "tenant-1", got.TenantID)
	require.Equal(t, string(core.SubjectKindUser), got.Owner.Kind)
	require.Empty(t, got.ActorID)
	require.NotNil(t, got.Binding)
	require.Equal(t, "guild-1", got.Binding.AccountID)
}

func TestSQLiteSessionStoreGetBoundaryBySessionID(t *testing.T) {
	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	require.NoError(t, store.UpsertBoundary(context.Background(), "local:discord:channel-1", &core.SessionBoundary{
		SessionID:      "sess_opaque_1",
		RoutingKey:     "local:discord:channel-1",
		TenantID:       "tenant-1",
		Partition:      "local",
		Scope:          core.SessionScopePerChannelPeer,
		ChannelID:      "discord",
		PeerID:         "channel-1",
		TrustClass:     core.TrustClassRemoteApproved,
		CreatedAt:      now,
		LastActivityAt: now,
	}))

	got, err := store.GetBoundaryBySessionID(context.Background(), "sess_opaque_1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "local:discord:channel-1", got.RoutingKey)
	require.Equal(t, "sess_opaque_1", got.SessionID)
}

func TestSQLiteSessionStoreMissingBoundaryLookups(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	got, err := store.GetBoundary(context.Background(), "missing")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = store.GetBoundaryBySessionID(context.Background(), "missing")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSQLiteSessionStorePersistsSessionDelegations(t *testing.T) {
	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.UpsertDelegation(context.Background(), core.SessionDelegationRecord{
		TenantID:  "tenant-1",
		SessionID: "sess_opaque_1",
		Grantee: core.DelegationSubjectRef{
			TenantID: "tenant-1",
			Kind:     string(core.SubjectKindServiceAccount),
			ID:       "operator-1",
		},
		Operations: []core.SessionOperation{core.SessionOperationSend, core.SessionOperationResume},
		CreatedAt:  now,
	}))

	records, err := store.ListDelegationsBySessionID(context.Background(), "sess_opaque_1")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "operator-1", records[0].Grantee.ID)
	require.Len(t, records[0].Operations, 2)
}

func TestSQLiteSessionStoreListDelegationsByTenantIDFiltersTenants(t *testing.T) {
	store, err := NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.UpsertDelegation(context.Background(), core.SessionDelegationRecord{
		TenantID:   "tenant-1",
		SessionID:  "sess-t1-a",
		Grantee:    core.DelegationSubjectRef{TenantID: "tenant-1", Kind: string(core.SubjectKindServiceAccount), ID: "op-1"},
		Operations: []core.SessionOperation{core.SessionOperationSend},
		CreatedAt:  now,
	}))
	require.NoError(t, store.UpsertDelegation(context.Background(), core.SessionDelegationRecord{
		TenantID:   "tenant-2",
		SessionID:  "sess-t2-b",
		Grantee:    core.DelegationSubjectRef{TenantID: "tenant-2", Kind: string(core.SubjectKindServiceAccount), ID: "op-2"},
		Operations: []core.SessionOperation{core.SessionOperationResume},
		CreatedAt:  now,
	}))

	records, err := store.ListDelegationsByTenantID(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "sess-t1-a", records[0].SessionID)
	require.Equal(t, "op-1", records[0].Grantee.ID)

	records2, err := store.ListDelegationsByTenantID(context.Background(), "tenant-2")
	require.NoError(t, err)
	require.Len(t, records2, 1)
	require.Equal(t, "sess-t2-b", records2[0].SessionID)
}

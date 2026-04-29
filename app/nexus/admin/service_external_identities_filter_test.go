package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestListExternalIdentitiesSubjectKindFilter(t *testing.T) {
	t.Parallel()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, identityStore.UpsertExternalIdentity(ctx, core.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   identity.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "discord-user-1",
		Subject:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		VerifiedAt: now,
		LastSeenAt: now,
	}))
	require.NoError(t, identityStore.UpsertExternalIdentity(ctx, core.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   identity.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "discord-bot-1",
		Subject:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "bot-1"},
		VerifiedAt: now,
		LastSeenAt: now,
	}))

	svc := NewService(ServiceConfig{Identities: identityStore}).(*service)

	// Filter by subject kind — only users
	result, err := svc.ListExternalIdentities(ctx, ListExternalIdentitiesRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
		SubjectKind: core.SubjectKindUser,
	})
	require.NoError(t, err)
	require.Len(t, result.Identities, 1)
	require.Equal(t, "discord-user-1", result.Identities[0].ExternalID)
}

func TestListExternalIdentitiesSubjectIDFilter(t *testing.T) {
	t.Parallel()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, identityStore.UpsertExternalIdentity(ctx, core.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   identity.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "ext-a",
		Subject:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-alice"},
		VerifiedAt: now,
		LastSeenAt: now,
	}))
	require.NoError(t, identityStore.UpsertExternalIdentity(ctx, core.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   identity.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "ext-b",
		Subject:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-bob"},
		VerifiedAt: now,
		LastSeenAt: now,
	}))

	svc := NewService(ServiceConfig{Identities: identityStore}).(*service)

	result, err := svc.ListExternalIdentities(ctx, ListExternalIdentitiesRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
		SubjectID: "user-alice",
	})
	require.NoError(t, err)
	require.Len(t, result.Identities, 1)
	require.Equal(t, "ext-a", result.Identities[0].ExternalID)
}

func TestListExternalIdentitiesNoFilterReturnsAll(t *testing.T) {
	t.Parallel()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	for i, extID := range []string{"ext-1", "ext-2", "ext-3"} {
		require.NoError(t, identityStore.UpsertExternalIdentity(ctx, core.ExternalIdentity{
			TenantID:   "tenant-1",
			Provider:   identity.ExternalProviderDiscord,
			AccountID:  "guild-1",
			ExternalID: extID,
			Subject:    core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: extID},
			VerifiedAt: now.Add(time.Duration(i) * time.Second),
			LastSeenAt: now.Add(time.Duration(i) * time.Second),
		}))
	}

	svc := NewService(ServiceConfig{Identities: identityStore}).(*service)

	result, err := svc.ListExternalIdentities(ctx, ListExternalIdentitiesRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Identities, 3)
}

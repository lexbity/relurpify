package identity_test

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/identity"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
	"github.com/stretchr/testify/require"
)

func TestStoreResolverResolveInbound(t *testing.T) {
	store, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   core.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "discord-user-1",
		Subject: core.SubjectRef{
			TenantID: "tenant-1",
			Kind:     core.SubjectKindUser,
			ID:       "user-1",
		},
	}))

	resolution, err := (identity.StoreResolver{
		Store:           store,
		DefaultTenantID: "tenant-1",
	}).ResolveInbound(context.Background(), channel.InboundMessage{
		Channel: "discord",
		Account: "guild-1",
		Sender: channel.Identity{
			ChannelID: "discord-user-1",
		},
		Conversation: channel.Conversation{ID: "channel-1"},
	})
	require.NoError(t, err)
	require.True(t, resolution.Resolved())
	require.Equal(t, "user-1", resolution.Owner.ID)
	require.NotNil(t, resolution.Binding)
	require.Equal(t, core.ExternalProviderDiscord, resolution.Binding.Provider)
}

func TestStoreResolverLeavesUnknownExternalIdentityUnbound(t *testing.T) {
	resolution, err := (identity.StoreResolver{
		DefaultTenantID: "tenant-1",
	}).ResolveInbound(context.Background(), channel.InboundMessage{
		Channel: "discord",
		Account: "guild-1",
		Sender: channel.Identity{
			ChannelID: "discord-user-missing",
		},
		Conversation: channel.Conversation{ID: "channel-1"},
	})
	require.NoError(t, err)
	require.True(t, resolution.Unbound())
	require.Equal(t, "tenant-1", resolution.TenantID)
	require.NotNil(t, resolution.Binding)
	require.Equal(t, core.ExternalProviderDiscord, resolution.Binding.Provider)
}

func TestStoreResolverFallsBackToTenantScopedBindingOutsideDefaultTenant(t *testing.T) {
	store, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.UpsertTenant(context.Background(), core.TenantRecord{ID: "tenant-a"}))
	require.NoError(t, store.UpsertSubject(context.Background(), core.SubjectRecord{
		TenantID: "tenant-a",
		Kind:     core.SubjectKindUser,
		ID:       "user-a",
	}))
	require.NoError(t, store.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:   "tenant-a",
		Provider:   core.ExternalProviderWebchat,
		AccountID:  "workspace-a",
		ExternalID: "peer-a",
		Subject: core.SubjectRef{
			TenantID: "tenant-a",
			Kind:     core.SubjectKindUser,
			ID:       "user-a",
		},
	}))

	resolution, err := (identity.StoreResolver{
		Store:           store,
		DefaultTenantID: "local",
	}).ResolveInbound(context.Background(), channel.InboundMessage{
		Channel: "webchat",
		Account: "workspace-a",
		Sender: channel.Identity{
			ChannelID: "peer-a",
		},
		Conversation: channel.Conversation{ID: "conv-1"},
	})
	require.NoError(t, err)
	require.True(t, resolution.Resolved())
	require.Equal(t, "tenant-a", resolution.TenantID)
	require.Equal(t, "user-a", resolution.Owner.ID)
}

package identity_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
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
	require.True(t, resolution.Resolved)
	require.Equal(t, "user-1", resolution.Owner.ID)
	require.NotNil(t, resolution.Binding)
	require.Equal(t, core.ExternalProviderDiscord, resolution.Binding.Provider)
}

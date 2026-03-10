package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSQLiteIdentityStoreExternalIdentityRoundTrip(t *testing.T) {
	store, err := NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	identity := core.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   core.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "user-42",
		Subject: core.SubjectRef{
			TenantID: "tenant-1",
			Kind:     core.SubjectKindUser,
			ID:       "internal-user-1",
		},
		VerifiedAt:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		LastSeenAt:    time.Date(2026, 3, 10, 12, 5, 0, 0, time.UTC),
		DisplayName:   "Lex",
		ProviderLabel: "discord-main",
	}
	require.NoError(t, store.UpsertExternalIdentity(context.Background(), identity))

	got, err := store.GetExternalIdentity(context.Background(), "tenant-1", core.ExternalProviderDiscord, "guild-1", "user-42")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, identity.Subject.ID, got.Subject.ID)
	require.Equal(t, identity.DisplayName, got.DisplayName)

	list, err := store.ListExternalIdentities(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "user-42", list[0].ExternalID)
}

func TestSQLiteIdentityStoreNodeEnrollmentRoundTrip(t *testing.T) {
	store, err := NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer store.Close()

	enrollment := core.NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-1",
		TrustClass: core.TrustClassWorkspaceTrusted,
		Owner: core.SubjectRef{
			TenantID: "tenant-1",
			Kind:     core.SubjectKindNode,
			ID:       "node-1",
		},
		PublicKey:      []byte("public-key"),
		KeyID:          "k1",
		PairedAt:       time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		LastVerifiedAt: time.Date(2026, 3, 10, 11, 30, 0, 0, time.UTC),
		AuthMethod:     core.AuthMethodNodeChallenge,
	}
	require.NoError(t, store.UpsertNodeEnrollment(context.Background(), enrollment))

	got, err := store.GetNodeEnrollment(context.Background(), "tenant-1", "node-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, enrollment.KeyID, got.KeyID)
	require.Equal(t, enrollment.AuthMethod, got.AuthMethod)

	list, err := store.ListNodeEnrollments(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "node-1", list[0].NodeID)
}

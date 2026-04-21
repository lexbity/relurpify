package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSQLiteTrustBundleStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteTrustBundleStore(filepath.Join(t.TempDir(), "trust_bundles.db"))
	require.NoError(t, err)
	defer store.Close()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.UpsertTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []core.SubjectRef{{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "gw-1"}},
		TrustAnchors:      []string{"cert-a"},
		RecipientKeys: []core.RecipientKeyAdvertisement{{
			Recipient: "runtime://mesh.remote/node-1/rt-1",
			KeyID:     "key-1",
			Version:   "v1",
			PublicKey: []byte("0123456789abcdef0123456789abcdef"),
			Active:    true,
			ExpiresAt: now.Add(time.Hour),
		}},
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	}))

	record, err := store.GetTrustBundle(context.Background(), "mesh.remote")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, "bundle-1", record.BundleID)
	require.Len(t, record.RecipientKeys, 1)
	require.Equal(t, "key-1", record.RecipientKeys[0].KeyID)

	list, err := store.ListTrustBundles(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "mesh.remote", list[0].TrustDomain)
	require.Len(t, list[0].RecipientKeys, 1)
}

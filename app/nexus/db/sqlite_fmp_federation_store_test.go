package db

import (
	"context"
	"path/filepath"
	"testing"

	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"github.com/stretchr/testify/require"
)

func TestSQLiteFMPFederationStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteFMPFederationStore(filepath.Join(t.TempDir(), "fmp_federation.db"))
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.SetTenantFederationPolicy(context.Background(), fwfmp.TenantFederationPolicy{
		TenantID:            "tenant-1",
		AllowedTrustDomains: []string{"mesh.remote", "mesh.backup"},
		AllowedRouteModes:   []fwfmp.RouteMode{fwfmp.RouteModeGateway},
		AllowMediation:      true,
		MaxTransferBytes:    2048,
	}))

	policy, err := store.GetTenantFederationPolicy(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.NotNil(t, policy)
	require.Equal(t, "tenant-1", policy.TenantID)
	require.ElementsMatch(t, []string{"mesh.remote", "mesh.backup"}, policy.AllowedTrustDomains)
	require.Equal(t, []fwfmp.RouteMode{fwfmp.RouteModeGateway}, policy.AllowedRouteModes)
	require.True(t, policy.AllowMediation)
	require.Equal(t, int64(2048), policy.MaxTransferBytes)
}

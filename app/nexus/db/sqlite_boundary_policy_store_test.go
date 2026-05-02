package db

import (
	"context"
	"path/filepath"
	"testing"

	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestSQLiteBoundaryPolicyStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteBoundaryPolicyStore(filepath.Join(t.TempDir(), "boundary_policies.db"))
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.UpsertBoundaryPolicy(context.Background(), fwfmp.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AcceptedSourceIdentities:     []identity.SubjectRef{{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "gw-1"}},
		AllowedRouteModes:            []fwfmp.RouteMode{fwfmp.RouteModeGateway},
		RequireGatewayAuthentication: true,
		AllowMediation:               true,
		MaxTransferBytes:             4096,
		MaxRetries:                   3,
		RetryBackoffSeconds:          30,
	}))

	record, err := store.GetBoundaryPolicy(context.Background(), "mesh.remote")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.True(t, record.AllowMediation)

	list, err := store.ListBoundaryPolicies(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, int64(4096), list[0].MaxTransferBytes)
}

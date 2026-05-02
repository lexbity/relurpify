package admin

import (
	"context"
	"testing"
	"time"

	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestListFMPTrustBundlesRequiresGlobalAdmin(t *testing.T) {
	t.Parallel()

	svc := NewService(ServiceConfig{
		FMP: &fwfmp.Service{Trust: &fwfmp.InMemoryTrustBundleStore{}},
	}).(*service)
	_, err := svc.ListFMPTrustBundles(context.Background(), ListFMPTrustBundlesRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "admin"},
			},
			TenantID: "tenant-1",
		},
	})
	require.Error(t, err)
	var adminErr AdminError
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorPolicyDenied, adminErr.Code)
}

func TestUpsertAndListFMPTrustBundles(t *testing.T) {
	t.Parallel()

	mesh := &fwfmp.Service{Trust: &fwfmp.InMemoryTrustBundleStore{}}
	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	bundle := fwfmp.TrustBundle{
		TrustDomain:       "mesh.remote",
		BundleID:          "bundle-1",
		GatewayIdentities: []identity.SubjectRef{{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "gw-1"}},
		IssuedAt:          now,
		ExpiresAt:         now.Add(time.Hour),
	}
	_, err := svc.UpsertFMPTrustBundle(context.Background(), UpsertFMPTrustBundleRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
		Bundle: bundle,
	})
	require.NoError(t, err)

	result, err := svc.ListFMPTrustBundles(context.Background(), ListFMPTrustBundlesRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Bundles, 1)
	require.Equal(t, "mesh.remote", result.Bundles[0].TrustDomain)
}

func TestSetAndListFMPBoundaryPolicies(t *testing.T) {
	t.Parallel()

	mesh := &fwfmp.Service{Boundaries: &fwfmp.InMemoryBoundaryPolicyStore{}}
	svc := NewService(ServiceConfig{FMP: mesh}).(*service)
	policy := fwfmp.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AllowedRouteModes:            []fwfmp.RouteMode{fwfmp.RouteModeGateway, fwfmp.RouteModeMediated},
		RequireGatewayAuthentication: true,
		AllowMediation:               true,
		MaxTransferBytes:             4096,
	}
	_, err := svc.SetFMPBoundaryPolicy(context.Background(), SetFMPBoundaryPolicyRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
		Policy: policy,
	})
	require.NoError(t, err)

	result, err := svc.ListFMPBoundaryPolicies(context.Background(), ListFMPBoundaryPoliciesRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Policies, 1)
	require.True(t, result.Policies[0].AllowMediation)
	require.Equal(t, int64(4096), result.Policies[0].MaxTransferBytes)
}

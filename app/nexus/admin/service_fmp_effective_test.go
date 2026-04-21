package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"github.com/stretchr/testify/require"
)

func TestGetEffectiveFMPFederationPolicyCombinesTenantAndBoundaryControls(t *testing.T) {
	t.Parallel()

	federationStore, err := db.NewSQLiteFMPFederationStore(filepath.Join(t.TempDir(), "fmp_federation.db"))
	require.NoError(t, err)
	defer federationStore.Close()
	require.NoError(t, federationStore.SetTenantFederationPolicy(context.Background(), core.TenantFederationPolicy{
		TenantID:            "tenant-1",
		AllowedTrustDomains: []string{"mesh.remote"},
		AllowedRouteModes:   []core.RouteMode{core.RouteModeGateway, core.RouteModeMediated},
		AllowMediation:      true,
		MaxTransferBytes:    2048,
	}))

	mesh := &fwfmp.Service{
		Trust:      &fwfmp.InMemoryTrustBundleStore{},
		Boundaries: &fwfmp.InMemoryBoundaryPolicyStore{},
	}
	require.NoError(t, mesh.RegisterTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain: "mesh.remote",
		BundleID:    "bundle-1",
		IssuedAt:    time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
		ExpiresAt:   time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC),
	}))
	require.NoError(t, mesh.SetBoundaryPolicy(context.Background(), core.BoundaryPolicy{
		TrustDomain:                  "mesh.remote",
		AcceptedSourceDomains:        []string{"mesh.remote"},
		AllowedRouteModes:            []core.RouteMode{core.RouteModeGateway},
		RequireGatewayAuthentication: true,
		MaxTransferBytes:             1024,
	}))

	svc := NewService(ServiceConfig{
		FMP:           mesh,
		FMPFederation: federationStore,
	}).(*service)
	result, err := svc.GetEffectiveFMPFederationPolicy(context.Background(), GetEffectiveFMPFederationPolicyRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "observer"},
			},
			TenantID: "tenant-1",
		},
		TrustDomain: "mesh.remote",
	})
	require.NoError(t, err)
	require.True(t, result.Policy.TrustBundlePresent)
	require.True(t, result.Policy.BoundaryPolicyPresent)
	require.True(t, result.Policy.AllowedTrustDomain)
	require.Equal(t, []string{string(core.RouteModeGateway)}, result.Policy.AllowedRouteModes)
	require.False(t, result.Policy.AllowMediation)
	require.Equal(t, int64(1024), result.Policy.MaxTransferBytes)
	require.True(t, result.Policy.RequireGatewayAuth)
	require.Equal(t, []string{"mesh.remote"}, result.Policy.AcceptedSourceDomains)
}

func TestGetEffectiveFMPFederationPolicyDeniesCrossTenantAccess(t *testing.T) {
	t.Parallel()

	svc := NewService(ServiceConfig{}).(*service)
	_, err := svc.GetEffectiveFMPFederationPolicy(context.Background(), GetEffectiveFMPFederationPolicyRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:observer"},
				Subject:       core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "observer"},
			},
			TenantID: "tenant-2",
		},
		TrustDomain: "mesh.remote",
	})
	require.Error(t, err)
	var adminErr AdminError
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorPolicyDenied, adminErr.Code)
}

package admin

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestSetAndGetTenantFMPFederationPolicy(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteFMPFederationStore(filepath.Join(t.TempDir(), "fmp_federation.db"))
	require.NoError(t, err)
	defer store.Close()

	svc := NewService(ServiceConfig{FMPFederation: store}).(*service)
	_, err = svc.SetTenantFMPFederationPolicy(context.Background(), SetTenantFMPFederationPolicyRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "admin"},
			},
			TenantID: "tenant-1",
		},
		AllowedTrustDomains: []string{"mesh.remote", "mesh.backup"},
		AllowedRouteModes:   []string{string(core.RouteModeGateway)},
		AllowMediation:      true,
		MaxTransferBytes:    2048,
	})
	require.NoError(t, err)

	result, err := svc.GetTenantFMPFederationPolicy(context.Background(), GetTenantFMPFederationPolicyRequest{
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
	require.NoError(t, err)
	require.Equal(t, "tenant-1", result.Policy.TenantID)
	require.Equal(t, []string{"mesh.backup", "mesh.remote"}, result.Policy.AllowedTrustDomains)
	require.Equal(t, []string{string(core.RouteModeGateway)}, result.Policy.AllowedRouteModes)
	require.True(t, result.Policy.AllowMediation)
	require.Equal(t, int64(2048), result.Policy.MaxTransferBytes)
}

func TestGetTenantFMPFederationPolicyDeniesCrossTenantAccess(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteFMPFederationStore(filepath.Join(t.TempDir(), "fmp_federation.db"))
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.SetTenantFederationPolicy(context.Background(), fwfmp.TenantFederationPolicy{
		TenantID:            "tenant-2",
		AllowedTrustDomains: []string{"mesh.remote"},
	}))

	svc := NewService(ServiceConfig{FMPFederation: store}).(*service)
	_, err = svc.GetTenantFMPFederationPolicy(context.Background(), GetTenantFMPFederationPolicyRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "admin"},
			},
			TenantID: "tenant-2",
		},
	})
	require.Error(t, err)
	var adminErr AdminError
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorPolicyDenied, adminErr.Code)
}

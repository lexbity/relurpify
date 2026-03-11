package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func newTenantTestSvc(t *testing.T) (*service, *db.SQLiteIdentityStore) {
	t.Helper()
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	t.Cleanup(func() { identityStore.Close() })
	return NewService(ServiceConfig{Identities: identityStore}).(*service), identityStore
}

func TestGetTenantReturnsTenantInfo(t *testing.T) {
	t.Parallel()
	svc, store := newTenantTestSvc(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.UpsertTenant(ctx, core.TenantRecord{
		ID:          "acme",
		DisplayName: "Acme Corp",
		CreatedAt:   now,
	}))

	result, err := svc.GetTenant(ctx, GetTenantRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("acme"),
			TenantID:  "acme",
		},
		TenantLookupID: "acme",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Tenant)
	require.Equal(t, "acme", result.Tenant.ID)
	require.Equal(t, "Acme Corp", result.Tenant.DisplayName)
	require.Nil(t, result.Tenant.DisabledAt)
}

func TestGetTenantNotFoundReturnsError(t *testing.T) {
	t.Parallel()
	svc, _ := newTenantTestSvc(t)

	_, err := svc.GetTenant(context.Background(), GetTenantRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("missing"),
			TenantID:  "missing",
		},
		TenantLookupID: "missing",
	})
	require.Error(t, err)
	var ae AdminError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, AdminErrorNotFound, ae.Code)
}

func TestSetTenantEnabledDisablesTenant(t *testing.T) {
	t.Parallel()
	svc, store := newTenantTestSvc(t)
	ctx := context.Background()

	require.NoError(t, store.UpsertTenant(ctx, core.TenantRecord{
		ID:        "tenant-x",
		CreatedAt: time.Now().UTC(),
	}))

	result, err := svc.SetTenantEnabled(ctx, SetTenantEnabledRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-x"),
			TenantID:  "tenant-x",
		},
		TenantLookupID: "tenant-x",
		Enabled:        false,
	})
	require.NoError(t, err)
	require.Equal(t, "tenant-x", result.TenantLookupID)
	require.False(t, result.Enabled)

	record, err := store.GetTenant(ctx, "tenant-x")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.NotNil(t, record.DisabledAt)
}

func TestSetTenantEnabledReenablesTenant(t *testing.T) {
	t.Parallel()
	svc, store := newTenantTestSvc(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	disabledAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	require.NoError(t, store.UpsertTenant(ctx, core.TenantRecord{
		ID:         "tenant-y",
		CreatedAt:  createdAt,
		DisabledAt: &disabledAt,
	}))

	result, err := svc.SetTenantEnabled(ctx, SetTenantEnabledRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-y"),
			TenantID:  "tenant-y",
		},
		TenantLookupID: "tenant-y",
		Enabled:        true,
	})
	require.NoError(t, err)
	require.True(t, result.Enabled)

	record, err := store.GetTenant(ctx, "tenant-y")
	require.NoError(t, err)
	require.Nil(t, record.DisabledAt)
}

func TestIssueTokenBlockedForDisabledTenant(t *testing.T) {
	t.Parallel()
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	tokenStore, err := db.NewSQLiteAdminTokenStore(filepath.Join(t.TempDir(), "tokens.db"))
	require.NoError(t, err)
	defer tokenStore.Close()

	ctx := context.Background()
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	disabledAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, identityStore.UpsertTenant(ctx, core.TenantRecord{
		ID:         "disabled-tenant",
		CreatedAt:  createdAt,
		DisabledAt: &disabledAt,
	}))
	require.NoError(t, identityStore.UpsertSubject(ctx, core.SubjectRecord{
		TenantID:  "disabled-tenant",
		Kind:      core.SubjectKindServiceAccount,
		ID:        "svc-1",
		CreatedAt: time.Now().UTC(),
	}))

	svc := NewService(ServiceConfig{
		Identities: identityStore,
		Tokens:     tokenStore,
	}).(*service)

	_, err = svc.IssueToken(ctx, IssueTokenRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("disabled-tenant"),
			TenantID:  "disabled-tenant",
		},
		SubjectKind: core.SubjectKindServiceAccount,
		SubjectID:   "svc-1",
	})
	require.Error(t, err)
	var ae AdminError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, AdminErrorInvalidArgument, ae.Code)
}

func TestListTenantsReturnsDisplayNameAndDisabledAt(t *testing.T) {
	t.Parallel()
	svc, store := newTenantTestSvc(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	disabledAt := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.UpsertTenant(ctx, core.TenantRecord{
		ID:          "alpha",
		DisplayName: "Alpha Team",
		CreatedAt:   createdAt,
		DisabledAt:  &disabledAt,
	}))
	require.NoError(t, store.UpsertTenant(ctx, core.TenantRecord{
		ID:          "beta",
		DisplayName: "Beta Team",
		CreatedAt:   createdAt,
	}))

	result, err := svc.ListTenants(ctx, ListTenantsRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "",
				Authenticated: true,
				Scopes:        []string{"nexus:admin", "nexus:admin:global"},
				Subject:       core.SubjectRef{Kind: core.SubjectKindServiceAccount, ID: "admin"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Tenants, 2)
	require.Equal(t, "alpha", result.Tenants[0].ID)
	require.Equal(t, "Alpha Team", result.Tenants[0].DisplayName)
	require.NotNil(t, result.Tenants[0].DisabledAt)
	require.Equal(t, "beta", result.Tenants[1].ID)
	require.Nil(t, result.Tenants[1].DisabledAt)
}

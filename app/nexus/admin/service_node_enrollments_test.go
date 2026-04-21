package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func newEnrollmentTestSvc(t *testing.T) (*service, *db.SQLiteIdentityStore) {
	t.Helper()
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	t.Cleanup(func() { identityStore.Close() })
	return NewService(ServiceConfig{Identities: identityStore}).(*service), identityStore
}

func globalAdminPrincipal(tenantID string) core.AuthenticatedPrincipal {
	return core.AuthenticatedPrincipal{
		TenantID:      tenantID,
		Authenticated: true,
		Scopes:        []string{"nexus:admin", "nexus:admin:global"},
		Subject:       core.SubjectRef{TenantID: tenantID, Kind: core.SubjectKindServiceAccount, ID: "admin"},
	}
}

func TestListNodeEnrollmentsReturnsEnrollmentsForTenant(t *testing.T) {
	t.Parallel()
	svc, store := newEnrollmentTestSvc(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.UpsertNodeEnrollment(ctx, core.NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-a",
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-a"},
		TrustClass: core.TrustClassRemoteApproved,
		PublicKey:  []byte("pk-a"),
		PairedAt:   now,
	}))
	require.NoError(t, store.UpsertNodeEnrollment(ctx, core.NodeEnrollment{
		TenantID:   "tenant-2",
		NodeID:     "node-b",
		Owner:      core.SubjectRef{TenantID: "tenant-2", Kind: core.SubjectKindNode, ID: "node-b"},
		TrustClass: core.TrustClassRemoteApproved,
		PublicKey:  []byte("pk-b"),
		PairedAt:   now,
	}))

	result, err := svc.ListNodeEnrollments(ctx, ListNodeEnrollmentsRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Enrollments, 1)
	require.Equal(t, "node-a", result.Enrollments[0].NodeID)
}

func TestRevokeNodeEnrollmentDeletesEnrollment(t *testing.T) {
	t.Parallel()
	svc, store := newEnrollmentTestSvc(t)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, store.UpsertNodeEnrollment(ctx, core.NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-x",
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindNode, ID: "node-x"},
		TrustClass: core.TrustClassRemoteApproved,
		PublicKey:  []byte("pk-x"),
		PairedAt:   now,
	}))

	result, err := svc.RevokeNodeEnrollment(ctx, RevokeNodeEnrollmentRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
		NodeID: "node-x",
	})
	require.NoError(t, err)
	require.Equal(t, "node-x", result.NodeID)

	gone, err := store.GetNodeEnrollment(ctx, "tenant-1", "node-x")
	require.NoError(t, err)
	require.Nil(t, gone)
}

func TestRevokeNodeEnrollmentNotFoundReturnsError(t *testing.T) {
	t.Parallel()
	svc, _ := newEnrollmentTestSvc(t)

	_, err := svc.RevokeNodeEnrollment(context.Background(), RevokeNodeEnrollmentRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
		NodeID: "missing-node",
	})
	require.Error(t, err)
	var ae AdminError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, AdminErrorNotFound, ae.Code)
}

func TestListNodeEnrollmentsNoIdentityStoreReturnsEmpty(t *testing.T) {
	t.Parallel()
	svc := NewService(ServiceConfig{}).(*service)

	result, err := svc.ListNodeEnrollments(context.Background(), ListNodeEnrollmentsRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Enrollments)
}

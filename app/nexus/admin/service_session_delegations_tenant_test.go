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

func TestListSessionDelegationsReturnsDelegationsForTenant(t *testing.T) {
	t.Parallel()

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer sessionStore.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, sessionStore.UpsertDelegation(ctx, core.SessionDelegationRecord{
		TenantID:  "tenant-1",
		SessionID: "sess-1",
		Grantee:   core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "operator-1"},
		Operations: []core.SessionOperation{core.SessionOperationSend},
		CreatedAt: now,
	}))
	require.NoError(t, sessionStore.UpsertDelegation(ctx, core.SessionDelegationRecord{
		TenantID:  "tenant-2",
		SessionID: "sess-2",
		Grantee:   core.SubjectRef{TenantID: "tenant-2", Kind: core.SubjectKindServiceAccount, ID: "operator-2"},
		Operations: []core.SessionOperation{core.SessionOperationResume},
		CreatedAt: now,
	}))

	svc := NewService(ServiceConfig{Sessions: sessionStore}).(*service)

	result, err := svc.ListSessionDelegations(ctx, ListSessionDelegationsRequest{
		AdminRequest: AdminRequest{
			Principal: globalAdminPrincipal("tenant-1"),
			TenantID:  "tenant-1",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Delegations, 1)
	require.Equal(t, "sess-1", result.Delegations[0].SessionID)
	require.Equal(t, "operator-1", result.Delegations[0].Grantee.ID)
}

func TestListSessionDelegationsUnauthorizedTenantReturnsError(t *testing.T) {
	t.Parallel()

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer sessionStore.Close()

	svc := NewService(ServiceConfig{Sessions: sessionStore}).(*service)

	_, err = svc.ListSessionDelegations(context.Background(), ListSessionDelegationsRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "admin"},
			},
			TenantID: "tenant-2",
		},
	})
	require.Error(t, err)
	var ae AdminError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, AdminErrorPolicyDenied, ae.Code)
}

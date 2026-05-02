package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestGrantSessionDelegation(t *testing.T) {
	t.Parallel()

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer sessionStore.Close()

	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()

	require.NoError(t, identityStore.UpsertTenant(context.Background(), identity.TenantRecord{ID: "tenant-1", CreatedAt: time.Now().UTC()}))
	require.NoError(t, identityStore.UpsertSubject(context.Background(), identity.SubjectRecord{
		TenantID:  "tenant-1",
		Kind:      identity.SubjectKindServiceAccount,
		ID:        "operator-1",
		CreatedAt: time.Now().UTC(),
	}))
	require.NoError(t, sessionStore.UpsertBoundary(context.Background(), "tenant-1:webchat:conv-1", &core.SessionBoundary{
		SessionID:  "sess-1",
		RoutingKey: "tenant-1:webchat:conv-1",
		TenantID:   "tenant-1",
		Partition:  "local",
		Scope:      core.SessionScopePerChannelPeer,
		ChannelID:  "webchat",
		PeerID:     "conv-1",
		Owner:      core.DelegationSubjectRef{TenantID: "tenant-1", Kind: string(identity.SubjectKindUser), ID: "user-1"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	}))

	svc := NewService(ServiceConfig{
		Sessions:   sessionStore,
		Identities: identityStore,
	}).(*service)
	result, err := svc.GrantSessionDelegation(context.Background(), GrantSessionDelegationRequest{
		AdminRequest: AdminRequest{
			Principal: identity.AuthenticatedPrincipal{
				TenantID:      "tenant-1",
				Authenticated: true,
				Scopes:        []string{"nexus:admin"},
				Subject:       identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "admin-1"},
			},
			TenantID: "tenant-1",
		},
		SessionID:   "sess-1",
		SubjectKind: identity.SubjectKindServiceAccount,
		SubjectID:   "operator-1",
		Operations:  []core.SessionOperation{core.SessionOperationSend},
	})
	require.NoError(t, err)
	require.Equal(t, "operator-1", result.Delegation.Grantee.ID)

	records, err := sessionStore.ListDelegationsBySessionID(context.Background(), "sess-1")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, core.SessionOperationSend, records[0].Operations[0])
}

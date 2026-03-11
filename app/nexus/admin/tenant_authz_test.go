package admin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestGetSessionDeniesCrossTenantAccess(t *testing.T) {
	t.Parallel()

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer sessionStore.Close()
	require.NoError(t, sessionStore.UpsertBoundary(context.Background(), "tenant-b:webchat:conv-1", &core.SessionBoundary{
		SessionID:  "sess-1",
		RoutingKey: "tenant-b:webchat:conv-1",
		TenantID:   "tenant-b",
		Partition:  "local",
		Scope:      core.SessionScopePerChannelPeer,
		ChannelID:  "webchat",
		PeerID:     "conv-1",
		Owner:      core.SubjectRef{TenantID: "tenant-b", Kind: core.SubjectKindServiceAccount, ID: "svc-b"},
		TrustClass: core.TrustClassRemoteApproved,
		CreatedAt:  time.Now().UTC(),
	}))

	svc := NewService(ServiceConfig{Sessions: sessionStore}).(*service)
	_, err = svc.GetSession(context.Background(), GetSessionRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:operator"},
				Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-b",
		},
		SessionID: "sess-1",
	})
	require.Error(t, err)
	var adminErr AdminError
	require.ErrorAs(t, err, &adminErr)
	require.Equal(t, AdminErrorPolicyDenied, adminErr.Code)
}

func TestListEventsFiltersToAuthorizedTenant(t *testing.T) {
	t.Parallel()

	eventLog, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()
	_, err = eventLog.Append(context.Background(), "local", []core.FrameworkEvent{
		{Timestamp: time.Now().UTC(), Type: "message.inbound.v1", Actor: core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"}, Partition: "local"},
		{Timestamp: time.Now().UTC(), Type: "message.inbound.v1", Actor: core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-b"}, Partition: "local"},
	})
	require.NoError(t, err)

	svc := NewService(ServiceConfig{Events: eventLog, Partition: "local"}).(*service)
	result, err := svc.ReadEventStream(context.Background(), ReadEventStreamRequest{
		AdminRequest: AdminRequest{
			Principal: core.AuthenticatedPrincipal{
				TenantID:      "tenant-a",
				Authenticated: true,
				Scopes:        []string{"nexus:operator"},
				Subject:       core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindServiceAccount, ID: "admin-a"},
			},
			TenantID: "tenant-a",
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	require.Equal(t, "tenant-a", result.Events[0].Actor.TenantID)
}

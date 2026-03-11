package admin

import (
	"context"
	"path/filepath"
	"testing"

	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestListChannelsFiltersActivityToAuthorizedTenant(t *testing.T) {
	t.Parallel()

	eventLog, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()

	_, err = eventLog.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: "local",
			Payload:   []byte(`{"channel":"webchat"}`),
		},
		{
			Type:      core.FrameworkEventMessageOutbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-b"},
			Partition: "local",
			Payload:   []byte(`{"channel":"webchat"}`),
		},
	})
	require.NoError(t, err)

	svc := NewService(ServiceConfig{
		Events:    eventLog,
		Partition: "local",
		Config: nexuscfg.Config{
			Channels: map[string]map[string]any{
				"webchat": {},
			},
		},
	}).(*service)

	result, err := svc.ListChannels(context.Background(), ListChannelsRequest{
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
	require.Len(t, result.Channels, 1)
	require.Equal(t, uint64(1), result.Channels[0].Inbound)
	require.Equal(t, uint64(0), result.Channels[0].Outbound)
}

func TestHealthFiltersEventCountsToAuthorizedTenant(t *testing.T) {
	t.Parallel()

	eventLog, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer eventLog.Close()

	nodeStore, err := db.NewSQLiteNodeStore(filepath.Join(t.TempDir(), "nodes.db"))
	require.NoError(t, err)
	defer nodeStore.Close()

	sessionStore, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer sessionStore.Close()

	_, err = eventLog.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Seq:       0,
			Type:      core.FrameworkEventMessageInbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Partition: "local",
			Payload:   []byte(`{"channel":"webchat"}`),
		},
		{
			Seq:       0,
			Type:      core.FrameworkEventMessageOutbound,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-b"},
			Partition: "local",
			Payload:   []byte(`{"channel":"webchat"}`),
		},
	})
	require.NoError(t, err)

	svc := NewService(ServiceConfig{
		Events:    eventLog,
		Nodes:     nodeStore,
		Sessions:  sessionStore,
		Partition: "local",
		Config: nexuscfg.Config{
			Gateway: nexuscfg.GatewayConfig{Bind: ":8090"},
			Channels: map[string]map[string]any{
				"webchat": {},
			},
		},
	}).(*service)

	result, err := svc.Health(context.Background(), HealthRequest{
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
	require.Equal(t, uint64(1), result.EventCounts[core.FrameworkEventMessageInbound])
	require.Zero(t, result.EventCounts[core.FrameworkEventMessageOutbound])
	require.Equal(t, uint64(1), result.Channels[0].Inbound)
	require.Equal(t, uint64(0), result.Channels[0].Outbound)
}

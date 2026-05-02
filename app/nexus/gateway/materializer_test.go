package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"github.com/stretchr/testify/require"
)

func TestStateMaterializerTracksSessionsAndChannels(t *testing.T) {
	materializer := NewStateMaterializer()

	inbound, err := json.Marshal(map[string]any{"channel": "webchat"})
	require.NoError(t, err)
	outbound, err := json.Marshal(map[string]any{"channel": "slack"})
	require.NoError(t, err)

	err = materializer.Apply(context.Background(), []core.FrameworkEvent{
		{
			Seq:       2,
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionCreated,
			Actor:     identity.EventActor{Kind: "agent", ID: "agent-session", TenantID: "tenant-a"},
		},
		{
			Seq:       3,
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Actor:     identity.EventActor{Kind: "channel", ID: "webchat", TenantID: "tenant-a"},
			Payload:   inbound,
		},
		{
			Seq:       4,
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageOutbound,
			Actor:     identity.EventActor{Kind: "channel", ID: "slack", TenantID: "tenant-b"},
			Payload:   outbound,
		},
	})
	require.NoError(t, err)

	state := materializer.State()
	require.Equal(t, uint64(4), state.LastSeq)
	require.Len(t, state.ActiveSessions, 1)
	require.Equal(t, uint64(1), state.ChannelActivity["webchat"].Inbound)
	require.Equal(t, uint64(1), state.ChannelActivity["slack"].Outbound)
	require.Equal(t, uint64(1), state.EventTypeCounts[core.FrameworkEventSessionCreated])

	tenantA := materializer.StateForTenant("tenant-a")
	require.Equal(t, uint64(3), tenantA.LastSeq)
	require.Len(t, tenantA.ActiveSessions, 1)
	require.Equal(t, uint64(1), tenantA.ChannelActivity["webchat"].Inbound)
	require.Zero(t, tenantA.ChannelActivity["slack"].Outbound)
	require.Equal(t, uint64(1), tenantA.EventTypeCounts[core.FrameworkEventSessionCreated])

	tenantB := materializer.StateForTenant("tenant-b")
	require.Equal(t, uint64(4), tenantB.LastSeq)
	require.Empty(t, tenantB.ActiveSessions)
	require.Equal(t, uint64(1), tenantB.ChannelActivity["slack"].Outbound)
	require.Zero(t, tenantB.ChannelActivity["webchat"].Inbound)
	require.Equal(t, uint64(1), tenantB.EventTypeCounts[core.FrameworkEventMessageOutbound])
}

func TestStateMaterializerSnapshotRestore(t *testing.T) {
	materializer := NewStateMaterializer()
	require.NoError(t, materializer.Apply(context.Background(), []core.FrameworkEvent{{
		Seq:       1,
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventSessionCreated,
		Actor:     identity.EventActor{Kind: "user", ID: "user-session", TenantID: "tenant-a"},
	}}))

	data, err := materializer.Snapshot(context.Background())
	require.NoError(t, err)

	restored := NewStateMaterializer()
	require.NoError(t, restored.Restore(context.Background(), data))
	require.Equal(t, materializer.State(), restored.State())
}

package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
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
			Actor:     core.EventActor{Kind: "agent", ID: "agent-session"},
		},
		{
			Seq:       3,
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   inbound,
		},
		{
			Seq:       4,
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageOutbound,
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
}

func TestStateMaterializerSnapshotRestore(t *testing.T) {
	materializer := NewStateMaterializer()
	require.NoError(t, materializer.Apply(context.Background(), []core.FrameworkEvent{{
		Seq:       1,
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventSessionCreated,
		Actor:     core.EventActor{Kind: "user", ID: "user-session"},
	}}))

	data, err := materializer.Snapshot(context.Background())
	require.NoError(t, err)

	restored := NewStateMaterializer()
	require.NoError(t, restored.Restore(context.Background(), data))
	require.Equal(t, materializer.State(), restored.State())
}

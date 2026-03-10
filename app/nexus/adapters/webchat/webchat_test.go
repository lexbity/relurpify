package webchat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type sinkRecorder struct{ events []core.FrameworkEvent }

func (s *sinkRecorder) Emit(_ context.Context, event core.FrameworkEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestWebchatAdapterEmitsInboundMessage(t *testing.T) {
	adapter := &Adapter{}
	sink := &sinkRecorder{}
	require.NoError(t, adapter.Start(context.Background(), nil, sink))
	require.NoError(t, adapter.emitInbound("webchat-1", "hello"))
	require.Eventually(t, func() bool { return len(sink.events) == 1 }, time.Second, 10*time.Millisecond)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(sink.events[0].Payload, &payload))
	require.Equal(t, core.FrameworkEventMessageInbound, sink.events[0].Type)
	require.Equal(t, "webchat", payload["channel"])
}

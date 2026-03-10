package channel

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type testAdapter struct {
	started bool
	stopped bool
	status  AdapterStatus
	lastMsg OutboundMessage
	starts  int
	stops   int
}

func (t *testAdapter) Name() string { return "webchat" }
func (t *testAdapter) Start(context.Context, json.RawMessage, EventSink) error {
	t.started = true
	t.starts++
	t.status.Connected = true
	return nil
}
func (t *testAdapter) Send(_ context.Context, msg OutboundMessage) error { t.lastMsg = msg; return nil }
func (t *testAdapter) Status() AdapterStatus                             { return t.status }
func (t *testAdapter) Stop(context.Context) error {
	t.stopped = true
	t.stops++
	t.status.Connected = false
	return nil
}

func TestManagerRegisterStartSendStop(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	adapter := &testAdapter{}
	require.NoError(t, manager.Register(adapter))
	require.NoError(t, manager.Start(context.Background(), nil))
	require.True(t, adapter.started)

	require.NoError(t, manager.Send(context.Background(), OutboundMessage{
		Channel:        "webchat",
		ConversationID: "conv-1",
		Content:        MessageContent{Text: "hello"},
	}))
	require.Equal(t, "hello", adapter.lastMsg.Content.Text)

	status := manager.Status()
	require.True(t, status["webchat"].Connected)

	require.NoError(t, manager.Stop(context.Background()))
	require.True(t, adapter.stopped)
}

func TestManagerRestart(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	adapter := &testAdapter{}
	require.NoError(t, manager.Register(adapter))
	require.NoError(t, manager.Start(context.Background(), map[string]json.RawMessage{"webchat": json.RawMessage(`{"enabled":true}`)}))

	require.NoError(t, manager.Restart(context.Background(), "webchat"))
	require.Equal(t, 2, adapter.starts)
	require.Equal(t, 1, adapter.stops)
	require.True(t, manager.Status()["webchat"].Connected)
}

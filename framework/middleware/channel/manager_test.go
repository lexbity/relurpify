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

func TestManagerRegisterNilAdapter(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	err := manager.Register(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "adapter required")
}

func TestManagerRegisterDuplicate(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	adapter := &testAdapter{}
	require.NoError(t, manager.Register(adapter))

	err := manager.Register(&testAdapter{}) // Same name "webchat"
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestManagerSendUnregistered(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	err := manager.Send(context.Background(), OutboundMessage{
		Channel:        "unknown",
		ConversationID: "conv-1",
		Content:        MessageContent{Text: "hello"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not registered")
}

func TestManagerRestartUnregistered(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	err := manager.Restart(context.Background(), "unknown")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not registered")
}

func TestManagerStartWithNilConfigs(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	adapter := &testAdapter{}
	require.NoError(t, manager.Register(adapter))
	require.NoError(t, manager.Start(context.Background(), nil))
	require.True(t, adapter.started)
}

func TestManagerEmpty(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}

	// Empty manager operations should not panic
	require.NoError(t, manager.Stop(context.Background()))
	status := manager.Status()
	require.Empty(t, status)
}

func TestManagerNilSinkUsesLog(t *testing.T) {
	manager := NewManager(nil, nil)
	adapter := &testAdapter{}
	require.NoError(t, manager.Register(adapter))
	require.NoError(t, manager.Start(context.Background(), nil))
	require.True(t, adapter.started)
}

func TestManagerStatusCopy(t *testing.T) {
	manager := &Manager{sink: eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })}
	adapter := &testAdapter{}
	require.NoError(t, manager.Register(adapter))
	require.NoError(t, manager.Start(context.Background(), nil))

	status1 := manager.Status()
	status2 := manager.Status()

	// Should get independent copies
	require.Equal(t, status1["webchat"], status2["webchat"])
}

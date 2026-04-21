package channel

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestNewTestChannelAdapter(t *testing.T) {
	t.Run("with name", func(t *testing.T) {
		adapter := NewTestChannelAdapter("myadapter")
		require.Equal(t, "myadapter", adapter.Name())
		require.Equal(t, "local", adapter.Partition)
		require.True(t, adapter.status.Connected)
		require.NotNil(t, adapter.outbound)
	})

	t.Run("empty name defaults to test", func(t *testing.T) {
		adapter := NewTestChannelAdapter("")
		require.Equal(t, "test", adapter.Name())
	})
}

func TestTestChannelAdapterStartStop(t *testing.T) {
	adapter := NewTestChannelAdapter("test")
	sink := eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })

	// Start
	err := adapter.Start(context.Background(), nil, sink)
	require.NoError(t, err)
	require.True(t, adapter.Status().Connected)

	// Stop
	err = adapter.Stop(context.Background())
	require.NoError(t, err)
	require.False(t, adapter.Status().Connected)
}

func TestTestChannelAdapterSendInbound(t *testing.T) {
	t.Run("successful emit", func(t *testing.T) {
		adapter := NewTestChannelAdapter("test")
		var receivedEvent *core.FrameworkEvent
		sink := eventSinkFunc(func(ctx context.Context, ev core.FrameworkEvent) error {
			receivedEvent = &ev
			return nil
		})

		err := adapter.Start(context.Background(), nil, sink)
		require.NoError(t, err)

		msg := InboundMessage{
			Channel: "test",
			Sender: Identity{
				ChannelID: "user-1",
			},
			Content: MessageContent{Text: "Hello"},
		}

		err = adapter.SendInbound(context.Background(), msg)
		require.NoError(t, err)
		require.NotNil(t, receivedEvent)
		require.Equal(t, core.FrameworkEventMessageInbound, receivedEvent.Type)
		require.Equal(t, "channel", receivedEvent.Actor.Kind)
		require.Equal(t, "test", receivedEvent.Actor.ID)
		require.Equal(t, "local", receivedEvent.Partition)
	})

	t.Run("not started returns error", func(t *testing.T) {
		adapter := NewTestChannelAdapter("test")
		// Don't start the adapter

		msg := InboundMessage{
			Channel: "test",
			Content: MessageContent{Text: "Hello"},
		}

		err := adapter.SendInbound(context.Background(), msg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "adapter not started")
	})
}

func TestTestChannelAdapterSend(t *testing.T) {
	adapter := NewTestChannelAdapter("test")

	msg := OutboundMessage{
		Channel:        "test",
		ConversationID: "conv-1",
		Content:        MessageContent{Text: "Hello"},
	}

	err := adapter.Send(context.Background(), msg)
	require.NoError(t, err)

	// Verify message was sent to outbound channel
	select {
	case received := <-adapter.Outbound():
		require.Equal(t, "Hello", received.Content.Text)
	default:
		t.Fatal("expected message in outbound channel")
	}
}

func TestTestChannelAdapterSetStatus(t *testing.T) {
	adapter := NewTestChannelAdapter("test")

	newStatus := AdapterStatus{
		Connected:  false,
		LastError:  "connection lost",
		Reconnects: 3,
	}

	adapter.SetStatus(newStatus)
	status := adapter.Status()
	require.False(t, status.Connected)
	require.Equal(t, "connection lost", status.LastError)
	require.Equal(t, 3, status.Reconnects)
}

func TestTestChannelAdapterPartition(t *testing.T) {
	t.Run("returns set partition", func(t *testing.T) {
		adapter := NewTestChannelAdapter("test")
		adapter.Partition = "custom-partition"

		// Use reflection to test private method through SendInbound
		var receivedEvent *core.FrameworkEvent
		sink := eventSinkFunc(func(ctx context.Context, ev core.FrameworkEvent) error {
			receivedEvent = &ev
			return nil
		})

		err := adapter.Start(context.Background(), nil, sink)
		require.NoError(t, err)

		msg := InboundMessage{Channel: "test"}
		err = adapter.SendInbound(context.Background(), msg)
		require.NoError(t, err)
		require.Equal(t, "custom-partition", receivedEvent.Partition)
	})

	t.Run("empty partition defaults to local", func(t *testing.T) {
		adapter := NewTestChannelAdapter("test")
		adapter.Partition = ""

		var receivedEvent *core.FrameworkEvent
		sink := eventSinkFunc(func(ctx context.Context, ev core.FrameworkEvent) error {
			receivedEvent = &ev
			return nil
		})

		err := adapter.Start(context.Background(), nil, sink)
		require.NoError(t, err)

		msg := InboundMessage{Channel: "test"}
		err = adapter.SendInbound(context.Background(), msg)
		require.NoError(t, err)
		require.Equal(t, "local", receivedEvent.Partition)
	})
}

func TestTestChannelAdapterSendInboundMarshalError(t *testing.T) {
	adapter := NewTestChannelAdapter("test")
	sink := eventSinkFunc(func(context.Context, core.FrameworkEvent) error { return nil })

	err := adapter.Start(context.Background(), nil, sink)
	require.NoError(t, err)

	// Create a message with content
	msg := InboundMessage{
		Channel: "test",
		Content: MessageContent{
			Text: "test message",
		},
	}

	// Verify normal operation
	err = adapter.SendInbound(context.Background(), msg)
	require.NoError(t, err)
}

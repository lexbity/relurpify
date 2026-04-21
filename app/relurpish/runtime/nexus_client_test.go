package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type fakeNexusConn struct {
	writes []any
	reads  []map[string]json.RawMessage
	block  chan struct{}
}

func (f *fakeNexusConn) WriteJSON(v any) error {
	f.writes = append(f.writes, v)
	return nil
}

func (f *fakeNexusConn) ReadJSON(v any) error {
	if len(f.reads) == 0 {
		if f.block != nil {
			<-f.block
		}
		return errors.New("closed")
	}
	data, err := json.Marshal(f.reads[0])
	if err != nil {
		return err
	}
	f.reads = f.reads[1:]
	return json.Unmarshal(data, v)
}

func (f *fakeNexusConn) Close() error { return nil }

func nexusFrame(t *testing.T, payload map[string]any) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	var frame map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &frame))
	return frame
}

func TestNexusClientConnectAndSubscribe(t *testing.T) {
	conn := &fakeNexusConn{
		reads: []map[string]json.RawMessage{
			nexusFrame(t, map[string]any{
				"type":       "connected",
				"session_id": "agent-session",
				"server_seq": 1,
				"capabilities": []map[string]any{{
					"id":   "remote.echo",
					"name": "remote.echo",
				}},
			}),
			nexusFrame(t, map[string]any{
				"type": "event",
				"event": map[string]any{
					"seq":  2,
					"type": core.FrameworkEventSessionMessage,
				},
			}),
		},
	}
	client := NewNexusClient(NexusConfig{Enabled: true, Address: "ws://nexus", AutoReconnect: false})
	client.Dial = func(context.Context, string, string) (nexusConn, error) { return conn, nil }

	ch, unsub := client.Subscribe(1)
	defer unsub()
	require.NoError(t, client.Start(context.Background()))
	require.Eventually(t, func() bool { return client.SessionID() == "agent-session" }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return len(client.Capabilities()) == 1 }, time.Second, 10*time.Millisecond)

	select {
	case event := <-ch:
		require.Equal(t, core.FrameworkEventSessionMessage, event.Type)
	case <-time.After(time.Second):
		t.Fatal("expected nexus event")
	}
}

func TestNexusClientInvokeCapability(t *testing.T) {
	block := make(chan struct{})
	conn := &fakeNexusConn{
		reads: []map[string]json.RawMessage{
			nexusFrame(t, map[string]any{
				"type":       "connected",
				"session_id": "agent-session",
				"server_seq": 1,
			}),
		},
		block: block,
	}
	client := NewNexusClient(NexusConfig{Enabled: true, Address: "ws://nexus"})
	client.Dial = func(context.Context, string, string) (nexusConn, error) { return conn, nil }
	require.NoError(t, client.Start(context.Background()))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for len(conn.writes) < 2 {
			time.Sleep(time.Millisecond)
		}
		request := conn.writes[1].(map[string]any)
		require.NoError(t, client.handleFrame(nexusFrame(t, map[string]any{
			"type":           "capability.result",
			"correlation_id": request["correlation_id"],
			"result": map[string]any{
				"success": true,
			},
		})))
		close(block)
	}()

	result, err := client.InvokeCapability(context.Background(), "sess-1", "remote.echo", map[string]any{"text": "hi"})
	require.NoError(t, err)
	require.True(t, result.Success)
	request := conn.writes[1].(map[string]any)
	require.Equal(t, "sess-1", request["session_key"])
	<-done
}

func TestNexusClientCorrelationIDsAreUnique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := randomCorrelationID()
		require.NotEmpty(t, id)
		require.False(t, seen[id], "duplicate correlation ID: %s", id)
		seen[id] = true
	}
}

func TestNexusClientReconnectsAfterTransientFailures(t *testing.T) {
	// First connection succeeds so Start returns nil and starts the reconnect loop.
	first := &fakeNexusConn{
		reads: []map[string]json.RawMessage{
			nexusFrame(t, map[string]any{
				"type":       "connected",
				"session_id": "agent-session-1",
				"server_seq": 1,
			}),
			// No more frames — ReadJSON returns an error → connection drops → reconnect.
		},
	}
	second := &fakeNexusConn{
		reads: []map[string]json.RawMessage{
			nexusFrame(t, map[string]any{
				"type":       "connected",
				"session_id": "agent-session-2",
				"server_seq": 2,
			}),
		},
	}
	dialCount := 0
	client := NewNexusClient(NexusConfig{Enabled: true, Address: "ws://nexus", AutoReconnect: true})
	client.Dial = func(ctx context.Context, address, token string) (nexusConn, error) {
		dialCount++
		switch dialCount {
		case 1:
			return first, nil
		case 2:
			// Simulate one transient failure after the first connection drops.
			return nil, fmt.Errorf("transient error")
		default:
			return second, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, client.Start(ctx))
	// Eventually the client reconnects and establishes the second session.
	require.Eventually(t, func() bool { return client.SessionID() == "agent-session-2" }, 5*time.Second, 10*time.Millisecond)
	require.GreaterOrEqual(t, dialCount, 3)
}

func TestNexusClientReconnects(t *testing.T) {
	first := &fakeNexusConn{
		reads: []map[string]json.RawMessage{
			nexusFrame(t, map[string]any{
				"type":       "connected",
				"session_id": "agent-session",
				"server_seq": 1,
			}),
		},
	}
	second := &fakeNexusConn{
		reads: []map[string]json.RawMessage{
			nexusFrame(t, map[string]any{
				"type":       "connected",
				"session_id": "agent-session-2",
				"server_seq": 2,
			}),
		},
	}
	dials := 0
	client := NewNexusClient(NexusConfig{Enabled: true, Address: "ws://nexus", AutoReconnect: true})
	client.Dial = func(context.Context, string, string) (nexusConn, error) {
		dials++
		if dials == 1 {
			return first, nil
		}
		return second, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, client.Start(ctx))
	require.Eventually(t, func() bool { return dials >= 2 }, time.Second, 10*time.Millisecond)
}

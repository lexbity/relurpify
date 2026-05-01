package node

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type fakeRPCConn struct {
	mu     sync.Mutex
	writes []any
	reads  []map[string]json.RawMessage
}

func (f *fakeRPCConn) WriteJSON(v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, v)
	return nil
}

func (f *fakeRPCConn) ReadJSON(v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	target := v.(*map[string]json.RawMessage)
	if len(f.reads) == 0 {
		time.Sleep(5 * time.Millisecond)
		return context.DeadlineExceeded
	}
	*target = f.reads[0]
	f.reads = f.reads[1:]
	return nil
}

func (f *fakeRPCConn) Close() error { return nil }

func (f *fakeRPCConn) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func (f *fakeRPCConn) firstWrite() any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) == 0 {
		return nil
	}
	return f.writes[0]
}

func TestWSConnectionInvokeRoundTrip(t *testing.T) {
	conn := &fakeRPCConn{}
	ws := &WSConnection{Conn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for conn.writeCount() == 0 {
			time.Sleep(time.Millisecond)
		}
		request := conn.firstWrite().(invokeRequest)
		payload, _ := json.Marshal(map[string]any{
			"type":           "capability.result",
			"correlation_id": request.CorrelationID,
			"result": map[string]any{
				"success": true,
			},
		})
		var frame map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &frame))
		require.NoError(t, ws.handleFrame(context.Background(), frame))
	}()

	result, err := ws.Invoke(ctx, "camera.capture", map[string]any{"quality": "high"})
	require.NoError(t, err)
	require.True(t, result.Success)
	<-done
}

func TestWSConnectionInvokeTimeout(t *testing.T) {
	ws := &WSConnection{Conn: &fakeRPCConn{}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := ws.Invoke(ctx, "camera.capture", nil)
	require.Error(t, err)
}

func TestWSConnectionHealthFrameUpdatesState(t *testing.T) {
	ws := &WSConnection{}
	payload, err := json.Marshal(map[string]any{
		"type": string(core.FrameworkEventNodeHealth),
		"health": map[string]any{
			"online": true,
		},
	})
	require.NoError(t, err)
	var frame map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &frame))
	require.NoError(t, ws.handleFrame(context.Background(), frame))
	require.True(t, ws.Health().Online)
}

func TestFramedRPCConnWrapsCapabilityFrames(t *testing.T) {
	conn := &fakeRPCConn{}
	framed := NewFramedRPCConn(conn, "sess-1")
	framed.Now = func() time.Time { return time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC) }

	require.NoError(t, framed.WriteJSON(invokeRequest{
		Type:          "capability.invoke",
		CorrelationID: "corr-1",
		CapabilityID:  "camera.capture",
	}))
	require.Len(t, conn.writes, 1)
	frame := conn.writes[0].(transportFrame)
	require.Equal(t, TransportFrameType, frame.Type)
	require.Equal(t, TransportChannelCapability, frame.Channel)
	require.Equal(t, "sess-1", frame.SessionID)
	require.NotEmpty(t, frame.Payload)
}

func TestFramedRPCConnUnwrapsTransportPayload(t *testing.T) {
	responsePayload, err := json.Marshal(map[string]any{
		"type":           "capability.result",
		"correlation_id": "corr-1",
		"result": map[string]any{
			"success": true,
		},
	})
	require.NoError(t, err)
	framePayload, err := json.Marshal(map[string]any{
		"type":       TransportFrameType,
		"channel":    TransportChannelCapability,
		"session_id": "sess-1",
		"payload":    json.RawMessage(responsePayload),
	})
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(framePayload, &raw))

	framed := NewFramedRPCConn(&fakeRPCConn{reads: []map[string]json.RawMessage{raw}}, "sess-1")
	var frame map[string]json.RawMessage
	require.NoError(t, framed.ReadJSON(&frame))
	require.Equal(t, "capability.result", frameTypeFromRaw(frame))
}

func TestWSConnectionDelegatesUnknownFramesToHandler(t *testing.T) {
	ws := &WSConnection{}
	called := false
	ws.FrameHandler = func(_ context.Context, _ *WSConnection, frame map[string]json.RawMessage) error {
		called = frameTypeFromRaw(frame) == "fmp.chunk.read"
		return nil
	}
	payload, err := json.Marshal(map[string]any{"type": "fmp.chunk.read"})
	require.NoError(t, err)
	var frame map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &frame))
	require.NoError(t, ws.handleFrame(context.Background(), frame))
	require.True(t, called)
}

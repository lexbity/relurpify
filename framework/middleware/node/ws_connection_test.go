package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type fakeRPCConn struct {
	writes []any
	reads  []map[string]json.RawMessage
}

func (f *fakeRPCConn) WriteJSON(v any) error {
	f.writes = append(f.writes, v)
	return nil
}

func (f *fakeRPCConn) ReadJSON(v any) error {
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

func TestWSConnectionInvokeRoundTrip(t *testing.T) {
	conn := &fakeRPCConn{}
	ws := &WSConnection{Conn: conn}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for len(conn.writes) == 0 {
			time.Sleep(time.Millisecond)
		}
		request := conn.writes[0].(invokeRequest)
		payload, _ := json.Marshal(map[string]any{
			"type":           "capability.result",
			"correlation_id": request.CorrelationID,
			"result": map[string]any{
				"success": true,
			},
		})
		var frame map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &frame))
		require.NoError(t, ws.handleFrame(frame))
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
	require.NoError(t, ws.handleFrame(frame))
	require.True(t, ws.Health().Online)
}

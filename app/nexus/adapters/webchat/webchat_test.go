package webchat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
	"github.com/gorilla/websocket"
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

func TestWebchatAdapterLifecycleAndSend(t *testing.T) {
	adapter := &Adapter{}
	require.Equal(t, "webchat", adapter.Name())
	require.Equal(t, channel.AdapterStatus{}, adapter.Status())

	sink := &sinkRecorder{}
	require.NoError(t, adapter.Start(context.Background(), nil, sink))
	require.True(t, adapter.Status().Connected)

	server := httptest.NewServer(adapter.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	require.Eventually(t, func() bool {
		return len(adapter.conns) == 1
	}, time.Second, 10*time.Millisecond)

	var id string
	for k := range adapter.conns {
		id = k
	}
	require.NotEmpty(t, id)

	require.NoError(t, adapter.Send(context.Background(), channel.OutboundMessage{
		ConversationID: id,
		Content: channel.MessageContent{
			Text: "hello from send",
		},
	}))
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "hello from send", string(data))

	require.NoError(t, adapter.Stop(context.Background()))
	require.False(t, adapter.Status().Connected)
	require.Len(t, adapter.conns, 0)
}

func TestWebchatAdapterHandlerAndReadLoop(t *testing.T) {
	adapter := &Adapter{}
	sink := &sinkRecorder{}
	require.NoError(t, adapter.Start(context.Background(), nil, sink))

	server := httptest.NewServer(adapter.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(adapter.conns) == 1
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("inbound hello")))
	require.Eventually(t, func() bool {
		return len(sink.events) == 1
	}, time.Second, 10*time.Millisecond)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(sink.events[0].Payload, &payload))
	require.Equal(t, "webchat", payload["channel"])
	require.Equal(t, "inbound hello", payload["content"].(map[string]any)["text"])

	require.NoError(t, conn.Close())
	require.Eventually(t, func() bool {
		return len(adapter.conns) == 0
	}, time.Second, 10*time.Millisecond)
}

func TestWebchatAdapterReadLoopWithoutSink(t *testing.T) {
	adapter := &Adapter{}
	server := httptest.NewServer(adapter.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ignored")))
	require.NoError(t, conn.Close())
}

func TestWebchatAdapterSendMissingConversation(t *testing.T) {
	adapter := &Adapter{}
	err := adapter.Send(context.Background(), channel.OutboundMessage{
		ConversationID: "missing",
		Content:        channel.MessageContent{Text: "hello"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not connected")
}

func TestWebchatAdapterHandlerRejectsUpgradeFailure(t *testing.T) {
	adapter := &Adapter{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	adapter.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "websocket")
}

func TestWebchatAdapterEmitInboundWithoutSink(t *testing.T) {
	adapter := &Adapter{}
	require.NoError(t, adapter.emitInbound("webchat-1", "hello"))
}

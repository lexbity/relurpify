package telegram

import (
	"context"
	"encoding/json"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
	"github.com/stretchr/testify/require"
)

type fakeClient struct {
	chatID  int64
	text    string
	replyTo int
}

func (f *fakeClient) SendMessage(_ context.Context, chatID int64, text string, replyToMessageID int) error {
	f.chatID = chatID
	f.text = text
	f.replyTo = replyToMessageID
	return nil
}

type sinkRecorder struct{ events []core.FrameworkEvent }

func (s *sinkRecorder) Emit(_ context.Context, event core.FrameworkEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestTelegramAdapterNormalizesMediaUpdate(t *testing.T) {
	adapter := &Adapter{Client: &fakeClient{}}
	sink := &sinkRecorder{}
	require.NoError(t, adapter.Start(context.Background(), json.RawMessage(`{"bot_token":"x"}`), sink))

	err := adapter.EmitUpdate(context.Background(), Update{
		UpdateID: 1,
		Message: &Message{
			MessageID: 7,
			Chat:      Chat{ID: 42, Type: "private"},
			From:      User{ID: 12, Username: "user"},
			Caption:   "photo",
			Photo:     []PhotoRef{{FileID: "photo-1", FileSize: 12}},
			Voice:     &VoiceRef{FileID: "voice-1", FileSize: 33, MimeType: "audio/ogg"},
		},
	})
	require.NoError(t, err)
	require.Len(t, sink.events, 1)
	require.Equal(t, core.FrameworkEventMessageInbound, sink.events[0].Type)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(sink.events[0].Payload, &payload))
	require.Equal(t, "telegram", payload["channel"])
	content := payload["content"].(map[string]any)
	require.Equal(t, "photo", content["text"])
	require.Len(t, content["media"], 2)
}

func TestTelegramAdapterSendFormatsReplyTo(t *testing.T) {
	client := &fakeClient{}
	adapter := &Adapter{Client: client}
	require.NoError(t, adapter.Start(context.Background(), json.RawMessage(`{"bot_token":"x"}`), &sinkRecorder{}))

	err := adapter.Send(context.Background(), channel.OutboundMessage{
		Channel:        "telegram",
		ConversationID: "42",
		ThreadID:       "8",
		Content:        channel.MessageContent{Text: "hello"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(42), client.chatID)
	require.Equal(t, "hello", client.text)
	require.Equal(t, 8, client.replyTo)
}

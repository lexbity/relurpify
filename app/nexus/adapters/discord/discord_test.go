package discord

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	"github.com/stretchr/testify/require"
)

type fakeSession struct {
	channelID string
	text      string
}

func (f *fakeSession) SendMessage(_ context.Context, channelID string, text string) error {
	f.channelID = channelID
	f.text = text
	return nil
}

type sinkRecorder struct{ events []core.FrameworkEvent }

func (s *sinkRecorder) Emit(_ context.Context, event core.FrameworkEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestDiscordAdapterNormalizesMentionAndThread(t *testing.T) {
	adapter := &Adapter{Session: &fakeSession{}}
	sink := &sinkRecorder{}
	require.NoError(t, adapter.Start(context.Background(), json.RawMessage(`{"bot_token":"x"}`), sink))

	err := adapter.EmitMessage(context.Background(), Message{
		ID:        "msg-1",
		ChannelID: "chan-1",
		ThreadID:  "thread-9",
		Content:   "hello",
		AuthorID:  "user-1",
		Author:    "User",
		Mentions:  []string{"bot-1"},
	}, "bot-1")
	require.NoError(t, err)
	require.Len(t, sink.events, 1)

	var payload channel.InboundMessage
	require.NoError(t, json.Unmarshal(sink.events[0].Payload, &payload))
	require.True(t, payload.Conversation.Mentioned)
	require.Equal(t, "thread-9", payload.Conversation.ThreadID)
}

func TestDiscordAdapterSend(t *testing.T) {
	session := &fakeSession{}
	adapter := &Adapter{Session: session}
	require.NoError(t, adapter.Start(context.Background(), json.RawMessage(`{"bot_token":"x"}`), &sinkRecorder{}))
	require.NoError(t, adapter.Send(context.Background(), channel.OutboundMessage{
		Channel:        "discord",
		ConversationID: "chan-1",
		Content:        channel.MessageContent{Text: "reply"},
	}))
	require.Equal(t, "chan-1", session.channelID)
	require.Equal(t, "reply", session.text)
}

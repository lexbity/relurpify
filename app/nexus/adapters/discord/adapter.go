package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

type Session interface {
	SendMessage(ctx context.Context, channelID string, text string) error
}

type Adapter struct {
	Session Session

	mu     sync.RWMutex
	sink   channel.EventSink
	status channel.AdapterStatus
}

type Config struct {
	BotToken  string `json:"bot_token"`
	AppID     string `json:"app_id,omitempty"`
	BotUserID string `json:"bot_user_id,omitempty"`
}

type Message struct {
	ID        string   `json:"id"`
	ChannelID string   `json:"channel_id"`
	ThreadID  string   `json:"thread_id,omitempty"`
	Content   string   `json:"content"`
	AuthorID  string   `json:"author_id"`
	Author    string   `json:"author"`
	Mentions  []string `json:"mentions,omitempty"`
	IsDM      bool     `json:"is_dm,omitempty"`
}

func (a *Adapter) Name() string { return "discord" }

func (a *Adapter) Start(_ context.Context, raw json.RawMessage, sink channel.EventSink) error {
	var cfg Config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	if cfg.BotToken == "" && a.Session == nil {
		return fmt.Errorf("discord bot_token required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sink = sink
	a.status.Connected = true
	return nil
}

func (a *Adapter) Stop(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Connected = false
	return nil
}

func (a *Adapter) Status() channel.AdapterStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *Adapter) Send(ctx context.Context, msg channel.OutboundMessage) error {
	a.mu.RLock()
	session := a.Session
	a.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("discord session unavailable")
	}
	return session.SendMessage(ctx, msg.ConversationID, msg.Content.Text)
}

func (a *Adapter) EmitMessage(ctx context.Context, msg Message, botUserID string) error {
	payload, _ := json.Marshal(channel.InboundMessage{
		Channel: "discord",
		Sender: channel.Identity{
			ChannelID:   msg.AuthorID,
			DisplayName: msg.Author,
		},
		Conversation: channel.Conversation{
			Kind:      conversationKind(msg),
			ID:        msg.ChannelID,
			ThreadID:  msg.ThreadID,
			Mentioned: mentioned(msg, botUserID),
		},
		Content: channel.MessageContent{
			Text: strings.TrimSpace(msg.Content),
		},
	})
	return a.sink.Emit(ctx, core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Actor:     identity.EventActor{Kind: "channel", ID: "discord"},
		Partition: "local",
	})
}

func mentioned(msg Message, botUserID string) bool {
	for _, id := range msg.Mentions {
		if id == botUserID {
			return true
		}
	}
	return false
}

func conversationKind(msg Message) string {
	if msg.IsDM {
		return "dm"
	}
	return "guild"
}

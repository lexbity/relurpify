package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
)

type Client interface {
	SendMessage(ctx context.Context, chatID int64, text string, replyToMessageID int) error
}

type Adapter struct {
	Client Client

	mu     sync.RWMutex
	sink   channel.EventSink
	status channel.AdapterStatus
}

type Config struct {
	BotToken string `json:"bot_token"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int        `json:"message_id"`
	Chat      Chat       `json:"chat"`
	From      User       `json:"from"`
	Text      string     `json:"text,omitempty"`
	Caption   string     `json:"caption,omitempty"`
	Photo     []PhotoRef `json:"photo,omitempty"`
	Voice     *VoiceRef  `json:"voice,omitempty"`
	Document  *Document  `json:"document,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

type PhotoRef struct {
	FileID      string `json:"file_id"`
	FileSize    int64  `json:"file_size,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type VoiceRef struct {
	FileID      string `json:"file_id"`
	FileSize    int64  `json:"file_size,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
	DurationSec int    `json:"duration,omitempty"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

func (a *Adapter) Name() string { return "telegram" }

func (a *Adapter) Start(_ context.Context, raw json.RawMessage, sink channel.EventSink) error {
	var cfg Config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	if cfg.BotToken == "" && a.Client == nil {
		return fmt.Errorf("telegram bot_token required")
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
	client := a.Client
	a.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("telegram client unavailable")
	}
	chatID, err := strconv.ParseInt(msg.ConversationID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram conversation id: %w", err)
	}
	replyID := 0
	if msg.ThreadID != "" {
		replyID, err = strconv.Atoi(msg.ThreadID)
		if err != nil {
			return fmt.Errorf("invalid reply message id: %w", err)
		}
	}
	return client.SendMessage(ctx, chatID, msg.Content.Text, replyID)
}

func (a *Adapter) EmitUpdate(ctx context.Context, update Update) error {
	if update.Message == nil {
		return nil
	}
	inbound := normalizeUpdate(update)
	payload, _ := json.Marshal(inbound)
	return a.sink.Emit(ctx, core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "channel", ID: "telegram"},
		Partition: "local",
	})
}

func normalizeUpdate(update Update) channel.InboundMessage {
	msg := update.Message
	content := channel.MessageContent{
		Text:  firstNonEmpty(msg.Text, msg.Caption),
		Media: mediaRefs(msg),
	}
	name := firstNonEmpty(msg.From.Username, msg.From.FirstName)
	return channel.InboundMessage{
		Channel: "telegram",
		Sender: channel.Identity{
			ChannelID:   strconv.FormatInt(msg.From.ID, 10),
			DisplayName: name,
		},
		Conversation: channel.Conversation{
			Kind: msg.Chat.Type,
			ID:   strconv.FormatInt(msg.Chat.ID, 10),
		},
		Content: content,
	}
}

func mediaRefs(msg *Message) []channel.MediaRef {
	if msg == nil {
		return nil
	}
	var refs []channel.MediaRef
	for _, photo := range msg.Photo {
		refs = append(refs, channel.MediaRef{
			Hash:        photo.FileID,
			ContentType: firstNonEmpty(photo.ContentType, "image/jpeg"),
			SizeBytes:   photo.FileSize,
		})
	}
	if msg.Voice != nil {
		refs = append(refs, channel.MediaRef{
			Hash:        msg.Voice.FileID,
			ContentType: firstNonEmpty(msg.Voice.MimeType, "audio/ogg"),
			SizeBytes:   msg.Voice.FileSize,
		})
	}
	if msg.Document != nil {
		refs = append(refs, channel.MediaRef{
			Hash:        msg.Document.FileID,
			ContentType: firstNonEmpty(msg.Document.MimeType, "application/octet-stream"),
			Filename:    msg.Document.FileName,
			SizeBytes:   msg.Document.FileSize,
		})
	}
	return refs
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

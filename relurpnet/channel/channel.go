package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
)

// Adapter normalizes inbound messages from an upstream service.
type Adapter interface {
	Name() string
	Start(ctx context.Context, config json.RawMessage, sink EventSink) error
	Send(ctx context.Context, msg OutboundMessage) error
	Status() AdapterStatus
	Stop(ctx context.Context) error
}

// EventSink receives normalized events from an adapter.
type EventSink interface {
	Emit(ctx context.Context, event core.FrameworkEvent) error
}

type InboundMessage struct {
	Channel      string          `json:"channel"`
	Account      string          `json:"account,omitempty"`
	Sender       Identity        `json:"sender"`
	Conversation Conversation    `json:"conversation"`
	Content      MessageContent  `json:"content"`
	RawMeta      json.RawMessage `json:"raw_meta,omitempty"`
}

type Identity struct {
	ChannelID   string `json:"channel_id"`
	DisplayName string `json:"display_name,omitempty"`
	ResolvedID  string `json:"resolved_id,omitempty"`
}

type Conversation struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id,omitempty"`
	Mentioned bool   `json:"mentioned"`
}

type MessageContent struct {
	Text       string          `json:"text,omitempty"`
	Media      []MediaRef      `json:"media,omitempty"`
	ReplyTo    string          `json:"reply_to,omitempty"`
	Reaction   string          `json:"reaction,omitempty"`
	Structured json.RawMessage `json:"structured,omitempty"`
}

type MediaRef struct {
	Hash          string `json:"hash"`
	ContentType   string `json:"content_type"`
	Filename      string `json:"filename,omitempty"`
	SizeBytes     int64  `json:"size_bytes"`
	Transcription string `json:"transcription,omitempty"`
}

type AdapterStatus struct {
	Connected  bool
	LastError  string
	Reconnects int
}

type OutboundMessage struct {
	Channel        string         `json:"channel"`
	Account        string         `json:"account,omitempty"`
	ConversationID string         `json:"conversation_id"`
	ThreadID       string         `json:"thread_id,omitempty"`
	Content        MessageContent `json:"content"`
}

// Manager supervises registered adapters.
type Manager struct {
	adapters map[string]Adapter
	configs  map[string]json.RawMessage
	sink     EventSink
	log      event.Log
	mu       sync.RWMutex
}

func NewManager(log event.Log, sink EventSink) *Manager {
	return &Manager{log: log, sink: sink}
}

func (m *Manager) Register(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.adapters == nil {
		m.adapters = map[string]Adapter{}
	}
	if _, exists := m.adapters[adapter.Name()]; exists {
		return fmt.Errorf("adapter %s already registered", adapter.Name())
	}
	m.adapters[adapter.Name()] = adapter
	return nil
}

func (m *Manager) Start(ctx context.Context, configs map[string]json.RawMessage) error {
	m.mu.Lock()
	if configs == nil {
		m.configs = nil
	} else {
		m.configs = make(map[string]json.RawMessage, len(configs))
		for name, config := range configs {
			m.configs[name] = append(json.RawMessage(nil), config...)
		}
	}
	defer m.mu.Unlock()
	for name, adapter := range m.adapters {
		if err := adapter.Start(ctx, configs[name], m.sinkOrDefault()); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, adapter := range m.adapters {
		if err := adapter.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Send(ctx context.Context, msg OutboundMessage) error {
	m.mu.RLock()
	adapter, ok := m.adapters[msg.Channel]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("adapter %s not registered", msg.Channel)
	}
	return adapter.Send(ctx, msg)
}

func (m *Manager) Status() map[string]AdapterStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]AdapterStatus, len(m.adapters))
	for name, adapter := range m.adapters {
		out[name] = adapter.Status()
	}
	return out
}

func (m *Manager) Restart(ctx context.Context, name string) error {
	m.mu.RLock()
	adapter, ok := m.adapters[name]
	config := append(json.RawMessage(nil), m.configs[name]...)
	sink := m.sinkOrDefault()
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("adapter %s not registered", name)
	}
	if err := adapter.Stop(ctx); err != nil {
		return err
	}
	return adapter.Start(ctx, config, sink)
}

func (m *Manager) sinkOrDefault() EventSink {
	if m.sink != nil {
		return m.sink
	}
	return eventSinkFunc(func(ctx context.Context, ev core.FrameworkEvent) error {
		if m.log == nil {
			return nil
		}
		_, err := m.log.Append(ctx, ev.Partition, []core.FrameworkEvent{ev})
		return err
	})
}

type eventSinkFunc func(ctx context.Context, event core.FrameworkEvent) error

func (f eventSinkFunc) Emit(ctx context.Context, event core.FrameworkEvent) error {
	return f(ctx, event)
}

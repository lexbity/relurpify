package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// TestChannelAdapter is a controllable adapter for end-to-end tests.
type TestChannelAdapter struct {
	AdapterName string
	Partition   string

	mu       sync.RWMutex
	sink     EventSink
	status   AdapterStatus
	outbound chan OutboundMessage
}

func NewTestChannelAdapter(name string) *TestChannelAdapter {
	if name == "" {
		name = "test"
	}
	return &TestChannelAdapter{
		AdapterName: name,
		Partition:   "local",
		status: AdapterStatus{
			Connected: true,
		},
		outbound: make(chan OutboundMessage, 32),
	}
}

func (a *TestChannelAdapter) Name() string {
	return a.AdapterName
}

func (a *TestChannelAdapter) Start(_ context.Context, _ json.RawMessage, sink EventSink) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sink = sink
	a.status.Connected = true
	a.status.LastError = ""
	return nil
}

func (a *TestChannelAdapter) Send(_ context.Context, msg OutboundMessage) error {
	a.outbound <- msg
	return nil
}

func (a *TestChannelAdapter) Status() AdapterStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *TestChannelAdapter) Stop(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.Connected = false
	return nil
}

func (a *TestChannelAdapter) SendInbound(ctx context.Context, msg InboundMessage) error {
	a.mu.RLock()
	sink := a.sink
	a.mu.RUnlock()
	if sink == nil {
		return fmt.Errorf("adapter not started")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return sink.Emit(ctx, core.FrameworkEvent{
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
		Actor:     core.EventActor{Kind: "channel", ID: a.AdapterName},
		Partition: a.partition(),
	})
}

func (a *TestChannelAdapter) Outbound() <-chan OutboundMessage {
	return a.outbound
}

func (a *TestChannelAdapter) SetStatus(status AdapterStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = status
}

func (a *TestChannelAdapter) partition() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.Partition == "" {
		return "local"
	}
	return a.Partition
}

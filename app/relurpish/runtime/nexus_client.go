package runtime

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/gorilla/websocket"
)

type nexusConn interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
}

type nexusDialer func(ctx context.Context, address, token string) (nexusConn, error)

type NexusClient struct {
	Address       string
	Token         string
	AutoReconnect bool
	Dial          nexusDialer
	OnConnected   func([]core.CapabilityDescriptor)

	mu           sync.RWMutex
	conn         nexusConn
	sessionID    string
	lastSeenSeq  uint64
	capabilities []core.CapabilityDescriptor
	subscribers  map[int]chan core.FrameworkEvent
	nextSubID    int
	pending      map[string]chan core.CapabilityExecutionResult
}

func NewNexusClient(cfg NexusConfig) *NexusClient {
	return &NexusClient{
		Address:       cfg.Address,
		Token:         cfg.Token,
		AutoReconnect: cfg.AutoReconnect,
		Dial:          dialNexusWebsocket,
		subscribers:   map[int]chan core.FrameworkEvent{},
		pending:       map[string]chan core.CapabilityExecutionResult{},
	}
}

func (c *NexusClient) Start(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if err := c.connectOnce(ctx); err != nil {
		return err
	}
	if c.AutoReconnect {
		go c.reconnectLoop(ctx)
	}
	return nil
}

func (c *NexusClient) Subscribe(buffer int) (<-chan core.FrameworkEvent, func()) {
	if buffer <= 0 {
		buffer = 8
	}
	ch := make(chan core.FrameworkEvent, buffer)
	c.mu.Lock()
	id := c.nextSubID
	c.nextSubID++
	if c.subscribers == nil {
		c.subscribers = map[int]chan core.FrameworkEvent{}
	}
	c.subscribers[id] = ch
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		if existing, ok := c.subscribers[id]; ok {
			delete(c.subscribers, id)
			close(existing)
		}
		c.mu.Unlock()
	}
}

func (c *NexusClient) Capabilities() []core.CapabilityDescriptor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]core.CapabilityDescriptor, len(c.capabilities))
	copy(out, c.capabilities)
	return out
}

func (c *NexusClient) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

func (c *NexusClient) InvokeCapability(ctx context.Context, sessionKey, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return nil, fmt.Errorf("nexus client not connected")
	}
	correlationID := randomCorrelationID()
	ch := make(chan core.CapabilityExecutionResult, 1)
	c.mu.Lock()
	c.pending[correlationID] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, correlationID)
		c.mu.Unlock()
	}()
	if err := conn.WriteJSON(map[string]any{
		"type":           "capability.invoke",
		"correlation_id": correlationID,
		"session_key":    sessionKey,
		"capability_id":  capabilityID,
		"args":           args,
	}); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		return &result, nil
	}
}

func (c *NexusClient) SendResponse(ctx context.Context, sessionKey, content string) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("nexus client not connected")
	}
	return conn.WriteJSON(map[string]any{
		"type":        "message.outbound",
		"session_key": sessionKey,
		"channel":     "nexus",
		"content": map[string]any{
			"text": content,
		},
	})
}

func (c *NexusClient) Close() error {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (c *NexusClient) connectOnce(ctx context.Context) error {
	if c.Dial == nil {
		c.Dial = dialNexusWebsocket
	}
	conn, err := c.Dial(ctx, c.Address, c.Token)
	if err != nil {
		return err
	}
	c.mu.RLock()
	lastSeenSeq := c.lastSeenSeq
	c.mu.RUnlock()
	if err := conn.WriteJSON(map[string]any{
		"type":          "connect",
		"version":       "1.0",
		"role":          "agent",
		"last_seen_seq": lastSeenSeq,
	}); err != nil {
		_ = conn.Close()
		return err
	}
	var connected struct {
		Type         string                      `json:"type"`
		SessionID    string                      `json:"session_id"`
		ServerSeq    uint64                      `json:"server_seq"`
		ReplayFrom   uint64                      `json:"replay_from"`
		Capabilities []core.CapabilityDescriptor `json:"capabilities,omitempty"`
	}
	if err := conn.ReadJSON(&connected); err != nil {
		_ = conn.Close()
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.sessionID = connected.SessionID
	c.lastSeenSeq = connected.ServerSeq
	c.capabilities = append([]core.CapabilityDescriptor(nil), connected.Capabilities...)
	onConnected := c.OnConnected
	caps := append([]core.CapabilityDescriptor(nil), c.capabilities...)
	c.mu.Unlock()
	if onConnected != nil {
		onConnected(caps)
	}
	go func() {
		_ = c.readLoop(ctx, conn)
	}()
	return nil
}

func (c *NexusClient) readLoop(ctx context.Context, conn nexusConn) error {
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return ctx.Err()
		default:
		}
		var frame map[string]json.RawMessage
		if err := conn.ReadJSON(&frame); err != nil {
			_ = conn.Close()
			c.mu.Lock()
			if c.conn == conn {
				c.conn = nil
			}
			c.mu.Unlock()
			return err
		}
		if err := c.handleFrame(frame); err != nil {
			return err
		}
	}
}

const (
	reconnectMinDelay = 200 * time.Millisecond
	reconnectMaxDelay = 30 * time.Second
)

func (c *NexusClient) reconnectLoop(ctx context.Context) {
	delay := reconnectMinDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		c.mu.RLock()
		connected := c.conn != nil
		c.mu.RUnlock()
		if connected {
			delay = reconnectMinDelay
			continue
		}
		if err := c.connectOnce(ctx); err != nil {
			// Exponential backoff capped at max, plus up to 25% jitter to
			// avoid thundering-herd when multiple clients reconnect together.
			delay = min(delay*2, reconnectMaxDelay)
			delay += time.Duration(rand.Int64N(int64(delay / 4)))
		} else {
			delay = reconnectMinDelay
		}
	}
}

func (c *NexusClient) handleFrame(frame map[string]json.RawMessage) error {
	var frameType string
	if raw, ok := frame["type"]; ok {
		if err := json.Unmarshal(raw, &frameType); err != nil {
			return err
		}
	}
	switch frameType {
	case "event":
		var eventFrame struct {
			Type   string              `json:"type"`
			Replay bool                `json:"replay"`
			Event  core.FrameworkEvent `json:"event"`
		}
		if err := remarshalNexusFrame(frame, &eventFrame); err != nil {
			return err
		}
		c.mu.Lock()
		if eventFrame.Event.Seq > c.lastSeenSeq {
			c.lastSeenSeq = eventFrame.Event.Seq
		}
		subscribers := make([]chan core.FrameworkEvent, 0, len(c.subscribers))
		for _, sub := range c.subscribers {
			subscribers = append(subscribers, sub)
		}
		c.mu.Unlock()
		for _, sub := range subscribers {
			select {
			case sub <- eventFrame.Event:
			default:
			}
		}
	case "capability.result":
		var response struct {
			Type          string                         `json:"type"`
			CorrelationID string                         `json:"correlation_id"`
			Result        core.CapabilityExecutionResult `json:"result"`
		}
		if err := remarshalNexusFrame(frame, &response); err != nil {
			return err
		}
		c.mu.RLock()
		ch := c.pending[response.CorrelationID]
		c.mu.RUnlock()
		if ch != nil {
			ch <- response.Result
		}
	}
	return nil
}

func remarshalNexusFrame(input any, out any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func randomCorrelationID() string {
	buf := make([]byte, 12)
	if _, err := cryptorand.Read(buf); err != nil {
		// Fallback: the collision probability over 12 random bytes is negligible;
		// a zero-entropy fallback is only reachable if the OS RNG is broken.
		panic("nexus: crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%x", buf)
}

func dialNexusWebsocket(ctx context.Context, address, token string) (nexusConn, error) {
	dialer := websocket.Dialer{}
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	conn, _, err := dialer.DialContext(ctx, address, header)
	return conn, err
}

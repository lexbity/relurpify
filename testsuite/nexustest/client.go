package nexustest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/gorilla/websocket"
)

type TestGatewayClient struct {
	conn *websocket.Conn

	mu           sync.RWMutex
	sessionID    string
	capabilities []core.CapabilityDescriptor

	events chan core.FrameworkEvent

	pendingMu sync.Mutex
	pending   map[string]chan capabilityResultFrame
	nextID    uint64
}

type connectedFrame struct {
	Type         string                      `json:"type"`
	SessionID    string                      `json:"session_id"`
	FeedScope    string                      `json:"feed_scope,omitempty"`
	Capabilities []core.CapabilityDescriptor `json:"capabilities,omitempty"`
}

type eventFrame struct {
	Type  string              `json:"type"`
	Event core.FrameworkEvent `json:"event"`
}

type capabilityResultFrame struct {
	Type          string                         `json:"type"`
	CorrelationID string                         `json:"correlation_id"`
	Result        core.CapabilityExecutionResult `json:"result"`
}

func NewTestGatewayClient() *TestGatewayClient {
	return &TestGatewayClient{
		events:  make(chan core.FrameworkEvent, 64),
		pending: make(map[string]chan capabilityResultFrame),
	}
}

func (c *TestGatewayClient) Connect(addr, token, role string) error {
	return c.ConnectWithLastSeenAndFeed(addr, token, role, 0, "")
}

func (c *TestGatewayClient) ConnectWithLastSeen(addr, token, role string, lastSeenSeq uint64) error {
	return c.ConnectWithLastSeenAndFeed(addr, token, role, lastSeenSeq, "")
}

func (c *TestGatewayClient) ConnectWithFeed(addr, token, role, feedScope string) error {
	return c.ConnectWithLastSeenAndFeed(addr, token, role, 0, feedScope)
}

func (c *TestGatewayClient) ConnectWithLastSeenAndFeed(addr, token, role string, lastSeenSeq uint64, feedScope string) error {
	if c == nil {
		return fmt.Errorf("gateway client required")
	}
	wsAddr := gatewayWSURL(addr)
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsAddr, headers)
	if err != nil {
		return err
	}
	connect := map[string]any{
		"type":          "connect",
		"version":       "1.0",
		"role":          role,
		"last_seen_seq": lastSeenSeq,
	}
	if strings.TrimSpace(feedScope) != "" {
		connect["feed_scope"] = strings.TrimSpace(feedScope)
	}
	if err := conn.WriteJSON(connect); err != nil {
		_ = conn.Close()
		return err
	}
	var frame connectedFrame
	if err := conn.ReadJSON(&frame); err != nil {
		_ = conn.Close()
		return err
	}
	if frame.Type != "connected" {
		_ = conn.Close()
		return fmt.Errorf("unexpected connect response %q", frame.Type)
	}

	c.conn = conn
	c.mu.Lock()
	c.sessionID = frame.SessionID
	c.capabilities = append([]core.CapabilityDescriptor(nil), frame.Capabilities...)
	c.mu.Unlock()
	go c.readLoop()
	return nil
}

func (c *TestGatewayClient) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

func (c *TestGatewayClient) Capabilities() []core.CapabilityDescriptor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]core.CapabilityDescriptor(nil), c.capabilities...)
}

func (c *TestGatewayClient) Events() <-chan core.FrameworkEvent {
	return c.events
}

func (c *TestGatewayClient) SendOutbound(sessionKey, text string) error {
	return c.conn.WriteJSON(map[string]any{
		"type":        "message.outbound",
		"session_key": sessionKey,
		"content": map[string]any{
			"text": text,
		},
	})
}

func (c *TestGatewayClient) InvokeCapability(ctx context.Context, sessionKey, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	correlationID := fmt.Sprintf("corr-%d", atomic.AddUint64(&c.nextID, 1))
	wait := make(chan capabilityResultFrame, 1)
	c.pendingMu.Lock()
	c.pending[correlationID] = wait
	c.pendingMu.Unlock()

	err := c.conn.WriteJSON(map[string]any{
		"type":           "capability.invoke",
		"correlation_id": correlationID,
		"session_key":    sessionKey,
		"capability_id":  capabilityID,
		"args":           args,
	})
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, correlationID)
		c.pendingMu.Unlock()
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, correlationID)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case frame, ok := <-wait:
		if !ok {
			return nil, fmt.Errorf("capability result channel closed")
		}
		result := frame.Result
		return &result, nil
	}
}

func (c *TestGatewayClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *TestGatewayClient) readLoop() {
	defer close(c.events)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.closePending()
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "event":
			var frame eventFrame
			if err := json.Unmarshal(data, &frame); err != nil {
				continue
			}
			c.events <- frame.Event
		case "capability.result":
			var frame capabilityResultFrame
			if err := json.Unmarshal(data, &frame); err != nil {
				continue
			}
			c.pendingMu.Lock()
			wait := c.pending[frame.CorrelationID]
			delete(c.pending, frame.CorrelationID)
			c.pendingMu.Unlock()
			if wait != nil {
				wait <- frame
				close(wait)
			}
		}
	}
}

func (c *TestGatewayClient) closePending() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, wait := range c.pending {
		close(wait)
		delete(c.pending, id)
	}
}

func gatewayWSURL(addr string) string {
	switch {
	case strings.HasPrefix(addr, "ws://"), strings.HasPrefix(addr, "wss://"):
		return addr
	case strings.HasPrefix(addr, "http://"):
		return "ws://" + strings.TrimPrefix(addr, "http://")
	case strings.HasPrefix(addr, "https://"):
		return "wss://" + strings.TrimPrefix(addr, "https://")
	default:
		return addr
	}
}

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type rpcConn interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
}

type WSConnection struct {
	Conn rpcConn

	Descriptor    NodeDescriptor
	HealthState   NodeHealth
	CapabilitySet []core.CapabilityDescriptor
	FrameHandler  func(context.Context, *WSConnection, map[string]json.RawMessage) error

	mu      sync.RWMutex
	pending map[string]chan invokeResponse
	closed  bool
}

type invokeRequest struct {
	Type          string         `json:"type"`
	CorrelationID string         `json:"correlation_id"`
	CapabilityID  string         `json:"capability_id"`
	Args          map[string]any `json:"args"`
}

type invokeResponse struct {
	Type          string                         `json:"type"`
	CorrelationID string                         `json:"correlation_id"`
	Result        core.CapabilityExecutionResult `json:"result"`
}

type healthFrame struct {
	Type   string          `json:"type"`
	Health NodeHealth `json:"health"`
}

const (
	TransportFrameType         = "transport.frame"
	TransportChannelControl    = "node.control"
	TransportChannelCapability = "node.capability"
	TransportChannelFMPControl = "fmp.control"
	TransportChannelFMPData    = "fmp.data"
)

type transportFrame struct {
	Type      string          `json:"type"`
	Channel   string          `json:"channel"`
	SessionID string          `json:"session_id,omitempty"`
	SentAt    time.Time       `json:"sent_at,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

type FramedRPCConn struct {
	Conn      rpcConn
	SessionID string
	Now       func() time.Time
}

func NewFramedRPCConn(conn rpcConn, sessionID string) *FramedRPCConn {
	return &FramedRPCConn{Conn: conn, SessionID: sessionID}
}

func (c *WSConnection) Node() NodeDescriptor { return c.Descriptor }

func (c *WSConnection) Health() NodeHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.HealthState
}

func (c *WSConnection) Capabilities() []core.CapabilityDescriptor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]core.CapabilityDescriptor, len(c.CapabilitySet))
	copy(out, c.CapabilitySet)
	return out
}

func (c *WSConnection) Invoke(ctx context.Context, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
	if c.Conn == nil {
		return nil, fmt.Errorf("node connection unavailable")
	}
	correlationID := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	ch := make(chan invokeResponse, 1)
	c.mu.Lock()
	if c.pending == nil {
		c.pending = map[string]chan invokeResponse{}
	}
	c.pending[correlationID] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, correlationID)
		c.mu.Unlock()
	}()

	if err := c.Conn.WriteJSON(invokeRequest{
		Type:          "capability.invoke",
		CorrelationID: correlationID,
		CapabilityID:  capabilityID,
		Args:          args,
	}); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-ch:
		return &response.Result, nil
	}
}

func (c *WSConnection) Close(_ context.Context) error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	if c.Conn == nil {
		return nil
	}
	return c.Conn.Close()
}

func (c *WSConnection) ReadLoop(ctx context.Context) error {
	if c.Conn == nil {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var frame map[string]json.RawMessage
		if err := c.Conn.ReadJSON(&frame); err != nil {
			return err
		}
		if err := c.handleFrame(ctx, frame); err != nil {
			return err
		}
	}
}

func (c *WSConnection) handleFrame(ctx context.Context, frame map[string]json.RawMessage) error {
	var frameType string
	if raw, ok := frame["type"]; ok {
		if err := json.Unmarshal(raw, &frameType); err != nil {
			return err
		}
	}
	switch frameType {
	case "capability.result":
		var response invokeResponse
		if err := remarshal(frame, &response); err != nil {
			return err
		}
		c.mu.RLock()
		ch := c.pending[response.CorrelationID]
		c.mu.RUnlock()
		if ch != nil {
			ch <- response
		}
	case string(core.FrameworkEventNodeHealth):
		var health healthFrame
		if err := remarshal(frame, &health); err != nil {
			return err
		}
		c.mu.Lock()
		c.HealthState = health.Health
		c.mu.Unlock()
	default:
		if c.FrameHandler != nil {
			return c.FrameHandler(ctx, c, frame)
		}
	}
	return nil
}

func (c *WSConnection) SendJSON(v any) error {
	if c == nil || c.Conn == nil {
		return fmt.Errorf("node connection unavailable")
	}
	return c.Conn.WriteJSON(v)
}

func remarshal(input any, out any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (c *FramedRPCConn) WriteJSON(v any) error {
	if c == nil || c.Conn == nil {
		return fmt.Errorf("framed rpc connection unavailable")
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	frame := transportFrame{
		Type:      TransportFrameType,
		Channel:   transportChannelForValue(v),
		SessionID: strings.TrimSpace(c.SessionID),
		Payload:   payload,
	}
	if c.Now != nil {
		frame.SentAt = c.Now().UTC()
	} else {
		frame.SentAt = time.Now().UTC()
	}
	return c.Conn.WriteJSON(frame)
}

func (c *FramedRPCConn) ReadJSON(v any) error {
	if c == nil || c.Conn == nil {
		return fmt.Errorf("framed rpc connection unavailable")
	}
	var raw map[string]json.RawMessage
	if err := c.Conn.ReadJSON(&raw); err != nil {
		return err
	}
	var frameType string
	if rawType, ok := raw["type"]; ok {
		if err := json.Unmarshal(rawType, &frameType); err != nil {
			return err
		}
	}
	if frameType != TransportFrameType {
		return remarshal(raw, v)
	}
	var frame transportFrame
	if err := remarshal(raw, &frame); err != nil {
		return err
	}
	if len(frame.Payload) == 0 {
		return fmt.Errorf("transport frame payload required")
	}
	return json.Unmarshal(frame.Payload, v)
}

func (c *FramedRPCConn) Close() error {
	if c == nil || c.Conn == nil {
		return nil
	}
	return c.Conn.Close()
}

func transportChannelForValue(v any) string {
	switch value := v.(type) {
	case invokeRequest, *invokeRequest, invokeResponse, *invokeResponse:
		return TransportChannelCapability
	case healthFrame, *healthFrame:
		return TransportChannelControl
	case map[string]json.RawMessage:
		return transportChannelForFrameType(frameTypeFromRaw(value))
	case map[string]any:
		if frameType, _ := value["type"].(string); frameType != "" {
			return transportChannelForFrameType(frameType)
		}
	}
	return TransportChannelControl
}

func frameTypeFromRaw(frame map[string]json.RawMessage) string {
	var frameType string
	if rawType, ok := frame["type"]; ok {
		_ = json.Unmarshal(rawType, &frameType)
	}
	return frameType
}

func transportChannelForFrameType(frameType string) string {
	switch {
	case strings.HasPrefix(frameType, "capability."):
		return TransportChannelCapability
	case strings.HasPrefix(frameType, "fmp.chunk."), strings.HasPrefix(frameType, "fmp.data."):
		return TransportChannelFMPData
	case strings.HasPrefix(frameType, "fmp."):
		return TransportChannelFMPControl
	default:
		return TransportChannelControl
	}
}

var _ Connection = (*WSConnection)(nil)
var _ rpcConn = (*FramedRPCConn)(nil)

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type rpcConn interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
	Close() error
}

type WSConnection struct {
	Conn rpcConn

	Descriptor    core.NodeDescriptor
	HealthState   core.NodeHealth
	CapabilitySet []core.CapabilityDescriptor

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
	Health core.NodeHealth `json:"health"`
}

func (c *WSConnection) Node() core.NodeDescriptor { return c.Descriptor }

func (c *WSConnection) Health() core.NodeHealth {
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
		if err := c.handleFrame(frame); err != nil {
			return err
		}
	}
}

func (c *WSConnection) handleFrame(frame map[string]json.RawMessage) error {
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
	}
	return nil
}

func remarshal(input any, out any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

var _ Connection = (*WSConnection)(nil)

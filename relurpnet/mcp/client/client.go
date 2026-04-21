package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	msession "codeburg.org/lexbit/relurpify/relurpnet/mcp/session"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/transport/stdio"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/versioning"
)

type StdioConfig struct {
	Command           string
	Args              []string
	Dir               string
	Env               []string
	ProviderID        string
	SessionID         string
	RemoteTarget      string
	LocalPeer         protocol.PeerInfo
	Capabilities      map[string]any
	PreferredVersions []string
	Recoverable       bool
	Policy            sandbox.CommandPolicy
}

type NotificationHandler func(method string)

type RequestHandler interface {
	HandleSamplingRequest(ctx context.Context, params protocol.CreateMessageParams) (*protocol.CreateMessageResult, error)
	HandleElicitationRequest(ctx context.Context, params protocol.ElicitationParams) (*protocol.ElicitationResult, error)
}

type Client struct {
	transport *stdio.Transport
	matrix    versioning.SupportMatrix
	session   *msession.Session

	mu         sync.Mutex
	pending    map[string]chan rpcResponse
	nextID     atomic.Int64
	closed     bool
	handler    NotificationHandler
	requests   RequestHandler
	readerDone chan error
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

func ConnectStdio(ctx context.Context, launcher stdio.Launcher, cfg StdioConfig) (*Client, error) {
	matrix := versioning.DefaultSupportMatrix()
	requested, err := matrix.ChooseRevision(cfg.PreferredVersions)
	if err != nil {
		return nil, err
	}
	session, err := msession.NewClientSession(msession.Config{
		ProviderID:        cfg.ProviderID,
		SessionID:         cfg.SessionID,
		TransportKind:     "stdio",
		RemoteTarget:      firstNonEmpty(cfg.RemoteTarget, cfg.Command),
		LocalPeer:         cfg.LocalPeer,
		RequestedVersion:  requested,
		LocalCapabilities: cfg.Capabilities,
		Recoverable:       cfg.Recoverable,
	})
	if err != nil {
		return nil, err
	}
	transport, err := stdio.Open(ctx, launcher, stdio.Config{
		Command: cfg.Command,
		Args:    cfg.Args,
		Dir:     cfg.Dir,
		Env:     cfg.Env,
		Policy:  cfg.Policy,
	})
	if err != nil {
		return nil, err
	}
	client := &Client{
		transport:  transport,
		matrix:     matrix,
		session:    session,
		pending:    make(map[string]chan rpcResponse),
		readerDone: make(chan error, 1),
	}
	go client.readLoop()
	if err := client.session.MarkTransportEstablished(); err != nil {
		_ = client.Close()
		return nil, err
	}
	var initResult protocol.InitializeResult
	if err := client.call(ctx, "initialize", protocol.InitializeRequest{
		ProtocolVersion: requested,
		ClientInfo:      cfg.LocalPeer,
		Capabilities:    cfg.Capabilities,
	}, &initResult); err != nil {
		_ = client.Close()
		return nil, err
	}
	if _, err := matrix.Negotiate(initResult.ProtocolVersion, []string{requested}); err != nil {
		_ = client.session.Fail(err.Error())
		_ = client.Close()
		return nil, err
	}
	if err := client.session.ApplyInitializeResult(initResult); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) SetNotificationHandler(handler NotificationHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

func (c *Client) SetRequestHandler(handler RequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = handler
}

func (c *Client) SessionSnapshot() msession.Snapshot {
	if c == nil || c.session == nil {
		return msession.Snapshot{}
	}
	return c.session.Snapshot()
}

func (c *Client) ListTools(ctx context.Context) ([]protocol.Tool, error) {
	var result protocol.ListToolsResult
	if err := c.call(ctx, "tools/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

func (c *Client) ListPrompts(ctx context.Context) ([]protocol.Prompt, error) {
	var result protocol.ListPromptsResult
	if err := c.call(ctx, "prompts/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	return result.Prompts, nil
}

func (c *Client) ListResources(ctx context.Context) ([]protocol.Resource, error) {
	var result protocol.ListResourcesResult
	if err := c.call(ctx, "resources/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	return result.Resources, nil
}

func (c *Client) CallTool(ctx context.Context, params protocol.CallToolParams) (*protocol.CallToolResult, error) {
	var result protocol.CallToolResult
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ReadResource(ctx context.Context, uri string) (*protocol.ReadResourceResult, error) {
	var result protocol.ReadResourceResult
	if err := c.call(ctx, "resources/read", protocol.ReadResourceParams{URI: uri}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) SubscribeResource(ctx context.Context, uri string) error {
	if err := c.call(ctx, "resources/subscribe", protocol.ResourceSubscribeParams{URI: uri}, nil); err != nil {
		return err
	}
	if c.session != nil {
		return c.session.SetSubscription(uri, true)
	}
	return nil
}

func (c *Client) UnsubscribeResource(ctx context.Context, uri string) error {
	if err := c.call(ctx, "resources/unsubscribe", protocol.ResourceSubscribeParams{URI: uri}, nil); err != nil {
		return err
	}
	if c.session != nil {
		return c.session.SetSubscription(uri, false)
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	pending := c.pending
	c.pending = map[string]chan rpcResponse{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- rpcResponse{err: context.Canceled}
	}
	if c.session != nil {
		_ = c.session.BeginClose()
	}
	err := c.transport.Close()
	if c.session != nil {
		c.session.MarkClosed()
	}
	return err
}

func (c *Client) Wait() error {
	if c == nil {
		return nil
	}
	return <-c.readerDone
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	if c == nil {
		return fmt.Errorf("client unavailable")
	}
	id := fmt.Sprintf("%d", c.nextID.Add(1))
	respCh := make(chan rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("client closed")
	}
	c.pending[id] = respCh
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	if c.session != nil {
		_ = c.session.UpdateRequestCount(1)
		defer func() { _ = c.session.UpdateRequestCount(-1) }()
	}
	if err := c.writeEnvelope(rpcEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  mustMarshal(params),
	}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-respCh:
		if resp.err != nil {
			return resp.err
		}
		if result == nil || len(resp.result) == 0 {
			return nil
		}
		return json.Unmarshal(resp.result, result)
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return c.writeEnvelope(rpcEnvelope{
			JSONRPC: "2.0",
			Method:  method,
			Params:  mustMarshal(params),
		})
	}
}

func (c *Client) writeEnvelope(envelope rpcEnvelope) error {
	encoder := json.NewEncoder(c.transport.Writer())
	return encoder.Encode(envelope)
}

func (c *Client) readLoop() {
	reader := bufio.NewReader(c.transport.Reader())
	decoder := json.NewDecoder(reader)
	for {
		var envelope rpcEnvelope
		if err := decoder.Decode(&envelope); err != nil {
			if c.session != nil && !errorsIsEOF(err) {
				_ = c.session.Fail(err.Error())
			}
			c.failPending(err)
			c.readerDone <- err
			return
		}
		if envelope.ID != "" && envelope.Method == "" {
			c.mu.Lock()
			respCh := c.pending[envelope.ID]
			c.mu.Unlock()
			if respCh == nil {
				continue
			}
			if envelope.Error != nil {
				respCh <- rpcResponse{err: fmt.Errorf("%s", envelope.Error.Message)}
				continue
			}
			respCh <- rpcResponse{result: envelope.Result}
			continue
		}
		if envelope.Method != "" && envelope.ID != "" {
			c.handleServerRequest(envelope)
			continue
		}
		if envelope.Method != "" && envelope.ID == "" {
			c.mu.Lock()
			handler := c.handler
			c.mu.Unlock()
			if handler != nil {
				go handler(envelope.Method)
			}
		}
	}
}

func (c *Client) handleServerRequest(envelope rpcEnvelope) {
	c.mu.Lock()
	handler := c.requests
	c.mu.Unlock()
	if handler == nil {
		_ = c.writeEnvelope(rpcEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   &rpcError{Code: -32601, Message: "method not supported"},
		})
		return
	}
	switch envelope.Method {
	case "sampling/createMessage":
		var params protocol.CreateMessageParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			_ = c.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32602, Message: err.Error()}})
			return
		}
		result, err := handler.HandleSamplingRequest(context.Background(), params)
		if err != nil {
			_ = c.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32010, Message: err.Error()}})
			return
		}
		_ = c.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: mustMarshal(result)})
	case "elicitation/create":
		var params protocol.ElicitationParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			_ = c.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32602, Message: err.Error()}})
			return
		}
		result, err := handler.HandleElicitationRequest(context.Background(), params)
		if err != nil {
			_ = c.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Error: &rpcError{Code: -32011, Message: err.Error()}})
			return
		}
		_ = c.writeEnvelope(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: mustMarshal(result)})
	default:
		_ = c.writeEnvelope(rpcEnvelope{
			JSONRPC: "2.0",
			ID:      envelope.ID,
			Error:   &rpcError{Code: -32601, Message: "method not supported"},
		})
	}
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- rpcResponse{err: err}
		delete(c.pending, id)
	}
}

func mustMarshal(value any) json.RawMessage {
	if value == nil {
		return json.RawMessage(`{}`)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func errorsIsEOF(err error) bool {
	return err == io.EOF || strings.Contains(strings.ToLower(err.Error()), "closed")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

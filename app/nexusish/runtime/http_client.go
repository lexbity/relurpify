package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
)

type httpMCPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	sessionID  string
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

func newHTTPMCPClient(ctx context.Context, baseURL, token string) (*httpMCPClient, error) {
	client := &httpMCPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{},
	}
	var init protocol.InitializeResult
	if err := client.call(ctx, "initialize", protocol.InitializeRequest{
		ProtocolVersion: protocol.Revision20250618,
		ClientInfo:      protocol.PeerInfo{Name: "nexusish", Version: "v1alpha1"},
	}, &init); err != nil {
		return nil, err
	}
	if err := client.notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *httpMCPClient) CallTool(ctx context.Context, params protocol.CallToolParams) (*protocol.CallToolResult, error) {
	var result protocol.CallToolResult
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *httpMCPClient) ReadResource(ctx context.Context, uri string) (*protocol.ReadResourceResult, error) {
	var result protocol.ReadResourceResult
	if err := c.call(ctx, "resources/read", protocol.ReadResourceParams{URI: uri}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *httpMCPClient) Close() error {
	if c.sessionID == "" {
		return nil
	}
	req, err := http.NewRequest(http.MethodDelete, c.baseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Mcp-Session-Id", c.sessionID)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	_, err = c.httpClient.Do(req)
	return err
}

func (c *httpMCPClient) call(ctx context.Context, method string, params any, result any) error {
	data, err := json.Marshal(rpcEnvelope{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  method,
		Params:  mustJSON(params),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if sessionID := resp.Header.Get("Mcp-Session-Id"); sessionID != "" {
		c.sessionID = sessionID
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mcp http %d", resp.StatusCode)
	}
	var envelope rpcEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return fmt.Errorf("%s", envelope.Error.Message)
	}
	if result != nil {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}

func (c *httpMCPClient) notify(ctx context.Context, method string, params any) error {
	data, err := json.Marshal(rpcEnvelope{
		JSONRPC: "2.0",
		Method:  method,
		Params:  mustJSON(params),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", c.sessionID)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mcp http %d", resp.StatusCode)
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

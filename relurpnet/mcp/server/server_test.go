package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	"github.com/stretchr/testify/require"
)

type stubExporter struct{}

func (stubExporter) ListTools(context.Context) ([]protocol.Tool, error) {
	return []protocol.Tool{{Name: "echo"}}, nil
}
func (stubExporter) CallTool(_ context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	return &protocol.CallToolResult{StructuredContent: map[string]any{"tool": name, "echo": args["message"]}}, nil
}
func (stubExporter) ListPrompts(context.Context) ([]protocol.Prompt, error) {
	return []protocol.Prompt{{Name: "summary"}}, nil
}
func (stubExporter) GetPrompt(_ context.Context, name string, _ map[string]any) (*protocol.GetPromptResult, error) {
	return &protocol.GetPromptResult{Messages: []protocol.ContentBlock{{Type: "text", Text: "prompt:" + name}}}, nil
}
func (stubExporter) ListResources(context.Context) ([]protocol.Resource, error) {
	return []protocol.Resource{{URI: "relurpify://capability/resource/docs"}}, nil
}
func (stubExporter) ReadResource(_ context.Context, uri string) (*protocol.ReadResourceResult, error) {
	return &protocol.ReadResourceResult{Contents: []protocol.ContentBlock{{Type: "text", Text: uri}}}, nil
}

func TestServeConnHandlesInitializeListAndCall(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	svc := New(protocol.PeerInfo{Name: "relurpify", Version: "dev"}, stubExporter{}, Hooks{})
	go func() {
		_ = svc.ServeConn(context.Background(), "peer-1", serverSide)
	}()

	dec := json.NewDecoder(bufio.NewReader(clientSide))
	enc := json.NewEncoder(clientSide)

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "initialize",
		"params": protocol.InitializeRequest{ProtocolVersion: protocol.Revision20250618},
	}))
	var initResp map[string]any
	require.NoError(t, dec.Decode(&initResp))
	require.Equal(t, "1", initResp["id"])

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{},
	}))
	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "id": "2", "method": "tools/list", "params": map[string]any{}}))
	var listResp struct {
		ID     string                   `json:"id"`
		Result protocol.ListToolsResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&listResp))
	require.Equal(t, "echo", listResp.Result.Tools[0].Name)

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "3", "method": "tools/call",
		"params": protocol.CallToolParams{Name: "echo", Arguments: map[string]any{"message": "hi"}},
	}))
	var callResp struct {
		ID     string                  `json:"id"`
		Result protocol.CallToolResult `json:"result"`
	}
	require.NoError(t, dec.Decode(&callResp))
	require.Equal(t, "hi", callResp.Result.StructuredContent["echo"])
}

func TestServeConnRejectsRequestsBeforeInitialized(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	svc := New(protocol.PeerInfo{Name: "relurpify", Version: "dev"}, stubExporter{}, Hooks{})
	go func() { _ = svc.ServeConn(context.Background(), "peer-1", serverSide) }()

	dec := json.NewDecoder(bufio.NewReader(clientSide))
	enc := json.NewEncoder(clientSide)
	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "initialize",
		"params": protocol.InitializeRequest{ProtocolVersion: protocol.Revision20250618},
	}))
	var discard map[string]any
	require.NoError(t, dec.Decode(&discard))

	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "id": "2", "method": "tools/list", "params": map[string]any{}}))
	var resp struct {
		ID    string `json:"id"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, dec.Decode(&resp))
	require.Equal(t, "2", resp.ID)
	require.Equal(t, "session not initialized", resp.Error.Message)
}

func TestServeConnCallsHooks(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	opened := make(chan string, 1)
	initialized := make(chan string, 1)
	closed := make(chan string, 1)
	svc := New(protocol.PeerInfo{Name: "relurpify"}, stubExporter{}, Hooks{
		OnSessionOpen: func(sessionID, requested string) {
			opened <- fmt.Sprintf("%s:%s", sessionID, requested)
		},
		OnSessionInitialized: func(sessionID, negotiated string, _ protocol.PeerInfo) {
			initialized <- fmt.Sprintf("%s:%s", sessionID, negotiated)
		},
		OnSessionClosed: func(sessionID string, err error) {
			closed <- sessionID
		},
	})
	go func() { _ = svc.ServeConn(context.Background(), "peer-1", serverSide) }()
	enc := json.NewEncoder(clientSide)
	dec := json.NewDecoder(bufio.NewReader(clientSide))
	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "initialize",
		"params": protocol.InitializeRequest{ProtocolVersion: protocol.Revision20250618},
	}))
	var discard map[string]any
	require.NoError(t, dec.Decode(&discard))
	require.NoError(t, enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}}))
	_ = clientSide.Close()
	require.Equal(t, "peer-1:"+protocol.Revision20250618, <-opened)
	require.Equal(t, "peer-1:"+protocol.Revision20250618, <-initialized)
	require.Eventually(t, func() bool {
		select {
		case <-closed:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestServeHTTPHandlesInitializeSessionLifecycleAndList(t *testing.T) {
	closed := make(chan string, 1)
	svc := New(protocol.PeerInfo{Name: "relurpify", Version: "dev"}, stubExporter{}, Hooks{
		OnSessionClosed: func(sessionID string, err error) {
			closed <- sessionID
		},
	})

	initReq := httptest.NewRequest(http.MethodPost, "/mcp", encodeEnvelope(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params": protocol.InitializeRequest{
			ProtocolVersion: protocol.Revision20250618,
			ClientInfo:      protocol.PeerInfo{Name: "client"},
		},
	}))
	initResp := httptest.NewRecorder()
	svc.ServeHTTP(initResp, initReq)
	require.Equal(t, http.StatusOK, initResp.Code)
	sessionID := initResp.Header().Get(SessionHeader)
	require.NotEmpty(t, sessionID)

	initializedReq := httptest.NewRequest(http.MethodPost, "/mcp", encodeEnvelope(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}))
	initializedReq.Header.Set(SessionHeader, sessionID)
	initializedResp := httptest.NewRecorder()
	svc.ServeHTTP(initializedResp, initializedReq)
	require.Equal(t, http.StatusAccepted, initializedResp.Code)

	listReq := httptest.NewRequest(http.MethodPost, "/mcp", encodeEnvelope(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "tools/list",
		"params":  map[string]any{},
	}))
	listReq.Header.Set(SessionHeader, sessionID)
	listResp := httptest.NewRecorder()
	svc.ServeHTTP(listResp, listReq)
	require.Equal(t, http.StatusOK, listResp.Code)
	var list struct {
		ID     string                   `json:"id"`
		Result protocol.ListToolsResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &list))
	require.Equal(t, "echo", list.Result.Tools[0].Name)

	closeReq := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	closeReq.Header.Set(SessionHeader, sessionID)
	closeResp := httptest.NewRecorder()
	svc.ServeHTTP(closeResp, closeReq)
	require.Equal(t, http.StatusNoContent, closeResp.Code)
	require.Equal(t, sessionID, <-closed)
}

func TestServeHTTPRejectsRequestsBeforeInitialized(t *testing.T) {
	svc := New(protocol.PeerInfo{Name: "relurpify", Version: "dev"}, stubExporter{}, Hooks{})
	initReq := httptest.NewRequest(http.MethodPost, "/mcp", encodeEnvelope(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params": protocol.InitializeRequest{
			ProtocolVersion: protocol.Revision20250618,
		},
	}))
	initResp := httptest.NewRecorder()
	svc.ServeHTTP(initResp, initReq)
	sessionID := initResp.Header().Get(SessionHeader)

	listReq := httptest.NewRequest(http.MethodPost, "/mcp", encodeEnvelope(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "tools/list",
		"params":  map[string]any{},
	}))
	listReq.Header.Set(SessionHeader, sessionID)
	listResp := httptest.NewRecorder()
	svc.ServeHTTP(listResp, listReq)
	var resp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &resp))
	require.Equal(t, "session not initialized", resp.Error.Message)
}

func TestServeConnResourceSubscriptionsReceiveUpdateNotifications(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	svc := New(protocol.PeerInfo{Name: "relurpify", Version: "dev"}, stubExporter{}, Hooks{})
	go func() { _ = svc.ServeConn(context.Background(), "peer-1", serverSide) }()

	dec := json.NewDecoder(bufio.NewReader(clientSide))
	enc := json.NewEncoder(clientSide)

	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "initialize",
		"params": protocol.InitializeRequest{ProtocolVersion: protocol.Revision20250618},
	}))
	var discard map[string]any
	require.NoError(t, dec.Decode(&discard))
	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{},
	}))
	require.NoError(t, enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": "2", "method": "resources/subscribe",
		"params": protocol.ResourceSubscribeParams{URI: "relurpify://capability/resource/docs"},
	}))
	require.NoError(t, dec.Decode(&discard))

	go func() {
		_ = svc.NotifyResourceUpdated("relurpify://capability/resource/docs")
	}()

	var notification struct {
		Method string                         `json:"method"`
		Params protocol.ResourceUpdatedParams `json:"params"`
	}
	require.NoError(t, dec.Decode(&notification))
	require.Equal(t, "notifications/resources/updated", notification.Method)
	require.Equal(t, "relurpify://capability/resource/docs", notification.Params.URI)
}

func encodeEnvelope(t *testing.T, payload map[string]any) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return strings.NewReader(string(data))
}

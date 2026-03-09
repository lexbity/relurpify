package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/mcp/protocol"
	"github.com/lexcodex/relurpify/framework/mcp/transport/stdio"
	"github.com/stretchr/testify/require"
)

type pipeProcess struct {
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stderrR *io.PipeReader
	waitCh  chan error
	pid     int
}

func (p *pipeProcess) Stdin() io.WriteCloser { return p.stdinW }
func (p *pipeProcess) Stdout() io.ReadCloser { return p.stdoutR }
func (p *pipeProcess) Stderr() io.ReadCloser { return p.stderrR }
func (p *pipeProcess) PID() int              { return p.pid }
func (p *pipeProcess) Wait() error           { return <-p.waitCh }
func (p *pipeProcess) Kill() error {
	select {
	case p.waitCh <- context.Canceled:
	default:
	}
	return nil
}

type pipeLauncher struct {
	process stdio.Process
}

func (l pipeLauncher) Launch(context.Context, stdio.Config) (stdio.Process, error) {
	return l.process, nil
}

func TestConnectStdioNegotiatesAndDiscovers(t *testing.T) {
	process, controller := newFixtureProcess()
	defer controller.close()
	client, err := ConnectStdio(context.Background(), pipeLauncher{process: process}, StdioConfig{
		Command:      "fixture-mcp",
		ProviderID:   "remote-mcp",
		SessionID:    "remote-mcp:primary",
		LocalPeer:    protocol.PeerInfo{Name: "relurpify", Version: "dev"},
		Capabilities: map[string]any{"roots": map[string]any{"listChanged": true}},
	})
	require.NoError(t, err)
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "remote.echo", tools[0].Name)

	prompts, err := client.ListPrompts(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 1)

	resources, err := client.ListResources(context.Background())
	require.NoError(t, err)
	require.Len(t, resources, 1)

	result, err := client.CallTool(context.Background(), protocol.CallToolParams{
		Name:      "remote.echo",
		Arguments: map[string]any{"message": "hello"},
	})
	require.NoError(t, err)
	require.Equal(t, "hello", result.StructuredContent["echo"])
	require.Equal(t, protocol.Revision20250618, client.SessionSnapshot().NegotiatedVersion)
}

func TestClientNotificationHandlerReceivesListChanged(t *testing.T) {
	process, controller := newFixtureProcess()
	defer controller.close()
	client, err := ConnectStdio(context.Background(), pipeLauncher{process: process}, StdioConfig{
		Command:    "fixture-mcp",
		ProviderID: "remote-mcp",
		SessionID:  "remote-mcp:primary",
		LocalPeer:  protocol.PeerInfo{Name: "relurpify", Version: "dev"},
	})
	require.NoError(t, err)
	defer client.Close()

	ch := make(chan string, 1)
	client.SetNotificationHandler(func(method string) {
		ch <- method
	})
	controller.setTools([]protocol.Tool{{Name: "remote.echo"}, {Name: "remote.search"}})
	require.NoError(t, controller.notify("notifications/tools/list_changed", map[string]any{}))

	select {
	case method := <-ch:
		require.Equal(t, "notifications/tools/list_changed", method)
	case <-time.After(time.Second):
		t.Fatal("expected notification")
	}
}

type fixtureController struct {
	mu        sync.Mutex
	encoder   *json.Encoder
	tools     []protocol.Tool
	prompts   []protocol.Prompt
	resources []protocol.Resource
	reqID     int
}

func newFixtureProcess() (stdio.Process, *fixtureController) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	_ = stderrW.Close()
	controller := &fixtureController{
		encoder: json.NewEncoder(serverToClientW),
		tools: []protocol.Tool{{
			Name:        "remote.echo",
			Description: "echo",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			},
		}},
		prompts:   []protocol.Prompt{{Name: "draft.summary", Description: "summary"}},
		resources: []protocol.Resource{{URI: "file:///tmp/catalog.json", Name: "catalog"}},
	}
	go controller.serve(clientToServerR)
	return &pipeProcess{
		stdinW:  clientToServerW,
		stdoutR: serverToClientR,
		stderrR: stderrR,
		waitCh:  make(chan error, 1),
		pid:     1001,
	}, controller
}

func (f *fixtureController) serve(reader io.Reader) {
	decoder := json.NewDecoder(bufio.NewReader(reader))
	for {
		var envelope rpcEnvelope
		if err := decoder.Decode(&envelope); err != nil {
			return
		}
		switch envelope.Method {
		case "initialize":
			var req protocol.InitializeRequest
			_ = json.Unmarshal(envelope.Params, &req)
			_ = f.encoder.Encode(rpcEnvelope{
				JSONRPC: "2.0",
				ID:      envelope.ID,
				Result: mustMarshal(protocol.InitializeResult{
					ProtocolVersion: protocol.Revision20250618,
					ServerInfo:      protocol.PeerInfo{Name: "fixture-mcp", Version: "1.0.0"},
					Capabilities: map[string]any{
						"tools":     map[string]any{"listChanged": true},
						"resources": map[string]any{"subscribe": true},
					},
				}),
			})
		case "tools/list":
			f.mu.Lock()
			tools := append([]protocol.Tool(nil), f.tools...)
			f.mu.Unlock()
			_ = f.encoder.Encode(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: mustMarshal(protocol.ListToolsResult{Tools: tools})})
		case "prompts/list":
			f.mu.Lock()
			prompts := append([]protocol.Prompt(nil), f.prompts...)
			f.mu.Unlock()
			_ = f.encoder.Encode(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: mustMarshal(protocol.ListPromptsResult{Prompts: prompts})})
		case "resources/list":
			f.mu.Lock()
			resources := append([]protocol.Resource(nil), f.resources...)
			f.mu.Unlock()
			_ = f.encoder.Encode(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: mustMarshal(protocol.ListResourcesResult{Resources: resources})})
		case "tools/call":
			var params protocol.CallToolParams
			_ = json.Unmarshal(envelope.Params, &params)
			_ = f.encoder.Encode(rpcEnvelope{
				JSONRPC: "2.0",
				ID:      envelope.ID,
				Result: mustMarshal(protocol.CallToolResult{
					StructuredContent: map[string]any{"echo": params.Arguments["message"]},
					Content:           []protocol.ContentBlock{{Type: "text", Text: fmt.Sprint(params.Arguments["message"])}},
				}),
			})
		case "resources/subscribe", "resources/unsubscribe":
			_ = f.encoder.Encode(rpcEnvelope{JSONRPC: "2.0", ID: envelope.ID, Result: mustMarshal(map[string]any{})})
		case "notifications/initialized":
		}
	}
}

func (f *fixtureController) notify(method string, params map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.encoder.Encode(rpcEnvelope{JSONRPC: "2.0", Method: method, Params: mustMarshal(params)})
}

func (f *fixtureController) setTools(tools []protocol.Tool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tools = append([]protocol.Tool(nil), tools...)
}

func (f *fixtureController) close() {}

type requestHandlerStub struct {
	sampling    *protocol.CreateMessageResult
	elicitation *protocol.ElicitationResult
}

func (r requestHandlerStub) HandleSamplingRequest(context.Context, protocol.CreateMessageParams) (*protocol.CreateMessageResult, error) {
	return r.sampling, nil
}

func (r requestHandlerStub) HandleElicitationRequest(context.Context, protocol.ElicitationParams) (*protocol.ElicitationResult, error) {
	return r.elicitation, nil
}

func (f *fixtureController) request(method string, params any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqID++
	return f.encoder.Encode(rpcEnvelope{
		JSONRPC: "2.0",
		ID:      fmt.Sprintf("srv-%d", f.reqID),
		Method:  method,
		Params:  mustMarshal(params),
	})
}

func TestClientHandlesSamplingAndElicitationRequests(t *testing.T) {
	process, controller := newFixtureProcess()
	defer controller.close()
	client, err := ConnectStdio(context.Background(), pipeLauncher{process: process}, StdioConfig{
		Command:    "fixture-mcp",
		ProviderID: "remote-mcp",
		SessionID:  "remote-mcp:primary",
		LocalPeer:  protocol.PeerInfo{Name: "relurpify", Version: "dev"},
	})
	require.NoError(t, err)
	defer client.Close()

	client.SetRequestHandler(requestHandlerStub{
		sampling: &protocol.CreateMessageResult{
			Role:    "assistant",
			Content: protocol.ContentBlock{Type: "text", Text: "sampled"},
		},
		elicitation: &protocol.ElicitationResult{
			Action:  "accept",
			Content: map[string]any{"topic": "MCP"},
		},
	})

	require.NoError(t, controller.request("sampling/createMessage", protocol.CreateMessageParams{
		Messages: []protocol.SamplingMessage{{Role: "user", Content: protocol.ContentBlock{Type: "text", Text: "hello"}}},
	}))
	require.NoError(t, controller.request("elicitation/create", protocol.ElicitationParams{
		Message: "Need details",
	}))
}

func TestClientSubscribeAndUnsubscribeResourceUpdatesSession(t *testing.T) {
	process, controller := newFixtureProcess()
	defer controller.close()
	client, err := ConnectStdio(context.Background(), pipeLauncher{process: process}, StdioConfig{
		Command:    "fixture-mcp",
		ProviderID: "remote-mcp",
		SessionID:  "remote-mcp:primary",
		LocalPeer:  protocol.PeerInfo{Name: "relurpify", Version: "dev"},
	})
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, client.SubscribeResource(context.Background(), "file:///tmp/catalog.json"))
	require.ElementsMatch(t, []string{"file:///tmp/catalog.json"}, client.SessionSnapshot().ActiveSubscriptions)
	require.NoError(t, client.UnsubscribeResource(context.Background(), "file:///tmp/catalog.json"))
	require.Empty(t, client.SessionSnapshot().ActiveSubscriptions)
}

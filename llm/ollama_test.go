package llm

import (
	"context"
	"encoding/json"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

type stubTool struct {
	name string
}

func (t stubTool) Name() string        { return t.name }
func (t stubTool) Description() string { return "stub tool" }
func (t stubTool) Category() string    { return "test" }
func (t stubTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "value", Type: "string", Required: false},
	}
}
func (t stubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"echo": args["value"],
		},
	}, nil
}
func (t stubTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }
func (t stubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "**"},
		},
	}}
}
func (t stubTool) Tags() []string { return nil }

func TestClientGenerate(t *testing.T) {
	client := NewClient("http://fake", "test")
	client.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			assert.Equal(t, "/api/generate", req.URL.Path)
			var payload map[string]interface{}
			assert.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
			assert.Equal(t, "hello", payload["prompt"])
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"text":"response"}`)),
				Header:     make(http.Header),
			}
		}),
	}

	resp, err := client.Generate(context.Background(), "hello", &core.LLMOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "response", resp.Text)
}

func TestClientChat(t *testing.T) {
	client := NewClient("http://fake", "chat-model")
	client.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			assert.Equal(t, "/api/chat", req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"text":"ok"}`)),
				Header:     make(http.Header),
			}
		}),
	}

	resp, err := client.Chat(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp.Text)
}

func TestClientChatTrimsOpenAIv1Endpoint(t *testing.T) {
	client := NewClient("http://fake/v1", "chat-model")
	client.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			assert.Equal(t, "/api/chat", req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"text":"ok"}`)),
				Header:     make(http.Header),
			}
		}),
	}

	resp, err := client.Chat(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp.Text)
}

func TestClientChatWithToolsParsesToolCalls(t *testing.T) {
	client := NewClient("http://fake", "model")
	client.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			assert.Equal(t, "/api/chat", req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(strings.NewReader(`{
					"message": {
						"role":"assistant",
						"content":"",
						"tool_calls": [{
							"id":"call-1",
							"type":"function",
							"function":{"name":"echo","arguments":"{\"value\":\"hi\"}"}
						}]
					},
					"done_reason":"tool_calls"
				}`)),
				Header: make(http.Header),
			}
		}),
	}

	tools := []core.Tool{stubTool{name: "echo"}}
	messages := []core.Message{
		{Role: "user", Content: "say hi"},
	}
	resp, err := client.ChatWithTools(context.Background(), messages, tools, &core.LLMOptions{})
	assert.NoError(t, err)
	if assert.Len(t, resp.ToolCalls, 1) {
		assert.Equal(t, "echo", resp.ToolCalls[0].Name)
		assert.Equal(t, map[string]interface{}{"value": "hi"}, resp.ToolCalls[0].Args)
	}
}

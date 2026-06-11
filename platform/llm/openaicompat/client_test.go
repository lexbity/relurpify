package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/model"
)

func TestChat_Sync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "model-a", payload["model"])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "hello"}, "finish_reason": "stop"},
			},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2},
		})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "token")
	resp, err := client.Chat(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, &model.LLMOptions{Model: "model-a"})
	require.NoError(t, err)
	assert.Equal(t, "hello", resp.Text)
	assert.Equal(t, "stop", resp.FinishReason)
	assert.Equal(t, 3, resp.Usage.PromptTokens)
	assert.Equal(t, 2, resp.Usage.CompletionTokens)
	assert.Equal(t, 5, resp.Usage.TotalTokens)
	assert.False(t, resp.Usage.Estimated)
}

func TestChat_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hel"}}]}`)
		flusher.Flush()
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}]}`)
		flusher.Flush()
		_, _ = fmt.Fprintln(w, `data: [DONE]`)
		flusher.Flush()
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")
	var got strings.Builder
	resp, err := client.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, nil, &model.LLMOptions{
		StreamCallback: func(token string) { got.WriteString(token) },
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", got.String())
	assert.Equal(t, "hello", resp.Text)
}

func TestChatWithTools_NativeEnabled_Sync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Contains(t, payload, "tools")
		tools := payload["tools"].([]any)
		assert.Len(t, tools, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   "call-1",
								"type": "function",
								"function": map[string]any{
									"name":      "echo",
									"arguments": `{"value":"hi"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL, NativeToolCalling: true}, "")
	resp, err := client.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, []model.LLMToolSpec{{Name: "echo"}}, nil)
	require.NoError(t, err)
	assert.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "echo", resp.ToolCalls[0].Name)
	assert.Equal(t, map[string]any{"value": "hi"}, resp.ToolCalls[0].Args)
}

func TestChatWithTools_NativeEnabled_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"echo","arguments":"{\"value\":\""}}]}}]}`)
		flusher.Flush()
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"hi\"}"}}]}}]}`)
		flusher.Flush()
		_, _ = fmt.Fprintln(w, `data: [DONE]`)
		flusher.Flush()
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL, NativeToolCalling: true}, "")
	var got strings.Builder
	resp, err := client.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, []model.LLMToolSpec{{Name: "echo"}}, &model.LLMOptions{
		StreamCallback: func(token string) { got.WriteString(token) },
	})
	require.NoError(t, err)
	assert.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "echo", resp.ToolCalls[0].Name)
	assert.Equal(t, map[string]any{"value": "hi"}, resp.ToolCalls[0].Args)
}

func TestChatWithTools_NativeDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.NotContains(t, payload, "tools")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL, NativeToolCalling: false}, "")
	resp, err := client.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, []model.LLMToolSpec{{Name: "echo"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Text)
}

func TestChatWithTools_ProfileDisablesNative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.NotContains(t, payload, "tools")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL, NativeToolCalling: true}, "")
	profile := &ModelProfile{}
	profile.ToolCalling.NativeAPI = false
	client.SetProfile(profile)
	resp, err := client.ChatWithTools(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, []model.LLMToolSpec{{Name: "echo"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Text)
	assert.False(t, client.UsesNativeToolCalling())
}

func TestBearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "ok"}}}})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "secret")
	_, err := client.Chat(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, nil)
	assert.NoError(t, err)
}

func TestBearerAuth_NoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "ok"}}}})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")
	_, err := client.Chat(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, nil)
	assert.NoError(t, err)
}

func TestChat_EstimatesUsageWhenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "hello"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")
	resp, err := client.Chat(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
	assert.True(t, resp.Usage.Estimated)
	assert.Equal(t, "char_div_4", resp.Usage.EstimationMethod)
	assert.Positive(t, resp.Usage.TotalTokens)
}

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "model-a", "object": "model", "owned_by": "org"}},
		})
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 1)
	assert.Equal(t, "model-a", models[0].Name)
}

func TestEmbedder_Single(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "model-a", payload["model"])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}}},
		})
	}))
	defer srv.Close()

	embedder := NewEmbedder(OpenAICompatConfig{Endpoint: srv.URL}, "model-a", "")
	embeddings, err := embedder.Embed(context.Background(), []string{"hello"})
	require.NoError(t, err)
	assert.Len(t, embeddings, 1)
	assert.Len(t, embeddings[0], 3)
	assert.Equal(t, 3, embedder.Dims())
}

func TestEmbedder_Batch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{0.1, 0.2}},
				{"embedding": []float32{0.3, 0.4}},
			},
		})
	}))
	defer srv.Close()

	embedder := NewEmbedder(OpenAICompatConfig{Endpoint: srv.URL}, "model-a", "")
	embeddings, err := embedder.Embed(context.Background(), []string{"a", "b"})
	require.NoError(t, err)
	assert.Len(t, embeddings, 2)
	assert.Len(t, embeddings[0], 2)
}

func TestHTTPError_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")
	_, err := client.Chat(context.Background(), []model.Message{{Role: "user", Content: "ping"}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestStreamingCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hel"}}]}`)
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	client := NewClient(OpenAICompatConfig{Endpoint: srv.URL}, "")
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := client.GenerateStream(ctx, "hello", nil)
	require.NoError(t, err)
	_, ok := <-ch
	assert.True(t, ok)
	cancel()
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not close after cancel")
	}
}

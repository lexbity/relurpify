package lmstudio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/llm/conformance"
	"codeburg.org/lexbit/relurpify/platform/llm/openaicompat"
	"github.com/stretchr/testify/require"
)

func TestBackend_ConformanceSuite(t *testing.T) {
	conformance.BackendConformanceSuite(t, conformance.BackendConformanceSpec{
		Name: "lmstudio",
		NewBackend: func(t *testing.T, endpoint string, nativeToolCalling bool) any {
			t.Helper()
			return NewBackend(Config{
				Endpoint:          endpoint,
				Model:             "test-model",
				APIKey:            "",
				NativeToolCalling: nativeToolCalling,
			})
		},
		NewServer: func(t *testing.T, scenario string, nativeToolCalling bool) *httptest.Server {
			t.Helper()
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "warm-fail" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				body, _ := io.ReadAll(r.Body)
				if scenario == "chat-tools-native" {
					require.Contains(t, string(body), `"tools"`)
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
												"arguments": `{"value":"ping"}`,
											},
										},
									},
								},
								"finish_reason": "tool_calls",
							},
						},
					})
					return
				}
				if scenario == "chat-tools-fallback" {
					require.NotContains(t, string(body), `"tools"`)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{
						{
							"message": map[string]any{
								"role":    "assistant",
								"content": "ok",
							},
							"finish_reason": "stop",
						},
					},
				})
			})
			mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "warm-fail" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]any{{"id": "model-a", "object": "model", "owned_by": "lmstudio"}},
				})
			})
			mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]any{{"embedding": []float32{1, 2, 3}}},
				})
			})
			return httptest.NewServer(mux)
		},
		Generate: func(backend any) (string, error) {
			resp, err := backend.(interface {
				Model() core.LanguageModel
			}).Model().Generate(context.Background(), "hello", nil)
			if err != nil {
				return "", err
			}
			return resp.Text, nil
		},
		Chat: func(backend any) (string, error) {
			resp, err := backend.(interface {
				Model() core.LanguageModel
			}).Model().Chat(context.Background(), []core.Message{
				{Role: "system", Content: "be concise"},
				{Role: "user", Content: "ping"},
			}, nil)
			if err != nil {
				return "", err
			}
			return resp.Text, nil
		},
		ChatWithToolsNative: func(backend any) error {
			_, err := backend.(interface {
				Model() core.LanguageModel
			}).Model().ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, []core.LLMToolSpec{{Name: "echo"}}, nil)
			return err
		},
		ChatWithToolsFallback: func(backend any) (string, error) {
			resp, err := backend.(interface {
				Model() core.LanguageModel
			}).Model().ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, []core.LLMToolSpec{{Name: "echo"}}, nil)
			if err != nil {
				return "", err
			}
			return resp.Text, nil
		},
		Warm: func(backend any) error {
			return backend.(interface{ Warm(context.Context) error }).Warm(context.Background())
		},
		WarmFail: func(backend any) error {
			return backend.(interface{ Warm(context.Context) error }).Warm(context.Background())
		},
		Health: func(backend any) any {
			report, _ := backend.(interface {
				Health(context.Context) (*HealthReport, error)
			}).Health(context.Background())
			return report
		},
		ListModels: func(backend any) any {
			models, _ := backend.(interface {
				ListModels(context.Context) ([]ModelInfo, error)
			}).ListModels(context.Background())
			return models
		},
		Embed: func(backend any) ([][]float32, error) {
			embedder := backend.(interface{ Embedder() *openaicompat.Embedder }).Embedder()
			return embedder.Embed(context.Background(), []string{"hello"})
		},
		Close: func(backend any) error {
			return backend.(interface{ Close() error }).Close()
		},
		SetDebugLogging: func(backend any, enabled bool) {
			backend.(interface{ SetDebugLogging(bool) }).SetDebugLogging(enabled)
		},
	})
}

func TestLMStudioBackend_Warm_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "test-model", "object": "model", "owned_by": "lmstudio"}},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	require.NoError(t, backend.Warm(context.Background()))
}

func TestLMStudioBackend_Warm_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	require.Error(t, backend.Warm(context.Background()))
}

func TestLMStudioBackend_Capabilities(t *testing.T) {
	backend := NewBackend(Config{NativeToolCalling: true})
	caps := backend.Capabilities()
	require.True(t, caps.NativeToolCalling)
	require.True(t, caps.UsageReporting)
	require.False(t, caps.ContextSizeDiscovery)
}

func TestLMStudioBackend_ModelContextSize_ProfileOverride(t *testing.T) {
	backend := NewBackend(Config{Endpoint: "http://localhost:1234", Model: "test-model"})
	backend.SetProfile(&openaicompat.ModelProfile{ContextSize: 4096})
	size, err := backend.ModelContextSize(context.Background())
	require.NoError(t, err)
	require.Equal(t, 4096, size)
}

func TestLMStudioBackend_Chat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"messages"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "response"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	resp, err := backend.Model().Chat(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
	require.Equal(t, "response", resp.Text)
}

func TestLMStudioBackend_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hel"}}]}`)
		flusher.Flush()
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}]}`)
		flusher.Flush()
		_, _ = fmt.Fprintln(w, `data: [DONE]`)
		flusher.Flush()
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	var got strings.Builder
	resp, err := backend.Model().ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil, &core.LLMOptions{
		StreamCallback: func(token string) { got.WriteString(token) },
	})
	require.NoError(t, err)
	require.Equal(t, "hello", got.String())
	require.Equal(t, "hello", resp.Text)
}

func TestLMStudioBackend_ChatWithTools_Native(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Contains(t, payload, "tools")
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

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model", NativeToolCalling: true})
	resp, err := backend.Model().ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, []core.LLMToolSpec{{Name: "echo"}}, nil)
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "echo", resp.ToolCalls[0].Name)
}

func TestLMStudioBackend_BearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model", APIKey: "secret"})
	_, err := backend.Model().Chat(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
}

func TestLMStudioBackend_NoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	_, err := backend.Model().Chat(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
}

func TestLMStudioBackend_Embeddings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/embeddings", r.URL.Path)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "embed-model", payload["model"])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{1, 2, 3}},
			},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "embed-model"})
	embedder := backend.Embedder()
	require.NotNil(t, embedder)
	vectors, err := embedder.Embed(context.Background(), []string{"hello"})
	require.NoError(t, err)
	require.Equal(t, [][]float32{{1, 2, 3}}, vectors)
}

func TestLMStudioBackend_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "model-a", "object": "model", "owned_by": "lmstudio"},
				{"id": "model-b", "object": "model", "owned_by": "lmstudio"},
			},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	models, err := backend.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "model-a", models[0].Name)
	require.Equal(t, "model-b", models[1].Name)
}

func TestLMStudioBackend_Health(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "model-a", "object": "model", "owned_by": "lmstudio"}},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	report, err := backend.Health(context.Background())
	require.NoError(t, err)
	require.Equal(t, BackendHealthReady, report.State)
}

func TestLMStudioBackend_CloseAndDebugLogging(t *testing.T) {
	backend := NewBackend(Config{Endpoint: "http://example.invalid", Model: "test-model"})
	require.NoError(t, backend.Close())
	require.NoError(t, backend.Close())
	backend.SetDebugLogging(true)
}

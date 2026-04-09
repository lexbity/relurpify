package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/llm/conformance"
	"github.com/stretchr/testify/require"
)

func TestBackend_ConformanceSuite(t *testing.T) {
	conformance.BackendConformanceSuite(t, conformance.BackendConformanceSpec{
		Name: "ollama",
		NewBackend: func(t *testing.T, endpoint string, nativeToolCalling bool) any {
			t.Helper()
			return NewBackend(Config{
				Endpoint:          endpoint,
				Model:             "test-model",
				EmbeddingModel:    "embed-model",
				NativeToolCalling: nativeToolCalling,
			})
		},
		NewServer: func(t *testing.T, scenario string, nativeToolCalling bool) *httptest.Server {
			t.Helper()
			mux := http.NewServeMux()
			mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "warm-fail" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"response": "generated"})
			})
			mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "warm-fail" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				body, _ := io.ReadAll(r.Body)
				if scenario == "chat-tools-native" {
					require.Contains(t, string(body), `"tools"`)
					_ = json.NewEncoder(w).Encode(map[string]any{
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
					})
					return
				}
				if scenario == "chat-tools-fallback" {
					require.NotContains(t, string(body), `"tools"`)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"message": map[string]any{
						"role":    "assistant",
						"content": "ok",
					},
				})
			})
			mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "warm-fail" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"models": []map[string]any{{"name": "test-model", "families": "llama", "quantization_level": "q4"}},
				})
			})
			mux.HandleFunc("/api/embed", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"embeddings": [][]float32{{1, 2, 3}},
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
			embedder := backend.(interface{ Embedder() *Embedder }).Embedder()
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

func TestBackend_Warm_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/tags", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{{"name": "test-model"}},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	require.NoError(t, backend.Warm(context.Background()))
}

func TestBackend_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/tags", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "test-model", "families": "llama", "quantization_level": "q4"},
			},
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	models, err := backend.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "test-model", models[0].Name)
	require.Equal(t, "llama", models[0].Family)
	require.Equal(t, "q4", models[0].Quantization)
}

func TestBackend_Capabilities(t *testing.T) {
	backend := NewBackend(Config{NativeToolCalling: true})
	caps := backend.Capabilities()
	require.True(t, caps.NativeToolCalling)
	require.True(t, caps.Streaming)
	require.True(t, caps.ModelListing)
	require.Equal(t, core.BackendClassTransport, caps.BackendClass)
}

func TestBackend_Chat_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/chat", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), `"messages"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "response",
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model"})
	resp, err := backend.Model().Chat(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)
	require.Equal(t, "response", resp.Text)
}

func TestBackend_ChatWithTools_NativeDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NotContains(t, string(body), `"tools"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "ok",
		})
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model", NativeToolCalling: false})
	resp, err := backend.Model().ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Text)
}

func TestBackend_Streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		_, _ = w.Write([]byte(`{"message":{"content":"hel"}}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"message":{"content":"lo"},"done_reason":"stop"}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	backend := NewBackend(Config{Endpoint: srv.URL, Model: "test-model", NativeToolCalling: true})
	var got strings.Builder
	resp, err := backend.Model().ChatWithTools(context.Background(), []core.Message{{Role: "user", Content: "ping"}}, nil, &core.LLMOptions{
		StreamCallback: func(token string) { got.WriteString(token) },
	})
	require.NoError(t, err)
	require.Equal(t, "hello", got.String())
	require.Equal(t, "hello", resp.Text)
}

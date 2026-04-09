package retrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/llm"
	"github.com/stretchr/testify/require"
)

type factoryStubEmbedder struct{}

func (factoryStubEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 2}}, nil
}

func (factoryStubEmbedder) ModelID() string { return "stub-model" }
func (factoryStubEmbedder) Dims() int       { return 2 }

type factoryStubBackend struct {
	embedder llm.Embedder
}

func (b factoryStubBackend) Model() core.LanguageModel { return nil }
func (b factoryStubBackend) Embedder() llm.Embedder    { return b.embedder }
func (factoryStubBackend) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (factoryStubBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{State: llm.BackendHealthReady}, nil
}
func (factoryStubBackend) ListModels(context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (factoryStubBackend) Warm(context.Context) error                          { return nil }
func (factoryStubBackend) Close() error                                        { return nil }
func (factoryStubBackend) SetDebugLogging(bool)                                {}

func TestNewEmbedder_BackendHasEmbedder(t *testing.T) {
	t.Parallel()

	stub := &factoryStubEmbedder{}
	backend := factoryStubBackend{embedder: stub}

	embedder, err := NewEmbedder(backend, EmbedderConfig{})
	require.NoError(t, err)
	require.Same(t, stub, embedder)
}

func TestNewEmbedder_BackendNilEmbedder_ConfigPresent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/embed", r.URL.Path)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "factory-embed", payload["model"])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{{9, 8, 7}},
		}))
	}))
	defer srv.Close()

	embedder, err := NewEmbedder(nil, EmbedderConfig{
		Provider: "ollama",
		Endpoint: srv.URL,
		Model:    "factory-embed",
	})
	require.NoError(t, err)
	require.NotNil(t, embedder)

	vectors, err := embedder.Embed(context.Background(), []string{"hello"})
	require.NoError(t, err)
	require.Equal(t, [][]float32{{9, 8, 7}}, vectors)
	require.Equal(t, "factory-embed", embedder.ModelID())
}

func TestNewEmbedder_BackendNilEmbedder_NoConfig(t *testing.T) {
	t.Parallel()

	embedder, err := NewEmbedder(nil, EmbedderConfig{})
	require.NoError(t, err)
	require.Nil(t, embedder)
}

func TestNewEmbedder_BackendNilEmbedder_OllamaConfig(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/embed", r.URL.Path)
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "nomic-embed-text", payload["model"])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"embedding": []float32{1, 2, 3, 4},
		}))
	}))
	defer srv.Close()

	embedder, err := NewEmbedder(nil, EmbedderConfig{
		Provider: "ollama",
		Endpoint: srv.URL,
		Model:    "nomic-embed-text",
	})
	require.NoError(t, err)
	require.NotNil(t, embedder)

	vectors, err := embedder.Embed(context.Background(), []string{"alpha"})
	require.NoError(t, err)
	require.Equal(t, [][]float32{{1, 2, 3, 4}}, vectors)
	require.Equal(t, "nomic-embed-text", embedder.ModelID())
}

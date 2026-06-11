package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbedder_Embed_Single(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/embed", r.URL.Path)

		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "nomic-embed-text", payload["model"])

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"embedding": []float32{1, 2, 3},
		}))
	}))
	defer srv.Close()

	embedder := NewEmbedder(Config{Endpoint: srv.URL, Model: "nomic-embed-text"}, "nomic-embed-text")
	vectors, err := embedder.Embed(context.Background(), []string{"alpha"})
	require.NoError(t, err)
	assert.Equal(t, [][]float32{{1, 2, 3}}, vectors)
	assert.Equal(t, 3, embedder.Dims())
}

func TestEmbedder_Embed_Batch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/embed", r.URL.Path)

		var payload map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "nomic-embed-text", payload["model"])

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{
				{1, 0, 0},
				{0, 1, 0},
			},
		}))
	}))
	defer srv.Close()

	embedder := NewEmbedder(Config{Endpoint: srv.URL, Model: "nomic-embed-text"}, "nomic-embed-text")
	vectors, err := embedder.Embed(context.Background(), []string{"alpha", "beta"})
	require.NoError(t, err)
	assert.Equal(t, [][]float32{{1, 0, 0}, {0, 1, 0}}, vectors)
	assert.Equal(t, 3, embedder.Dims())
}

func TestEmbedder_ModelID(t *testing.T) {
	t.Parallel()

	embedder := NewEmbedder(Config{Endpoint: "http://example.invalid", Model: "embed-model"}, "embed-model")
	require.Equal(t, "embed-model", embedder.ModelID())
}

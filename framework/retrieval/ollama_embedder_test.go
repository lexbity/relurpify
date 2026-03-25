package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOllamaEmbedderEmbed(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/embed", r.URL.Path)

		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "nomic-embed-text", payload["model"])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{
				{1, 2, 3},
				{4, 5, 6},
			},
		}))
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local listener unavailable in this environment: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	embedder := NewOllamaEmbedder(fmt.Sprintf("http://%s", listener.Addr().String()), "nomic-embed-text")
	vectors, err := embedder.Embed(context.Background(), []string{"alpha", "beta"})
	require.NoError(t, err)
	require.Len(t, vectors, 2)
	require.Equal(t, 3, embedder.Dims())
	require.Equal(t, "nomic-embed-text", embedder.ModelID())
	require.Equal(t, []float32{1, 2, 3}, vectors[0])
}

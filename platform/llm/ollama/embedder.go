package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Embedder implements retrieval.Embedder via the Ollama /api/embed endpoint.
type Embedder struct {
	baseURL string
	model   string
	client  *http.Client

	mu   sync.RWMutex
	dims int
}

// NewEmbedder constructs an Ollama-backed embedder.
func NewEmbedder(cfg Config, model string) *Embedder {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = strings.TrimSpace(cfg.EmbeddingModel)
	}
	if model == "" {
		model = strings.TrimSpace(cfg.Model)
	}
	return &Embedder{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e == nil {
		return nil, fmt.Errorf("ollama embedder not configured")
	}
	if strings.TrimSpace(e.model) == "" {
		return nil, fmt.Errorf("ollama embedder model required")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	reqBody, err := json.Marshal(map[string]any{
		"model": e.model,
		"input": texts,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if resp, err := e.client.Do(req); err != nil {
		return nil, err
	} else {
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("ollama embed failed: %s", resp.Status)
		}
		var payload struct {
			Embeddings [][]float32 `json:"embeddings"`
			Embedding  []float32   `json:"embedding"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, err
		}
		embeddings := payload.Embeddings
		if len(embeddings) == 0 && len(payload.Embedding) > 0 {
			embeddings = [][]float32{payload.Embedding}
		}
		if len(embeddings) != len(texts) {
			return nil, fmt.Errorf("ollama embed returned %d embeddings for %d texts", len(embeddings), len(texts))
		}
		if len(embeddings) > 0 {
			e.mu.Lock()
			e.dims = len(embeddings[0])
			e.mu.Unlock()
		}
		return embeddings, nil
	}
}

func (e *Embedder) ModelID() string {
	if e == nil {
		return ""
	}
	return e.model
}

func (e *Embedder) Dims() int {
	if e == nil {
		return 0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dims
}

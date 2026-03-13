package retrieval

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

// OllamaEmbedder implements Embedder via the Ollama /api/embed endpoint.
type OllamaEmbedder struct {
	baseURL string
	model   string

	client *http.Client

	mu   sync.RWMutex
	dims int
}

// NewOllamaEmbedder constructs an Ollama-backed embedder.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   strings.TrimSpace(model),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
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

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
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

func (e *OllamaEmbedder) ModelID() string {
	if e == nil {
		return ""
	}
	return e.model
}

func (e *OllamaEmbedder) Dims() int {
	if e == nil {
		return 0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dims
}

package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Embedder implements retrieval.Embedder using /v1/embeddings.
type Embedder struct {
	client *Client
	model  string
	dims   int
}

// NewEmbedder constructs a new OpenAI-compatible embedder.
func NewEmbedder(cfg OpenAICompatConfig, model string) *Embedder {
	return &Embedder{
		client: NewClient(cfg),
		model:  strings.TrimSpace(model),
	}
}

func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e == nil {
		return nil, fmt.Errorf("embedder not configured")
	}
	if strings.TrimSpace(e.model) == "" {
		return nil, fmt.Errorf("embedder model required")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	reqBody := map[string]any{
		"model": e.model,
		"input": texts,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.client.cfg.normalizedEndpoint()+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(e.client.cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(e.client.cfg.APIKey))
	}
	resp, err := e.client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readHTTPError(resp)
	}
	var payload struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	embeddings := make([][]float32, 0, len(payload.Data))
	for _, item := range payload.Data {
		embeddings = append(embeddings, item.Embedding)
	}
	if len(embeddings) == 0 && len(payload.Embedding) > 0 {
		embeddings = append(embeddings, payload.Embedding)
	}
	if len(embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding returned %d vectors for %d texts", len(embeddings), len(texts))
	}
	if len(embeddings) > 0 {
		e.dims = len(embeddings[0])
	}
	return embeddings, nil
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
	return e.dims
}

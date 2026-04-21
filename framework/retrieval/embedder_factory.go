package retrieval

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/llm"
)

// EmbedderConfig captures fallback embedder construction settings.
type EmbedderConfig struct {
	Provider string
	Endpoint string
	Model    string
	APIKey   string
}

// NewEmbedder selects the best available embedder for retrieval.
func NewEmbedder(backend llm.ManagedBackend, cfg EmbedderConfig) (Embedder, error) {
	if backend != nil {
		if embedder := backend.Embedder(); embedder != nil {
			return embedder, nil
		}
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		return nil, nil
	}
	fallback, err := llm.New(llm.ProviderConfig{
		Provider: cfg.Provider,
		Endpoint: cfg.Endpoint,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("build embedder backend: %w", err)
	}
	defer fallback.Close()
	if embedder := fallback.Embedder(); embedder != nil {
		return embedder, nil
	}
	return nil, nil
}

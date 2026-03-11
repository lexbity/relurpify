package retrieval

import "context"

// Embedder produces dense vector representations of text.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ModelID() string
	Dims() int
}

package ollama

import "time"

// Config captures construction settings for the Ollama transport backend.
type Config struct {
	Endpoint          string
	Model             string
	EmbeddingModel    string
	ModelPath         string
	APIKey            string
	Timeout           time.Duration
	NativeToolCalling bool
	Debug             bool
	Config            map[string]any
}

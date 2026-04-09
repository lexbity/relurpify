package openaicompat

import "time"

// ModelInfo summarizes a model exposed by an OpenAI-compatible backend.
type ModelInfo struct {
	Name          string    `json:"name"`
	Family        string    `json:"family,omitempty"`
	ParameterSize string    `json:"parameter_size,omitempty"`
	ContextSize   int       `json:"context_size,omitempty"`
	Quantization  string    `json:"quantization,omitempty"`
	HasGPU        bool      `json:"has_gpu,omitempty"`
	OwnedBy       string    `json:"owned_by,omitempty"`
	Object        string    `json:"object,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

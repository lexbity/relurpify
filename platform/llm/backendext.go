package llm

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// SessionAwareBackend is implemented by backends that can bind and evict
// backend-local sessions.
type SessionAwareBackend interface {
	WithSession(sessionID string) core.LanguageModel
	EvictSession(ctx context.Context, sessionID string) error
	ActiveSessions(ctx context.Context) ([]string, error)
}

// NativeTokenStream provides token-level streaming with backend metadata.
type NativeTokenStream interface {
	ChatTokenStream(ctx context.Context, messages []core.Message, opts *core.LLMOptions) (<-chan Token, error)
}

// Token is a backend-native token stream item.
type Token struct {
	Text    string
	ID      int32
	Logprob float32
	Final   bool
}

// BatchInferenceBackend provides coalesced batch inference.
type BatchInferenceBackend interface {
	ChatBatch(ctx context.Context, requests []BatchRequest, opts *core.LLMOptions) ([]*core.LLMResponse, error)
}

// BatchRequest bundles a single batch inference request.
type BatchRequest struct {
	Messages  []core.Message
	Tools     []core.LLMToolSpec
	SessionID string
}

// BackendResourceReporter exposes backend resource metrics.
type BackendResourceReporter interface {
	ResourceSnapshot(ctx context.Context) (*ResourceSnapshot, error)
}

// ModelController exposes explicit model load and unload controls.
type ModelController interface {
	LoadModel(ctx context.Context, path string, opts ModelLoadOptions) error
	UnloadModel(ctx context.Context) error
	ModelInfo(ctx context.Context) (*LoadedModelInfo, error)
}

// ModelLoadOptions controls native model loading behavior.
type ModelLoadOptions struct {
	ContextSize int
	Threads     int
	GPULayers   int
	BatchSize   int
	FlashAttn   bool
	Config      map[string]any
}

// LoadedModelInfo summarizes a loaded model.
type LoadedModelInfo struct {
	Path           string
	Architecture   string
	ParameterCount int64
	ContextLength  int
	Quantization   string
	VRAMEstimateMB int64
}

// BackendRecovery exposes restart and error history hooks.
type BackendRecovery interface {
	Restart(ctx context.Context) error
	ErrorHistory() []BackendError
}

// BackendError records a backend failure event.
type BackendError struct {
	Err        error
	OccurredAt time.Time
	Stack      string
}

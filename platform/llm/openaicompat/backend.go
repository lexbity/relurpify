package openaicompat

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// BackendConfig configures an OpenAI-compatible managed backend.
type BackendConfig struct {
	Endpoint          string
	Model             string
	Timeout           time.Duration
	NativeToolCalling bool
	Debug             bool
}

// Backend implements the managed backend facade for an OpenAI-compatible provider.
type Backend struct {
	client *Client
	cfg    BackendConfig
}

// NewBackend constructs a managed backend for an OpenAI-compatible provider.
func NewBackend(cfg BackendConfig, apiKey string) *Backend {
	clientCfg := OpenAICompatConfig{
		Endpoint:          cfg.Endpoint,
		Timeout:           cfg.Timeout,
		NativeToolCalling: cfg.NativeToolCalling,
	}
	return &Backend{
		client: NewClient(clientCfg, apiKey),
		cfg:    cfg,
	}
}

// Model returns the underlying language model client.
func (b *Backend) Model() llm.LanguageModel {
	if b == nil {
		return nil
	}
	return b.client
}

// Embedder returns an OpenAI-compatible embedder bound to the endpoint.
func (b *Backend) Embedder() llm.Embedder {
	if b == nil || b.client == nil {
		return nil
	}
	model := strings.TrimSpace(b.cfg.Model)
	if model == "" {
		return nil
	}
	return NewEmbedder(OpenAICompatConfig{
		Endpoint:          b.cfg.Endpoint,
		Timeout:           b.cfg.Timeout,
		NativeToolCalling: b.cfg.NativeToolCalling,
	}, model, b.client.apiKey)
}

// Capabilities reports the transport-backed feature set.
func (b *Backend) Capabilities() model.BackendCapabilities {
	if b == nil {
		return model.BackendCapabilities{}
	}
	return model.BackendCapabilities{
		NativeToolCalling:    b.cfg.NativeToolCalling,
		Streaming:            true,
		Embeddings:           true,
		ModelListing:         true,
		BackendClass:         model.BackendClassTransport,
		UsageReporting:       true,
		ContextSizeDiscovery: false,
	}
}

// ModelContextSize reports the profile override when present.
func (b *Backend) ModelContextSize(ctx context.Context) (int, error) {
	if b != nil && b.client != nil {
		if size := b.client.ContextSize(); size > 0 {
			return size, nil
		}
	}
	return 0, nil
}

// Health checks backend reachability via /v1/models.
func (b *Backend) Health(ctx context.Context) (*llm.HealthReport, error) {
	models, err := b.client.ListModels(ctx)
	if err != nil {
		return &llm.HealthReport{
			State:      llm.BackendHealthUnhealthy,
			Message:    err.Error(),
			LastError:  err.Error(),
			ErrorCount: 1,
		}, err
	}
	_ = models
	return &llm.HealthReport{
		State:       llm.BackendHealthReady,
		Message:     "backend reachable",
		UptimeSince: time.Now().UTC(),
	}, nil
}

// ListModels fetches /v1/models and converts it into model summaries.
func (b *Backend) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	if b == nil || b.client == nil {
		return nil, nil
	}
	models, err := b.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]llm.ModelInfo, 0, len(models))
	contextSize := b.client.ContextSize()
	for _, m := range models {
		size := m.ContextSize
		if size == 0 && contextSize > 0 {
			size = contextSize
		}
		out = append(out, llm.ModelInfo{
			Name:          m.Name,
			Family:        m.Family,
			ParameterSize: m.ParameterSize,
			ContextSize:   size,
			Quantization:  m.Quantization,
			HasGPU:        m.HasGPU,
		})
	}
	return out, nil
}

// Warm performs a reachability check.
func (b *Backend) Warm(ctx context.Context) error {
	_, err := b.ListModels(ctx)
	return err
}

// Close is a no-op for the HTTP-based backend.
func (b *Backend) Close() error {
	return nil
}

// SetDebugLogging toggles verbose request logging.
func (b *Backend) SetDebugLogging(enabled bool) {
	if b == nil || b.client == nil {
		return
	}
	b.client.SetDebugLogging(enabled)
}

// SetProfile attaches a resolved model profile to the underlying client.
func (b *Backend) SetProfile(p *model.ModelProfile) {
	if b == nil || b.client == nil {
		return
	}
	b.client.SetProfile(p)
}

// Reset is a no-op for HTTP-based backends.
func (b *Backend) Reset(ctx context.Context, strategy string) error {
	return nil
}

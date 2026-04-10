package lmstudio

import (
	"context"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/llm/openaicompat"
)

// Backend implements the managed backend facade for LM Studio.
type Backend struct {
	client *openaicompat.Client
	cfg    Config
}

// NewBackend constructs a managed LM Studio backend.
func NewBackend(cfg Config) *Backend {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "http://localhost:1234"
	}
	cfg.Endpoint = endpoint
	clientCfg := openaicompat.OpenAICompatConfig{
		Endpoint:          endpoint,
		APIKey:            cfg.APIKey,
		Timeout:           cfg.Timeout,
		NativeToolCalling: cfg.NativeToolCalling,
	}
	return &Backend{
		client: openaicompat.NewClient(clientCfg),
		cfg:    cfg,
	}
}

// Model returns the underlying language model client.
func (b *Backend) Model() core.LanguageModel {
	if b == nil {
		return nil
	}
	return b.client
}

// Embedder returns an OpenAI-compatible embedder bound to the LM Studio endpoint.
func (b *Backend) Embedder() *openaicompat.Embedder {
	if b == nil || b.client == nil {
		return nil
	}
	model := strings.TrimSpace(b.cfg.Model)
	if model == "" {
		return nil
	}
	return openaicompat.NewEmbedder(openaicompat.OpenAICompatConfig{
		Endpoint:          b.cfg.Endpoint,
		APIKey:            b.cfg.APIKey,
		Timeout:           b.cfg.Timeout,
		NativeToolCalling: b.cfg.NativeToolCalling,
	}, model)
}

// Capabilities reports the transport-backed feature set.
func (b *Backend) Capabilities() core.BackendCapabilities {
	if b == nil {
		return core.BackendCapabilities{}
	}
	return core.BackendCapabilities{
		NativeToolCalling: b.cfg.NativeToolCalling,
		Streaming:         true,
		Embeddings:        true,
		ModelListing:      true,
		BackendClass:      core.BackendClassTransport,
	}
}

// Health checks backend reachability via /v1/models.
func (b *Backend) Health(ctx context.Context) (*HealthReport, error) {
	models, err := b.ListModels(ctx)
	if err != nil {
		return &HealthReport{
			State:      BackendHealthUnhealthy,
			Message:    err.Error(),
			LastError:  err.Error(),
			ErrorCount: 1,
		}, err
	}
	_ = models
	return &HealthReport{
		State:       BackendHealthReady,
		Message:     "backend reachable",
		UptimeSince: nowUTC(),
	}, nil
}

// ListModels fetches /v1/models and converts it into model summaries.
func (b *Backend) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if b == nil || b.client == nil {
		return nil, nil
	}
	models, err := b.client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		out = append(out, ModelInfo{
			Name:          m.Name,
			Family:        m.Family,
			ParameterSize: m.ParameterSize,
			ContextSize:   m.ContextSize,
			Quantization:  m.Quantization,
			HasGPU:        m.HasGPU,
		})
	}
	return out, nil
}

// Warm performs a reachability check against the LM Studio backend.
func (b *Backend) Warm(ctx context.Context) error {
	_, err := b.ListModels(ctx)
	return err
}

// Close is idempotent for the LM Studio backend wrapper.
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
func (b *Backend) SetProfile(p *openaicompat.ModelProfile) {
	if b == nil || b.client == nil {
		return
	}
	b.client.SetProfile(p)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

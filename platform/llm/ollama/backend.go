package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// Backend implements the managed backend facade for Ollama transports.
type Backend struct {
	client *Client
	cfg    Config
}

type managedModel struct {
	client            *Client
	nativeToolCalling bool
}

// NewBackend constructs a managed Ollama backend.
func NewBackend(cfg Config) *Backend {
	return &Backend{client: NewClient(cfg.Endpoint, cfg.Model), cfg: cfg}
}

// Model returns the underlying language model client.
func (b *Backend) Model() core.LanguageModel {
	return managedModel{client: b.client, nativeToolCalling: b.cfg.NativeToolCalling}
}

// Embedder returns a transport-backed embedder when embedding is enabled.
func (b *Backend) Embedder() *Embedder {
	if b == nil {
		return nil
	}
	model := strings.TrimSpace(b.cfg.EmbeddingModel)
	if model == "" {
		model = strings.TrimSpace(b.cfg.Model)
	}
	if model == "" {
		return nil
	}
	return NewEmbedder(Config{
		Endpoint: b.cfg.Endpoint,
		Model:    model,
		APIKey:   b.cfg.APIKey,
		Timeout:  b.cfg.Timeout,
		Debug:    b.cfg.Debug,
	}, model)
}

// Capabilities reports the transport-backed feature set.
func (b *Backend) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{
		NativeToolCalling: b.cfg.NativeToolCalling,
		Streaming:         true,
		Embeddings:        true,
		ModelListing:      true,
		BackendClass:      core.BackendClassTransport,
	}
}

// Health checks backend reachability and model listing availability.
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
		UptimeSince: time.Now().UTC(),
	}, nil
}

// ListModels fetches /api/tags and converts it into model summaries.
func (b *Backend) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.client.ollamaAPIEndpoint()+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama tags error: %s", resp.Status)
	}
	var payload struct {
		Models []struct {
			Name         string `json:"name"`
			Size         int64  `json:"size"`
			Digest       string `json:"digest"`
			ModifiedAt   string `json:"modified_at"`
			Families     string `json:"families"`
			Parameters   string `json:"parameters"`
			Quantization string `json:"quantization_level"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(payload.Models))
	for _, m := range payload.Models {
		out = append(out, ModelInfo{
			Name:         m.Name,
			Family:       strings.TrimSpace(m.Families),
			Quantization: strings.TrimSpace(m.Quantization),
		})
	}
	return out, nil
}

// Warm performs a reachability check against the Ollama backend.
func (b *Backend) Warm(ctx context.Context) error {
	_, err := b.ListModels(ctx)
	return err
}

// Close drains idle connections when the underlying transport supports it.
func (b *Backend) Close() error {
	if transport, ok := b.client.getHTTPClient().Transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
	return nil
}

// SetDebugLogging toggles verbose request logging.
func (b *Backend) SetDebugLogging(enabled bool) {
	b.client.SetDebugLogging(enabled)
}

func (m managedModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	return m.client.Generate(ctx, prompt, options)
}

func (m managedModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return m.client.GenerateStream(ctx, prompt, options)
}

func (m managedModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return m.client.Chat(ctx, messages, options)
}

func (m managedModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	if !m.nativeToolCalling {
		return m.client.Chat(ctx, messages, options)
	}
	return m.client.ChatWithTools(ctx, messages, tools, options)
}

func (m managedModel) ToolRepairStrategy() string {
	return "heuristic-only"
}

func (m managedModel) MaxToolsPerCall() int {
	return 0
}

func (m managedModel) UsesNativeToolCalling() bool {
	return m.nativeToolCalling
}

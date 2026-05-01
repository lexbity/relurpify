package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Re-export contract types for local usage
type (
	LanguageModel       = contracts.LanguageModel
	LLMOptions          = contracts.LLMOptions
	LLMResponse         = contracts.LLMResponse
	Message             = contracts.Message
	LLMToolSpec         = contracts.LLMToolSpec
	Schema              = contracts.Schema
	BackendClass        = contracts.BackendClass
	BackendCapabilities = contracts.BackendCapabilities
)

const (
	BackendClassTransport = contracts.BackendClassTransport
)

// Backend implements the managed backend facade for Ollama transports.
type Backend struct {
	client *Client
	cfg    Config

	mu                sync.Mutex
	cachedContextSize int
	contextSizeLoaded bool
}

type managedModel struct {
	client *Client
}

// NewBackend constructs a managed Ollama backend.
func NewBackend(cfg Config) *Backend {
	client := NewClient(cfg.Endpoint, cfg.Model)
	client.SetNativeToolCalling(cfg.NativeToolCalling)
	return &Backend{client: client, cfg: cfg}
}

// Model returns the underlying language model client.
func (b *Backend) Model() LanguageModel {
	return managedModel{client: b.client}
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
func (b *Backend) Capabilities() BackendCapabilities {
	return BackendCapabilities{
		NativeToolCalling:    b.cfg.NativeToolCalling,
		Streaming:            true,
		Embeddings:           true,
		ModelListing:         true,
		BackendClass:         BackendClassTransport,
		UsageReporting:       true,
		ContextSizeDiscovery: true,
	}
}

// ModelContextSize discovers and caches the active model context size.
func (b *Backend) ModelContextSize(ctx context.Context) (int, error) {
	if b == nil || b.client == nil {
		return 0, nil
	}
	if override := b.client.ContextSize(); override > 0 {
		b.storeModelContextSize(override)
		return override, nil
	}
	if size, ok := b.cachedModelContextSize(); ok {
		return size, nil
	}
	size, err := b.discoverModelContextSize(ctx)
	if err != nil {
		return 0, err
	}
	b.storeModelContextSize(size)
	return size, nil
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
	contextSize, _ := b.cachedModelContextSize()
	for _, m := range payload.Models {
		size := 0
		if strings.TrimSpace(m.Name) == strings.TrimSpace(b.cfg.Model) {
			size = contextSize
		}
		out = append(out, ModelInfo{
			Name:         m.Name,
			Family:       strings.TrimSpace(m.Families),
			ContextSize:  size,
			Quantization: strings.TrimSpace(m.Quantization),
		})
	}
	return out, nil
}

// Warm performs a reachability check against the Ollama backend.
func (b *Backend) Warm(ctx context.Context) error {
	_, err := b.ListModels(ctx)
	_, _ = b.ModelContextSize(ctx)
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

// SetProfile attaches a resolved model profile to the underlying client.
func (b *Backend) SetProfile(p *ModelProfile) {
	if b == nil || b.client == nil {
		return
	}
	b.client.SetProfile(p)
}

func (b *Backend) cachedModelContextSize() (int, bool) {
	if b != nil && b.client != nil {
		if override := b.client.ContextSize(); override > 0 {
			return override, true
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.contextSizeLoaded {
		return 0, false
	}
	return b.cachedContextSize, true
}

func (b *Backend) storeModelContextSize(size int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cachedContextSize = size
	b.contextSizeLoaded = true
}

func (b *Backend) discoverModelContextSize(ctx context.Context) (int, error) {
	model := strings.TrimSpace(b.cfg.Model)
	if model == "" {
		return 0, nil
	}
	body, err := json.Marshal(map[string]any{"name": model})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.client.ollamaAPIEndpoint()+"/api/show", strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.getHTTPClient().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("ollama show error: %s", resp.Status)
	}
	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return 0, err
	}
	return extractOllamaContextSize(decoded), nil
}

func extractOllamaContextSize(payload map[string]any) int {
	if size := findContextSize(payload); size > 0 {
		return size
	}
	return 0
}

func findContextSize(value any) int {
	switch v := value.(type) {
	case map[string]any:
		for k, item := range v {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "context_length") || strings.Contains(lk, "num_ctx") || lk == "contextsize" {
				if size := asInt(item); size > 0 {
					return size
				}
			}
			if size := findContextSize(item); size > 0 {
				return size
			}
		}
	case []any:
		for _, item := range v {
			if size := findContextSize(item); size > 0 {
				return size
			}
		}
	case string:
		if size := extractContextSizeFromText(v); size > 0 {
			return size
		}
	}
	return 0
}

func extractContextSizeFromText(text string) int {
	if text == "" {
		return 0
	}
	re := regexp.MustCompile(`(?i)num_ctx\s+(\d+)`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	return asIntString(match[1])
}

func asInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case json.Number:
		n, _ := val.Int64()
		return int(n)
	case string:
		return asIntString(val)
	default:
		return 0
	}
}

func asIntString(text string) int {
	n := 0
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func (m managedModel) Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error) {
	return m.client.Generate(ctx, prompt, options)
}

func (m managedModel) GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error) {
	return m.client.GenerateStream(ctx, prompt, options)
}

func (m managedModel) Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error) {
	return m.client.Chat(ctx, messages, options)
}

func (m managedModel) ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error) {
	return m.client.ChatWithTools(ctx, messages, tools, options)
}

func (m managedModel) ToolRepairStrategy() string {
	return m.client.ToolRepairStrategy()
}

func (m managedModel) MaxToolsPerCall() int {
	return m.client.MaxToolsPerCall()
}

func (m managedModel) UsesNativeToolCalling() bool {
	return m.client.UsesNativeToolCalling()
}

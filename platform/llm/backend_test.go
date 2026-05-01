package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type stubManagedBackend struct {
	model core.LanguageModel
}

func (s *stubManagedBackend) Model() core.LanguageModel {
	return s.model
}

func (s *stubManagedBackend) Embedder() Embedder {
	return nil
}

func (s *stubManagedBackend) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}

func (s *stubManagedBackend) ModelContextSize(context.Context) (int, error) {
	return 0, nil
}

func (s *stubManagedBackend) Health(context.Context) (*HealthReport, error) {
	return &HealthReport{State: BackendHealthReady}, nil
}

func (s *stubManagedBackend) ListModels(context.Context) ([]ModelInfo, error) {
	return nil, nil
}

func (s *stubManagedBackend) Warm(context.Context) error {
	return nil
}

func (s *stubManagedBackend) Close() error {
	return nil
}

func (s *stubManagedBackend) SetDebugLogging(bool) {}

func (s *stubManagedBackend) WithSession(string) core.LanguageModel {
	return s.model
}

func (s *stubManagedBackend) EvictSession(context.Context, string) error {
	return nil
}

func (s *stubManagedBackend) ActiveSessions(context.Context) ([]string, error) {
	return nil, nil
}

func (s *stubManagedBackend) ChatTokenStream(context.Context, []core.Message, *core.LLMOptions) (<-chan Token, error) {
	ch := make(chan Token)
	close(ch)
	return ch, nil
}

func (s *stubManagedBackend) ChatBatch(context.Context, []BatchRequest, *core.LLMOptions) ([]*core.LLMResponse, error) {
	return nil, nil
}

func (s *stubManagedBackend) ResourceSnapshot(context.Context) (*ResourceSnapshot, error) {
	return &ResourceSnapshot{}, nil
}

func (s *stubManagedBackend) LoadModel(context.Context, string, ModelLoadOptions) error {
	return nil
}

func (s *stubManagedBackend) UnloadModel(context.Context) error {
	return nil
}

func (s *stubManagedBackend) ModelInfo(context.Context) (*LoadedModelInfo, error) {
	return &LoadedModelInfo{}, nil
}

func (s *stubManagedBackend) Restart(context.Context) error {
	return nil
}

func (s *stubManagedBackend) ErrorHistory() []BackendError {
	return nil
}

type stubLanguageModel struct{}

func (stubLanguageModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "ok"}, nil
}

func (stubLanguageModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubLanguageModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "ok"}, nil
}

func (stubLanguageModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "ok"}, nil
}

func TestManagedBackendInterfaceCompleteness(t *testing.T) {
	var _ ManagedBackend = (*stubManagedBackend)(nil)
}

func TestOptionalInterfaceTypeAssertions(t *testing.T) {
	var backend ManagedBackend = &stubManagedBackend{model: stubLanguageModel{}}

	_, ok := backend.(SessionAwareBackend)
	require.True(t, ok)

	_, ok = backend.(NativeTokenStream)
	require.True(t, ok)

	_, ok = backend.(BatchInferenceBackend)
	require.True(t, ok)

	_, ok = backend.(BackendResourceReporter)
	require.True(t, ok)

	_, ok = backend.(ModelController)
	require.True(t, ok)

	_, ok = backend.(BackendRecovery)
	require.True(t, ok)
}

func TestBackendCapabilities_Defaults(t *testing.T) {
	var caps core.BackendCapabilities

	require.False(t, caps.NativeToolCalling)
	require.False(t, caps.Streaming)
	require.False(t, caps.Embeddings)
	require.False(t, caps.ModelListing)
	require.Equal(t, core.BackendClass(""), caps.BackendClass)
}

func TestProviderConfig_Validate(t *testing.T) {
	t.Run("missing provider", func(t *testing.T) {
		err := (ProviderConfig{}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "provider required")
	})

	t.Run("transport endpoint required", func(t *testing.T) {
		err := (ProviderConfig{Provider: "ollama"}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "endpoint required")
	})

	t.Run("negative timeout rejected", func(t *testing.T) {
		err := (ProviderConfig{Provider: "native", Timeout: -time.Second}).Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "timeout must be >= 0")
	})

	t.Run("valid transport config", func(t *testing.T) {
		err := (ProviderConfig{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
			Timeout:  30 * time.Second,
		}).Validate()
		require.NoError(t, err)
	})
}

func TestHealthReport_Serialisation(t *testing.T) {
	report := HealthReport{
		State:       BackendHealthDegraded,
		Message:     "slow response",
		LastError:   "timeout",
		LastErrorAt: time.Unix(123, 0).UTC(),
		ErrorCount:  7,
		UptimeSince: time.Unix(100, 0).UTC(),
		Resources: &ResourceSnapshot{
			VRAMUsedMB:      512,
			VRAMTotalMB:     1024,
			SystemRAMUsedMB: 2048,
			ThreadsActive:   4,
			KVCacheSlots:    16,
			KVCacheUsed:     12,
			ModelLoaded:     true,
		},
	}

	data, err := json.Marshal(report)
	require.NoError(t, err)

	var decoded HealthReport
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, report, decoded)
}

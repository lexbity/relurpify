package agenttest

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/llm"

	"github.com/stretchr/testify/require"
)

type mockBackend struct {
	models      []llm.ModelInfo
	health      *llm.HealthReport
	healthErr   error
	listErr     error
	profile     *llm.ModelProfile
	debug       bool
	resetCalled bool
	resetStrategy string
}

func (m *mockBackend) Model() llm.LanguageModel { return nil }
func (m *mockBackend) Embedder() llm.Embedder { return nil }
func (m *mockBackend) Capabilities() llm.BackendCapabilities { return llm.BackendCapabilities{} }
func (m *mockBackend) ModelContextSize(context.Context) (int, error) { return 0, nil }
func (m *mockBackend) Health(ctx context.Context) (*llm.HealthReport, error) { return m.health, m.healthErr }
func (m *mockBackend) ListModels(ctx context.Context) ([]llm.ModelInfo, error) { return m.models, m.listErr }
func (m *mockBackend) Warm(context.Context) error { return nil }
func (m *mockBackend) Close() error { return nil }
func (m *mockBackend) SetDebugLogging(enabled bool) { m.debug = enabled }
func (m *mockBackend) SetProfile(profile *llm.ModelProfile) { m.profile = profile }
func (m *mockBackend) Reset(ctx context.Context, strategy string) error { 
	m.resetCalled = true
	m.resetStrategy = strategy
	return nil
}

func TestPreflightCaseBackend(t *testing.T) {
	t.Run("model found in list", func(t *testing.T) {
		backend := &mockBackend{
			models: []llm.ModelInfo{
				{Name: "qwen2.5-coder:14b", Family: "qwen2", ParameterSize: "14B"},
			},
			health: &llm.HealthReport{State: llm.BackendHealthReady},
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "qwen2.5-coder:14b")
		require.NoError(t, err)
		require.NotNil(t, provenance)
		require.Equal(t, "qwen2.5-coder:14b", provenance.RequestedModel)
		require.Equal(t, "qwen2.5-coder:14b", provenance.LoadedName)
	})

	t.Run("model not found in list", func(t *testing.T) {
		backend := &mockBackend{
			models: []llm.ModelInfo{
				{Name: "other-model:7b", Family: "other", ParameterSize: "7B"},
			},
			health: &llm.HealthReport{State: llm.BackendHealthReady},
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "qwen2.5-coder:14b")
		require.Error(t, err)
		require.Nil(t, provenance)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("healthy backend with empty list - soft-allow", func(t *testing.T) {
		backend := &mockBackend{
			models: []llm.ModelInfo{},
			health: &llm.HealthReport{State: llm.BackendHealthReady},
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "qwen2.5-coder:14b")
		require.NoError(t, err)
		require.NotNil(t, provenance)
		require.Equal(t, "qwen2.5-coder:14b", provenance.RequestedModel)
		require.Contains(t, provenance.Details["note"], "model list was empty but backend was healthy")
	})

	t.Run("health check error", func(t *testing.T) {
		backend := &mockBackend{
			healthErr: errors.New("health check failed"),
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "qwen2.5-coder:14b")
		require.Error(t, err)
		require.Nil(t, provenance)
		require.Contains(t, err.Error(), "health check failed")
	})

	t.Run("list models error", func(t *testing.T) {
		backend := &mockBackend{
			health: &llm.HealthReport{State: llm.BackendHealthReady},
			listErr: errors.New("list failed"),
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "qwen2.5-coder:14b")
		require.Error(t, err)
		require.Nil(t, provenance)
		require.Contains(t, err.Error(), "model list failed")
	})

	t.Run("health returns nil report", func(t *testing.T) {
		backend := &mockBackend{
			health: nil,
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "qwen2.5-coder:14b")
		require.Error(t, err)
		require.Nil(t, provenance)
		require.Contains(t, err.Error(), "nil report")
	})

	t.Run("empty model name", func(t *testing.T) {
		backend := &mockBackend{
			models: []llm.ModelInfo{},
			health: &llm.HealthReport{State: llm.BackendHealthReady},
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "")
		require.Error(t, err)
		require.Nil(t, provenance)
		require.Contains(t, err.Error(), "model name is empty")
	})

	t.Run("case-insensitive model match", func(t *testing.T) {
		backend := &mockBackend{
			models: []llm.ModelInfo{
				{Name: "QWEN2.5-CODER:14B", Family: "qwen2", ParameterSize: "14B"},
			},
			health: &llm.HealthReport{State: llm.BackendHealthReady},
		}

		provenance, err := preflightCaseBackend(context.Background(), backend, "qwen2.5-coder:14b")
		require.NoError(t, err)
		require.NotNil(t, provenance)
		require.Equal(t, "QWEN2.5-CODER:14B", provenance.LoadedName)
	})
}

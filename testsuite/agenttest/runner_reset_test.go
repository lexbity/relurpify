//go:build live
// +build live

package agenttest

import (
	"context"
	"log"
	"os"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/llm"

	"github.com/stretchr/testify/require"
)

type resetMockBackend struct {
	resetCalled   bool
	resetStrategy string
}

func (m *resetMockBackend) Model() llm.LanguageModel                      { return nil }
func (m *resetMockBackend) Embedder() llm.Embedder                        { return nil }
func (m *resetMockBackend) Capabilities() llm.BackendCapabilities         { return llm.BackendCapabilities{} }
func (m *resetMockBackend) ModelContextSize(context.Context) (int, error) { return 0, nil }
func (m *resetMockBackend) Health(ctx context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{State: llm.BackendHealthReady}, nil
}
func (m *resetMockBackend) ListModels(ctx context.Context) ([]llm.ModelInfo, error) { return nil, nil }
func (m *resetMockBackend) Warm(context.Context) error                              { return nil }
func (m *resetMockBackend) Close() error                                            { return nil }
func (m *resetMockBackend) SetDebugLogging(enabled bool)                            {}
func (m *resetMockBackend) SetProfile(profile *llm.ModelProfile)                    {}
func (m *resetMockBackend) Reset(ctx context.Context, strategy string) error {
	m.resetCalled = true
	m.resetStrategy = strategy
	return nil
}

func TestResetBackendIfNeeded(t *testing.T) {
	t.Run("none strategy - no reset", func(t *testing.T) {
		backend := &resetMockBackend{}
		opts := RunOptions{BackendReset: "none"}
		logger := log.New(os.Stderr, "", 0)

		err := resetBackendIfNeeded(context.Background(), logger, backend, opts, "test-model")
		require.NoError(t, err)
		require.False(t, backend.resetCalled)
	})

	t.Run("empty strategy - no reset", func(t *testing.T) {
		backend := &resetMockBackend{}
		opts := RunOptions{BackendReset: ""}
		logger := log.New(os.Stderr, "", 0)

		err := resetBackendIfNeeded(context.Background(), logger, backend, opts, "test-model")
		require.NoError(t, err)
		require.False(t, backend.resetCalled)
	})

	t.Run("model strategy - calls reset", func(t *testing.T) {
		backend := &resetMockBackend{}
		opts := RunOptions{BackendReset: "model"}
		logger := log.New(os.Stderr, "", 0)

		err := resetBackendIfNeeded(context.Background(), logger, backend, opts, "test-model")
		require.NoError(t, err)
		require.True(t, backend.resetCalled)
		require.Equal(t, "model", backend.resetStrategy)
	})

	t.Run("server strategy - calls reset", func(t *testing.T) {
		backend := &resetMockBackend{}
		opts := RunOptions{BackendReset: "server"}
		logger := log.New(os.Stderr, "", 0)

		err := resetBackendIfNeeded(context.Background(), logger, backend, opts, "test-model")
		require.NoError(t, err)
		require.True(t, backend.resetCalled)
		require.Equal(t, "server", backend.resetStrategy)
	})

	t.Run("unknown strategy - calls reset", func(t *testing.T) {
		backend := &resetMockBackend{}
		opts := RunOptions{BackendReset: "unknown"}
		logger := log.New(os.Stderr, "", 0)

		err := resetBackendIfNeeded(context.Background(), logger, backend, opts, "test-model")
		require.NoError(t, err)
		require.True(t, backend.resetCalled)
		require.Equal(t, "unknown", backend.resetStrategy)
	})

	t.Run("case insensitive strategy", func(t *testing.T) {
		backend := &resetMockBackend{}
		opts := RunOptions{BackendReset: "MODEL"}
		logger := log.New(os.Stderr, "", 0)

		err := resetBackendIfNeeded(context.Background(), logger, backend, opts, "test-model")
		require.NoError(t, err)
		require.True(t, backend.resetCalled)
		require.Equal(t, "model", backend.resetStrategy)
	})

	t.Run("strategy with whitespace", func(t *testing.T) {
		backend := &resetMockBackend{}
		opts := RunOptions{BackendReset: "  model  "}
		logger := log.New(os.Stderr, "", 0)

		err := resetBackendIfNeeded(context.Background(), logger, backend, opts, "test-model")
		require.NoError(t, err)
		require.True(t, backend.resetCalled)
		require.Equal(t, "model", backend.resetStrategy)
	})
}

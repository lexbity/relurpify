package ollama

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackendReset(t *testing.T) {
	originalExec := execCommandContext
	originalSleep := sleepFn
	t.Cleanup(func() {
		execCommandContext = originalExec
		sleepFn = originalSleep
	})
	execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	}
	sleepFn = func(time.Duration) {}

	backend := &Backend{}

	t.Run("none strategy returns nil", func(t *testing.T) {
		require.NoError(t, backend.Reset(context.Background(), "none"))
	})

	t.Run("empty strategy returns nil", func(t *testing.T) {
		require.NoError(t, backend.Reset(context.Background(), ""))
	})

	t.Run("unknown strategy returns nil", func(t *testing.T) {
		require.NoError(t, backend.Reset(context.Background(), "unknown"))
	})

	t.Run("model strategy with no model name returns nil", func(t *testing.T) {
		require.NoError(t, backend.Reset(context.Background(), "model"))
	})

	t.Run("server strategy returns nil", func(t *testing.T) {
		require.NoError(t, backend.Reset(context.Background(), "server"))
	})
}

func TestBackendReset_MissingOllamaBinary(t *testing.T) {
	backend := &Backend{}
	// PATH stripped: ollama cannot be found, and the failure surfaces
	// instead of being swallowed.
	t.Setenv("PATH", "")
	err := backend.Reset(context.Background(), "model")
	require.Error(t, err)
}

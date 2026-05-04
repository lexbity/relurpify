package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagedBackendAdapter_Reset(t *testing.T) {
	t.Run("none strategy returns nil", func(t *testing.T) {
		adapter := managedBackendAdapter{}
		err := adapter.Reset(context.Background(), "none")
		require.NoError(t, err)
	})

	t.Run("empty strategy returns nil", func(t *testing.T) {
		adapter := managedBackendAdapter{}
		err := adapter.Reset(context.Background(), "")
		require.NoError(t, err)
	})

	t.Run("unknown strategy returns nil", func(t *testing.T) {
		adapter := managedBackendAdapter{}
		err := adapter.Reset(context.Background(), "unknown")
		require.NoError(t, err)
	})

	t.Run("model strategy with no model name returns nil", func(t *testing.T) {
		adapter := managedBackendAdapter{modelName: ""}
		err := adapter.Reset(context.Background(), "model")
		require.NoError(t, err)
	})

	t.Run("server strategy returns nil", func(t *testing.T) {
		adapter := managedBackendAdapter{}
		err := adapter.Reset(context.Background(), "server")
		require.NoError(t, err)
	})
}

func TestLMStudioBackendAdapter_Reset(t *testing.T) {
	t.Run("always returns nil", func(t *testing.T) {
		adapter := lmStudioBackendAdapter{}
		err := adapter.Reset(context.Background(), "any")
		require.NoError(t, err)
	})

	t.Run("empty strategy returns nil", func(t *testing.T) {
		adapter := lmStudioBackendAdapter{}
		err := adapter.Reset(context.Background(), "")
		require.NoError(t, err)
	})
}

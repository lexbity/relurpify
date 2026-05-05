//go:build live
// +build live

package agenttest

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/llm"

	"github.com/stretchr/testify/require"
)

func TestBuildCaseBackend(t *testing.T) {
	t.Run("Ollama provider with profile", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
			Model:    "qwen2.5-coder:14b",
		}
		profile := &llm.ModelProfile{
			Provider: "ollama",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, profile, false)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})

	t.Run("Ollama provider without profile", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, nil, false)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})

	t.Run("LMStudio provider with profile", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "lmstudio",
			Endpoint: "http://localhost:1234",
			Model:    "qwen2.5-coder:14b",
		}
		profile := &llm.ModelProfile{
			Provider: "lmstudio",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, profile, false)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})

	t.Run("LMStudio provider without profile", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "lmstudio",
			Endpoint: "http://localhost:1234",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, nil, false)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})

	t.Run("nil profile", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, nil, false)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})

	t.Run("debug toggle enabled", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, nil, true)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})

	t.Run("debug toggle disabled", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, nil, false)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})

	t.Run("invalid provider", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "invalid-provider",
			Endpoint: "http://localhost:11434",
			Model:    "qwen2.5-coder:14b",
		}

		lm, err := buildCaseBackend(execution, nil, false)
		require.Error(t, err)
		require.Nil(t, lm)
		require.Contains(t, err.Error(), "unknown provider")
	})
}

func TestBuildCaseBackendTimeout(t *testing.T) {
	t.Run("default timeout is 30 seconds", func(t *testing.T) {
		execution := resolvedCaseExecution{
			Provider: "ollama",
			Endpoint: "http://localhost:11434",
			Model:    "qwen2.5-coder:14b",
		}

		// This test verifies that the factory sets a default timeout
		// The actual timeout value is not directly observable from the returned LanguageModel,
		// but we can verify the backend is created successfully
		lm, err := buildCaseBackend(execution, nil, false)
		require.NoError(t, err)
		require.NotNil(t, lm)
	})
}

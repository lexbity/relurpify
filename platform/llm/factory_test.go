package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFactory_OllamaDefault(t *testing.T) {
	backend, err := New(ProviderConfig{
		Model: "test-model",
	})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_DefaultProvider_Ollama(t *testing.T) {
	backend, err := New(ProviderConfig{
		Model: "test-model",
	})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestDefaultConfig_ResolvesToOllama(t *testing.T) {
	backend, err := New(ProviderConfig{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_OllamaExplicit(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: "ollama",
		Model:    "test-model",
	})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_LMStudio(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: "lmstudio",
		Model:    "test-model",
	})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_UnknownProvider(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: "mystery",
		Endpoint: "http://localhost:11434",
		Model:    "test-model",
	})
	require.Error(t, err)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "mystery")
}

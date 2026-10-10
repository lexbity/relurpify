package envcomposition

import (
	"testing"

	llm "codeburg.org/lexbit/relurpify/platform/llm"
	"github.com/stretchr/testify/require"
)

// The composition root owns the set of linked provider kinds (model.go blank-
// imports the backend packages), so the kind-registry and dispatch tests for
// the backend kinds live here, not in the llm package.

func TestRegisteredKindsIncludesLinkedBackends(t *testing.T) {
	registered := llm.RegisteredKinds()
	require.ElementsMatch(t, []string{"ollama", "lmstudio", "offline", "tape", "openai_compatible"}, registered)
	require.Len(t, registered, 5, "expected exactly 5 registered kinds")
}

func TestNewDispatchesOnKind(t *testing.T) {
	backend, err := llm.New(llm.ProviderConfig{
		Kind:     "ollama",
		Endpoint: "http://localhost:11434",
		Model:    "test-model",
	}, llm.ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestNewDispatchesOnProviderWhenKindEmpty(t *testing.T) {
	backend, err := llm.New(llm.ProviderConfig{
		Provider: "ollama",
		Endpoint: "http://localhost:11434",
		Model:    "test-model",
	}, llm.ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestNewDefaultsToOllamaWhenBothEmpty(t *testing.T) {
	backend, err := llm.New(llm.ProviderConfig{
		Endpoint: "http://localhost:11434",
		Model:    "test-model",
	}, llm.ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestNew_LMStudio(t *testing.T) {
	backend, err := llm.New(llm.ProviderConfig{
		Kind:     "lmstudio",
		Endpoint: "http://localhost:1234",
		Model:    "test-model",
	}, llm.ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestIsRegisteredKind_LinkedBackends(t *testing.T) {
	require.True(t, llm.IsRegisteredKind("ollama"))
	require.True(t, llm.IsRegisteredKind("OLLAMA"))
	require.True(t, llm.IsRegisteredKind("lmstudio"))
	require.True(t, llm.IsRegisteredKind("openai_compatible"))
	require.False(t, llm.IsRegisteredKind("vllm"))
}

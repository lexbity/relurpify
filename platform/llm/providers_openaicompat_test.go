package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAICompatBackendConstructs(t *testing.T) {
	backend, err := New(ProviderConfig{
		Kind:     "openai_compatible",
		Endpoint: "http://localhost:8080/v1",
		Model:    "test-model",
	}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)

	// Verify the backend model interface is functional
	lm := backend.Model()
	require.NotNil(t, lm)
}

func TestOpenAICompatRequiresEndpoint(t *testing.T) {
	backend, err := New(ProviderConfig{
		Kind:  "openai_compatible",
		Model: "test-model",
	}, ProviderSecrets{})
	require.Error(t, err)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "endpoint required")
}

func TestOpenAICompatFromProviderField(t *testing.T) {
	// name-as-kind fallback: Provider = kind
	backend, err := New(ProviderConfig{
		Provider: "openai_compatible",
		Endpoint: "http://localhost:8080/v1",
		Model:    "test-model",
	}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestOpenAICompatCapabilities(t *testing.T) {
	backend, err := New(ProviderConfig{
		Kind:              "openai_compatible",
		Endpoint:          "http://localhost:8080/v1",
		Model:             "test-model",
		NativeToolCalling: true,
	}, ProviderSecrets{})
	require.NoError(t, err)

	caps := backend.Capabilities()
	require.True(t, caps.Streaming)
	require.True(t, caps.ModelListing)
	require.True(t, caps.NativeToolCalling)
	require.True(t, caps.Embeddings)
}

func TestOpenAICompatSetProfile(t *testing.T) {
	backend, err := New(ProviderConfig{
		Kind:     "openai_compatible",
		Endpoint: "http://localhost:8080/v1",
		Model:    "test-model",
	}, ProviderSecrets{})
	require.NoError(t, err)

	// SetProfile should not panic
	backend.SetProfile(nil)
	backend.SetProfile(&ModelProfile{})
}

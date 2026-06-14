package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	factoryTestEndpoint         = "http://localhost:11434"
	factoryTestProviderOllama   = "ollama"
	factoryTestProviderLMStudio = "lmstudio"
	factoryTestProviderOffline  = "offline"
	factoryTestProviderTape     = "tape"
	factoryTestModel            = "test-model"
	factoryTestTapeModel        = "tape-model"
	factoryTestTapeHeaderKind   = "_header"
	factoryTestTapeGenerateKind = "generate"
	factoryTestTapePrompt       = "hello"
	factoryTestTapeResponse     = "hello from tape"
)

func TestRegisteredProvidersIncludesBuiltinsAndOffline(t *testing.T) {
	registered := RegisteredProviders()
	require.ElementsMatch(t, []string{"ollama", "lmstudio", "offline", "tape"}, registered)
}

func TestRegisterProviderPanicsOnDuplicateInTests(t *testing.T) {
	require.Panics(t, func() {
		RegisterProvider(factoryTestProviderOffline, func(ProviderConfig, ProviderSecrets) (ManagedBackend, error) {
			return offlineBackend{}, nil
		})
	})
}

func TestFactory_OllamaDefault(t *testing.T) {
	backend, err := New(ProviderConfig{
		Model: factoryTestModel,
	}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_DefaultProvider_Ollama(t *testing.T) {
	backend, err := New(ProviderConfig{
		Model: factoryTestModel,
	}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestDefaultConfig_ResolvesToOllama(t *testing.T) {
	backend, err := New(ProviderConfig{}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_OllamaExplicit(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: factoryTestProviderOllama,
		Model:    factoryTestModel,
	}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_LMStudio(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: factoryTestProviderLMStudio,
		Model:    factoryTestModel,
	}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestFactory_TapeProvider(t *testing.T) {
	dir := t.TempDir()
	tapePath := filepath.Join(dir, "tape.jsonl")
	writeTapeFixtureFile(t, tapePath, []tapeEntry{
		{
			Kind: factoryTestTapeHeaderKind,
			Request: tapeRequest{Header: &TapeHeader{
				Kind:       factoryTestTapeHeaderKind,
				ProviderID: factoryTestProviderOllama,
				ModelName:  factoryTestTapeModel,
				SuiteName:  "suite",
				CaseName:   "case",
			}},
		},
		{
			Kind:        factoryTestTapeGenerateKind,
			Fingerprint: fingerprint(factoryTestTapeGenerateKind, tapeRequest{Prompt: factoryTestTapePrompt, Options: &LLMOptions{Model: factoryTestTapeModel}}),
			Response:    &LLMResponse{Text: factoryTestTapeResponse},
		},
	})

	backend, err := New(ProviderConfig{
		Provider: factoryTestProviderTape,
		Model:    factoryTestTapeModel,
		TapePath: tapePath,
	}, ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)

	health, err := backend.Health(context.Background())
	require.NoError(t, err)
	require.NotNil(t, health)
	require.Equal(t, BackendHealthReady, health.State)

	resp, err := backend.Model().Generate(context.Background(), factoryTestTapePrompt, &LLMOptions{Model: factoryTestTapeModel})
	require.NoError(t, err)
	require.Equal(t, factoryTestTapeResponse, resp.Text)

	models, err := backend.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, factoryTestTapeModel, models[0].Name)
}

func TestFactory_TapeProviderRequiresTapePath(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: factoryTestProviderTape,
		Model:    factoryTestTapeModel,
	}, ProviderSecrets{})
	require.Error(t, err)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "tape_path required")
}

func TestFactory_UnknownProvider(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: "mystery",
		Endpoint: factoryTestEndpoint,
		Model:    factoryTestModel,
	}, ProviderSecrets{})
	require.Error(t, err)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "mystery")
}

func writeTapeFixtureFile(t *testing.T, path string, entries []tapeEntry) {
	t.Helper()
	f, err := os.Create(filepath.Clean(path))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		require.NoError(t, enc.Encode(entry))
	}
}

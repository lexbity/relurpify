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

func TestRegisteredKindsDoesNotContainEmptyOrUnknown(t *testing.T) {
	registered := RegisteredKinds()
	for _, kind := range registered {
		require.NotEmpty(t, kind, "provider kind should not be empty")
		require.NotEqual(t, "unknown", kind)
	}
}

func TestRegisterKindPanicsOnDuplicateInTests(t *testing.T) {
	require.Panics(t, func() {
		RegisterKind(factoryTestProviderOffline, func(ProviderConfig, ProviderSecrets) (ManagedBackend, error) {
			return offlineBackend{}, nil
		})
	})
}

func TestNew_TapeProvider(t *testing.T) {
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
		Kind:     factoryTestProviderTape,
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

func TestNew_TapeProviderFromProviderField(t *testing.T) {
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
}

func TestNew_TapeProviderRequiresTapePath(t *testing.T) {
	backend, err := New(ProviderConfig{
		Kind:  factoryTestProviderTape,
		Model: factoryTestTapeModel,
	}, ProviderSecrets{})
	require.Error(t, err)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "tape_path required")
}

func TestNew_UnknownKind(t *testing.T) {
	backend, err := New(ProviderConfig{
		Kind:     "vllm",
		Endpoint: factoryTestEndpoint,
		Model:    factoryTestModel,
	}, ProviderSecrets{})
	require.Error(t, err)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "unknown provider kind")
}

func TestNew_UnknownProvider(t *testing.T) {
	backend, err := New(ProviderConfig{
		Provider: "mystery",
		Endpoint: factoryTestEndpoint,
		Model:    factoryTestModel,
	}, ProviderSecrets{})
	require.Error(t, err)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "unknown provider kind")
}

func TestIsRegisteredKind(t *testing.T) {
	require.True(t, IsRegisteredKind("offline"))
	require.True(t, IsRegisteredKind("tape"))
	require.False(t, IsRegisteredKind("vllm"))
	require.False(t, IsRegisteredKind(""))
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

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveModelRef_BothFields(t *testing.T) {
	providers := []*ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434", AvailableModels: []string{"gemma4:e4b"}},
	}
	resolved, err := ResolveModelRef(ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, ModelRef{}, providers)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Equal(t, "ollama", resolved.Provider.Name)
	require.Equal(t, "gemma4:e4b", resolved.Name)
}

func TestResolveModelRef_NameOnly(t *testing.T) {
	providers := []*ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434", AvailableModels: []string{"gemma4:e4b"}},
	}
	resolved, err := ResolveModelRef(ModelRef{Name: "gemma4:e4b"}, ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, providers)
	require.NoError(t, err)
	require.Equal(t, "ollama", resolved.Provider.Name)
	require.Equal(t, "gemma4:e4b", resolved.Name)
}

func TestResolveModelRef_ProviderOnly(t *testing.T) {
	providers := []*ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434", AvailableModels: []string{"gemma4:e4b"}},
	}
	resolved, err := ResolveModelRef(ModelRef{Provider: "ollama"}, ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, providers)
	require.NoError(t, err)
	require.Equal(t, "ollama", resolved.Provider.Name)
	require.Equal(t, "gemma4:e4b", resolved.Name)
}

func TestResolveModelRef_Neither(t *testing.T) {
	providers := []*ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434", AvailableModels: []string{"gemma4:e4b"}},
	}
	resolved, err := ResolveModelRef(ModelRef{}, ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, providers)
	require.NoError(t, err)
	require.Equal(t, "ollama", resolved.Provider.Name)
	require.Equal(t, "gemma4:e4b", resolved.Name)
}

func TestResolveModelRef_UnknownProvider(t *testing.T) {
	providers := []*ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434"},
	}
	_, err := ResolveModelRef(ModelRef{Provider: "missing", Name: "gemma4:e4b"}, ModelRef{}, providers)
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider \"missing\" not found")
	require.Contains(t, err.Error(), "ollama")
}

func TestResolveModelRef_ModelNotInAvailableList(t *testing.T) {
	providers := []*ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434", AvailableModels: []string{"gemma4:e4b"}},
	}
	_, err := ResolveModelRef(ModelRef{Provider: "ollama", Name: "qwen2.5-coder:14b"}, ModelRef{}, providers)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not listed in provider \"ollama\".available_models")
	require.Contains(t, err.Error(), "gemma4:e4b")
}

func TestResolveModelRef_EmptyAvailableModels(t *testing.T) {
	providers := []*ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434"},
	}
	resolved, err := ResolveModelRef(ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, ModelRef{}, providers)
	require.NoError(t, err)
	require.Equal(t, "ollama", resolved.Provider.Name)
	require.Equal(t, "gemma4:e4b", resolved.Name)
}

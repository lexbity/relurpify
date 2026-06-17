package runtime

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
	"github.com/stretchr/testify/require"
)

func TestProviderDefinitionFromResolved_MapsAllFields(t *testing.T) {
	r := &model.ResolvedProvider{
		Schema:                "relurpify/model/provider/v1",
		Name:                  "test-provider",
		Kind:                  "ollama",
		Endpoint:              "http://localhost:11434",
		RequestTimeoutSeconds: 120,
		AvailableModels:       []string{"model-a", "model-b"},
		NativeToolCalling:     true,
		MaxConcurrent:         2,
		Description:           "A test provider",
		SetupHint:             "Run `test` to verify",
		SourcePath:            "/tmp/test.provider.yaml",
	}
	def := providerDefinitionFromResolved(r)
	require.Equal(t, r.Name, def.Name)
	require.Equal(t, r.Kind, def.Kind)
	require.Equal(t, r.Endpoint, def.Endpoint)
	require.Equal(t, r.RequestTimeoutSeconds, def.RequestTimeoutSeconds)
	require.Equal(t, r.AvailableModels, def.AvailableModels)
	require.Equal(t, r.NativeToolCalling, def.NativeToolCalling)
	require.Equal(t, r.MaxConcurrent, def.MaxConcurrent)
	require.Equal(t, r.Description, def.Description)
	require.Equal(t, r.SetupHint, def.SetupHint)
	require.Equal(t, r.SourcePath, def.SourcePath)
}

func TestConverterCarriesHints(t *testing.T) {
	r := &model.ResolvedProvider{
		Name:      "custom",
		Kind:      "ollama",
		Endpoint:  "http://localhost:11434",
		Description: "Custom provider description",
		SetupHint:   "Custom setup instructions",
	}
	def := providerDefinitionFromResolved(r)
	require.Equal(t, "Custom provider description", def.Description)
	require.Equal(t, "Custom setup instructions", def.SetupHint)
}

func TestConverterCarriesHints_Empty(t *testing.T) {
	r := &model.ResolvedProvider{
		Name:     "minimal",
		Kind:     "ollama",
		Endpoint: "http://localhost:11434",
	}
	def := providerDefinitionFromResolved(r)
	require.Empty(t, def.Description)
	require.Empty(t, def.SetupHint)
}

func TestProviderDefinitionFromResolved_Nil(t *testing.T) {
	def := providerDefinitionFromResolved(nil)
	require.Empty(t, def.Name)
}

func TestProviderDefinitionFromResolved_CopiesAvailableModels(t *testing.T) {
	models := []string{"m1", "m2"}
	r := &model.ResolvedProvider{
		Name:            "p",
		Kind:            "ollama",
		Endpoint:        "http://localhost:11434",
		AvailableModels: models,
	}
	def := providerDefinitionFromResolved(r)
	require.Equal(t, models, def.AvailableModels)
	// Mutating the original should not affect the def
	models[0] = "changed"
	require.NotEqual(t, models, def.AvailableModels)
}

func TestBuildProviderRegistry_EmptyInput(t *testing.T) {
	reg, err := buildProviderRegistry(nil)
	require.NoError(t, err)
	require.NotNil(t, reg)
	_, found := reg.Resolve("anything")
	require.False(t, found)
}

func TestBuildProviderRegistry_EmptySlice(t *testing.T) {
	reg, err := buildProviderRegistry([]*model.ResolvedProvider{})
	require.NoError(t, err)
	require.NotNil(t, reg)
	_, found := reg.Resolve("anything")
	require.False(t, found)
}

func TestBuildProviderRegistry_FromProviders(t *testing.T) {
	providers := []*model.ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434"},
		{Name: "lmstudio", Kind: "lmstudio", Endpoint: "http://localhost:1234"},
	}
	reg, err := buildProviderRegistry(providers)
	require.NoError(t, err)
	require.NotNil(t, reg)

	def, found := reg.Resolve("ollama")
	require.True(t, found)
	require.Equal(t, "http://localhost:11434", def.Endpoint)
	require.Equal(t, "ollama", def.Kind)

	def, found = reg.Resolve("lmstudio")
	require.True(t, found)
	require.Equal(t, "http://localhost:1234", def.Endpoint)
	require.Equal(t, "lmstudio", def.Kind)

	_, found = reg.Resolve("nonexistent")
	require.False(t, found)
}

func TestBuildProviderRegistry_DuplicateName(t *testing.T) {
	providers := []*model.ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434"},
		{Name: "ollama", Kind: "ollama", Endpoint: "http://other:11434"},
	}
	_, err := buildProviderRegistry(providers)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")
}

func TestProviderCatalogResolve_ReturnsKind(t *testing.T) {
	providers := []*model.ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434"},
	}
	reg, _ := buildProviderRegistry(providers)
	def, found := reg.Resolve("ollama")
	require.True(t, found)
	require.Equal(t, "ollama", def.Kind)
}

func TestProviderCatalogResolve_ReturnsKindMismatch(t *testing.T) {
	// A provider whose name differs from its kind
	providers := []*model.ResolvedProvider{
		{Name: "my-llama", Kind: "ollama", Endpoint: "http://localhost:11434"},
	}
	reg, _ := buildProviderRegistry(providers)
	def, found := reg.Resolve("my-llama")
	require.True(t, found)
	require.Equal(t, "ollama", def.Kind)
	require.Equal(t, "my-llama", def.Name)
}

func TestProviderCatalogResolve_EndpointFromCatalog(t *testing.T) {
	providers := []*model.ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://custom:11434"},
	}
	reg, _ := buildProviderRegistry(providers)
	def, found := reg.Resolve("ollama")
	require.True(t, found)
	require.Equal(t, "http://custom:11434", def.Endpoint)
}

func TestProviderCatalogResolve_RequestTimeout(t *testing.T) {
	providers := []*model.ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434", RequestTimeoutSeconds: 300},
	}
	reg, _ := buildProviderRegistry(providers)
	def, found := reg.Resolve("ollama")
	require.True(t, found)
	require.Equal(t, 300, def.RequestTimeoutSeconds)
}

func TestProviderCatalogResolve_NativeToolCalling(t *testing.T) {
	providers := []*model.ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434", NativeToolCalling: true},
		{Name: "lmstudio", Kind: "lmstudio", Endpoint: "http://localhost:1234", NativeToolCalling: false},
	}
	reg, _ := buildProviderRegistry(providers)
	
	def, found := reg.Resolve("ollama")
	require.True(t, found)
	require.True(t, def.NativeToolCalling)

	def, found = reg.Resolve("lmstudio")
	require.True(t, found)
	require.False(t, def.NativeToolCalling)
}

// Test that the llm.New function dispatches on Kind from the registry
func TestNewWithKindFromCatalog(t *testing.T) {
	reg, err := buildProviderRegistry([]*model.ResolvedProvider{
		{Name: "ollama", Kind: "ollama", Endpoint: "http://localhost:11434"},
	})
	require.NoError(t, err)

	def, found := reg.Resolve("ollama")
	require.True(t, found)

	backend, err := llm.New(llm.ProviderConfig{
		Kind:     def.Kind,
		Endpoint: def.Endpoint,
		Model:    "test-model",
	}, llm.ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestNewWithProviderFallbackFromCatalog(t *testing.T) {
	// When Kind is empty, Provider is used as the kind (name-as-kind fallback)
	backend, err := llm.New(llm.ProviderConfig{
		Provider: "ollama",
		Endpoint: "http://localhost:11434",
		Model:    "test-model",
	}, llm.ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

func TestNewUnknownKind(t *testing.T) {
	_, err := llm.New(llm.ProviderConfig{
		Kind: "vllm",
	}, llm.ProviderSecrets{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown provider kind")
}

func TestOpenAICompatFromCatalog(t *testing.T) {
	reg, err := buildProviderRegistry([]*model.ResolvedProvider{
		{
			Name:     "my-openai",
			Kind:     "openai_compatible",
			Endpoint: "http://custom:8080/v1",
		},
	})
	require.NoError(t, err)

	def, found := reg.Resolve("my-openai")
	require.True(t, found)
	require.Equal(t, "openai_compatible", def.Kind)
	require.Equal(t, "http://custom:8080/v1", def.Endpoint)

	backend, err := llm.New(llm.ProviderConfig{
		Kind:     def.Kind,
		Endpoint: def.Endpoint,
		Model:    "test-model",
	}, llm.ProviderSecrets{})
	require.NoError(t, err)
	require.NotNil(t, backend)
}

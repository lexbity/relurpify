package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_UnknownKind(t *testing.T) {
	err := (ProviderConfig{
		Kind:     "vllm",
		Endpoint: "http://localhost:8000",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown provider kind")
}

func TestValidate_UnknownKindFromProvider(t *testing.T) {
	err := (ProviderConfig{
		Provider: "mystery",
		Endpoint: "http://localhost:8000",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown provider kind")
}

func TestValidate_EndpointRequiredForOllama(t *testing.T) {
	err := (ProviderConfig{
		Kind: "ollama",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "endpoint required")
}

func TestValidate_EndpointRequiredForOllamaFromProvider(t *testing.T) {
	err := (ProviderConfig{
		Provider: "ollama",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "endpoint required")
}

func TestValidate_EndpointRequiredForLMStudio(t *testing.T) {
	err := (ProviderConfig{
		Kind: "lmstudio",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "endpoint required")
}

func TestValidate_TapeRequiresPath(t *testing.T) {
	err := (ProviderConfig{
		Kind: "tape",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "tape_path required")
}

func TestValidate_TapeRequiresPathFromProvider(t *testing.T) {
	err := (ProviderConfig{
		Provider: "tape",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "tape_path required")
}

func TestValidate_TapeWithPath(t *testing.T) {
	err := (ProviderConfig{
		Kind:     "tape",
		TapePath: "/tmp/test.tape.jsonl",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_OfflineNoEndpoint(t *testing.T) {
	err := (ProviderConfig{
		Kind: "offline",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_OfflineFromProvider(t *testing.T) {
	err := (ProviderConfig{
		Provider: "offline",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_OllamaWithEndpoint(t *testing.T) {
	err := (ProviderConfig{
		Kind:     "ollama",
		Endpoint: "http://localhost:11434",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_LMStudioWithEndpoint(t *testing.T) {
	err := (ProviderConfig{
		Kind:     "lmstudio",
		Endpoint: "http://localhost:1234",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_OpenAICompatibleRequiresEndpoint(t *testing.T) {
	err := (ProviderConfig{
		Kind: "openai_compatible",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "endpoint required")
}

func TestValidate_OpenAICompatibleWithEndpoint(t *testing.T) {
	err := (ProviderConfig{
		Kind:     "openai_compatible",
		Endpoint: "http://localhost:8080/v1",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_NegativeTimeout(t *testing.T) {
	err := (ProviderConfig{
		Kind:     "ollama",
		Endpoint: "http://localhost:11434",
		Timeout:  -1,
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout must be >= 0")
}

func TestValidate_ZeroTimeout(t *testing.T) {
	err := (ProviderConfig{
		Kind:     "ollama",
		Endpoint: "http://localhost:11434",
		Timeout:  0,
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_KindPrevailsOverProvider(t *testing.T) {
	// When both are set, Kind is the effective kind.
	err := (ProviderConfig{
		Kind:     "ollama",
		Provider: "tape",
		Endpoint: "http://localhost:11434",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_KindPrevailsEvenWhenProviderIsGhost(t *testing.T) {
	// Kind takes precedence, so even if Provider is a ghost name,
	// validation uses Kind.
	err := (ProviderConfig{
		Kind:     "ollama",
		Provider: "vllm",
		Endpoint: "http://localhost:11434",
	}).Validate()
	require.NoError(t, err)
}

func TestValidate_EmptyKindAndProvider(t *testing.T) {
	err := (ProviderConfig{}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider kind required")
}

func TestKindRequiresEndpoint(t *testing.T) {
	require.True(t, kindRequiresEndpoint("ollama"))
	require.True(t, kindRequiresEndpoint("OLLAMA"))
	require.True(t, kindRequiresEndpoint("lmstudio"))
	require.True(t, kindRequiresEndpoint("openai_compatible"))
	require.False(t, kindRequiresEndpoint("tape"))
	require.False(t, kindRequiresEndpoint("offline"))
	require.False(t, kindRequiresEndpoint(""))
	require.False(t, kindRequiresEndpoint("vllm"))
	require.False(t, kindRequiresEndpoint("unknown"))
}

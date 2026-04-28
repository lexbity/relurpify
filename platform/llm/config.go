package llm

import (
	"fmt"
	"strings"
	"time"
)

// ProviderConfig captures backend construction settings for the managed
// backend factory.
type ProviderConfig struct {
	Provider          string         `yaml:"provider" json:"provider"`
	Endpoint          string         `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model             string         `yaml:"model,omitempty" json:"model,omitempty"`
	ModelPath         string         `yaml:"model_path,omitempty" json:"model_path,omitempty"`
	APIKey            string         `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Timeout           time.Duration  `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	NativeToolCalling bool           `yaml:"native_tool_calling,omitempty" json:"native_tool_calling,omitempty"`
	Debug             bool           `yaml:"debug,omitempty" json:"debug,omitempty"`
	Config            map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// RuntimeConfigSource is implemented by runtime config structs that can be
// normalized into a ProviderConfig without importing those packages here.
type RuntimeConfigSource interface {
	InferenceProviderValue() string
	InferenceEndpointValue() string
	InferenceModelValue() string
	InferenceAPIKeyValue() string
	InferenceNativeToolCallingValue() bool
}

// ProviderConfigFromRuntimeConfig maps a runtime config into a provider manifest.
func ProviderConfigFromRuntimeConfig(cfg RuntimeConfigSource) ProviderConfig {
	if cfg == nil {
		return ProviderConfig{}
	}
	return ProviderConfig{
		Provider:          cfg.InferenceProviderValue(),
		Endpoint:          cfg.InferenceEndpointValue(),
		Model:             cfg.InferenceModelValue(),
		APIKey:            cfg.InferenceAPIKeyValue(),
		NativeToolCalling: cfg.InferenceNativeToolCallingValue(),
	}
}

// Validate checks the provider config for basic completeness.
func (c ProviderConfig) Validate() error {
	if strings.TrimSpace(c.Provider) == "" {
		return fmt.Errorf("provider required")
	}
	if isTransportProvider(c.Provider) && strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("provider %q endpoint required", c.Provider)
	}
	if c.Timeout < 0 {
		return fmt.Errorf("provider %q timeout must be >= 0", c.Provider)
	}
	return nil
}

func isTransportProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "ollama", "lmstudio", "openai-compat", "openai_compat", "openai", "vllm", "tgi", "llama-server", "llama_server":
		return true
	default:
		return false
	}
}

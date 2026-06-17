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
	Kind              string         `yaml:"kind,omitempty" json:"kind,omitempty"`
	Endpoint          string         `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model             string         `yaml:"model,omitempty" json:"model,omitempty"`
	ModelPath         string         `yaml:"model_path,omitempty" json:"model_path,omitempty"`
	TapePath          string         `yaml:"tape_path,omitempty" json:"tape_path,omitempty"`
	Timeout           time.Duration  `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	NativeToolCalling bool           `yaml:"native_tool_calling,omitempty" json:"native_tool_calling,omitempty"`
	Debug             bool           `yaml:"debug,omitempty" json:"debug,omitempty"`
	Config            map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// ProviderSecrets carries env-only credentials for a provider.
type ProviderSecrets struct {
	APIKey string
}

// RuntimeConfigSource is implemented by runtime config structs that can be
// normalized into a ProviderConfig without importing those packages here.
type RuntimeConfigSource interface {
	InferenceProviderValue() string
	InferenceEndpointValue() string
	InferenceModelValue() string
	InferenceTapePathValue() string
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
		TapePath:          cfg.InferenceTapePathValue(),
		NativeToolCalling: cfg.InferenceNativeToolCallingValue(),
	}
}

// Validate checks the provider config for basic completeness.
// It resolves the effective kind from c.Kind (preferred) or c.Provider,
// checks that the kind is registered, then validates transport-specific
// requirements.
func (c ProviderConfig) Validate() error {
	kind := strings.ToLower(strings.TrimSpace(c.Kind))
	if kind == "" {
		kind = strings.ToLower(strings.TrimSpace(c.Provider))
	}
	if kind == "" {
		return fmt.Errorf("provider kind required")
	}
	if !IsRegisteredKind(kind) {
		return fmt.Errorf("unknown provider kind %q", kind)
	}
	if kindRequiresEndpoint(kind) && strings.TrimSpace(c.Endpoint) == "" {
		return fmt.Errorf("provider kind %q endpoint required", kind)
	}
	if kind == "tape" && strings.TrimSpace(c.TapePath) == "" {
		return fmt.Errorf("provider kind %q tape_path required", kind)
	}
	if c.Timeout < 0 {
		return fmt.Errorf("provider kind %q timeout must be >= 0", kind)
	}
	return nil
}

// kindRequiresEndpoint returns true when a provider kind needs a network
// endpoint to function.
func kindRequiresEndpoint(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "ollama", "lmstudio", "openai_compatible":
		return true
	default:
		return false
	}
}

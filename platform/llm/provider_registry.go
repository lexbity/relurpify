package llm

import (
	"fmt"
	"strings"
)

// ProviderDefinition describes a loaded model provider file.
type ProviderDefinition struct {
	Name                  string
	Kind                  string
	Endpoint              string
	RequestTimeoutSeconds int
	AvailableModels       []string
	NativeToolCalling     bool
	MaxConcurrent         int
	SourcePath            string
}

// ProviderRegistry stores provider definitions indexed by name.
type ProviderRegistry struct {
	byName map[string]ProviderDefinition
}

// NewProviderRegistryFromDefinitions builds a registry from already-loaded
// provider definitions. The framework/llmconfig adapter converts YAML provider
// configs into these definitions, keeping platform/llm free of any dependency
// on framework configuration loaders.
func NewProviderRegistryFromDefinitions(defs []ProviderDefinition) (*ProviderRegistry, error) {
	reg := &ProviderRegistry{byName: map[string]ProviderDefinition{}}
	for _, def := range defs {
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if key == "" {
			continue
		}
		if _, exists := reg.byName[key]; exists {
			return nil, fmt.Errorf("duplicate provider definition %q", def.Name)
		}
		reg.byName[key] = def
	}
	return reg, nil
}

// Resolve returns the named provider definition.
func (r *ProviderRegistry) Resolve(name string) (ProviderDefinition, bool) {
	if r == nil {
		return ProviderDefinition{}, false
	}
	def, ok := r.byName[strings.ToLower(strings.TrimSpace(name))]
	return def, ok
}

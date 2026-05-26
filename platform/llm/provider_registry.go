package llm

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgmodel "codeburg.org/lexbit/relurpify/framework/cfgload/model"
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

// NewProviderRegistry loads model provider files from a directory.
func NewProviderRegistry(dir string) (*ProviderRegistry, error) {
	loaded, err := cfgmodel.LoadProviderDir(dir, cfgload.StrictDecode)
	if err != nil {
		return nil, err
	}
	reg := &ProviderRegistry{byName: map[string]ProviderDefinition{}}
	for _, provider := range loaded {
		if provider == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(provider.Name))
		if key == "" {
			continue
		}
		if _, exists := reg.byName[key]; exists {
			return nil, fmt.Errorf("duplicate provider definition %q", provider.Name)
		}
		reg.byName[key] = ProviderDefinition{
			Name:                  provider.Name,
			Kind:                  provider.Kind,
			Endpoint:              provider.Endpoint,
			RequestTimeoutSeconds: provider.RequestTimeoutSeconds,
			AvailableModels:       append([]string(nil), provider.AvailableModels...),
			NativeToolCalling:     provider.NativeToolCalling,
			MaxConcurrent:         provider.MaxConcurrent,
			SourcePath:            provider.SourcePath,
		}
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

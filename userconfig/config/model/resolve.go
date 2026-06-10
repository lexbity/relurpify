package model

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ModelRef is the parsed form of a model declaration in any config file.
// Both fields are optional in YAML and represented as empty strings when absent.
type ModelRef struct {
	Provider string `yaml:"provider,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

// ResolvedModelRef is the result of running ModelRef through the provider registry.
type ResolvedModelRef struct {
	Provider *ResolvedProvider
	Name     string
	Profile  *ModelProfileConfig
}

// ResolveModelRef resolves a ModelRef against the provider registry using the workspace default for missing fields.
func ResolveModelRef(ref ModelRef, workspaceDefault ModelRef, providers []*ResolvedProvider) (*ResolvedModelRef, error) {
	resolvedProvider := strings.TrimSpace(ref.Provider)
	if resolvedProvider == "" {
		resolvedProvider = strings.TrimSpace(workspaceDefault.Provider)
	}
	if resolvedProvider == "" {
		return nil, fmt.Errorf("no provider specified and workspace has no default")
	}

	resolvedName := strings.TrimSpace(ref.Name)
	if resolvedName == "" {
		resolvedName = strings.TrimSpace(workspaceDefault.Name)
	}
	if resolvedName == "" {
		return nil, fmt.Errorf("no model name specified and workspace has no default")
	}

	provider := findProviderByName(providers, resolvedProvider)
	if provider == nil {
		return nil, fmt.Errorf("provider %q not found; available: [%s]", resolvedProvider, strings.Join(providerNames(providers), ", "))
	}

	if len(provider.AvailableModels) > 0 && !slices.Contains(provider.AvailableModels, resolvedName) {
		return nil, fmt.Errorf("model %q not listed in provider %q.available_models; available: [%s]", resolvedName, provider.Name, strings.Join(provider.AvailableModels, ", "))
	}

	return &ResolvedModelRef{Provider: provider, Name: resolvedName}, nil
}

func findProviderByName(providers []*ResolvedProvider, name string) *ResolvedProvider {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if strings.ToLower(strings.TrimSpace(provider.Name)) == name {
			return provider
		}
	}
	return nil
}

func providerNames(providers []*ResolvedProvider) []string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		name := strings.TrimSpace(provider.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ProviderRequiresAuth reports whether a provider kind requires an API key.
func ProviderRequiresAuth(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "openai_compatible":
		return true
	default:
		return false
	}
}

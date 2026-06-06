package modelselect

import (
	"fmt"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
)

// ProviderDefinition describes a resolved provider within a ProviderRegistry.
type ProviderDefinition struct {
	Name              string
	Endpoint          string
	Kind              string
	AvailableModels   []string
	NativeToolCalling bool
	MaxConcurrent     int
	SourcePath        string
}

// ProviderRegistry indexes provider definitions keyed by normalized name.
type ProviderRegistry struct {
	providers []*ProviderDefinition
}

// NewProviderRegistry builds a registry from config-file provider definitions.
func NewProviderRegistry(configs []*model.ResolvedProvider) *ProviderRegistry {
	reg := &ProviderRegistry{}
	for _, cfg := range configs {
		reg.providers = append(reg.providers, &ProviderDefinition{
			Name:              cfg.Name,
			Endpoint:          cfg.Endpoint,
			Kind:              cfg.Kind,
			AvailableModels:   append([]string{}, cfg.AvailableModels...),
			NativeToolCalling: cfg.NativeToolCalling,
			MaxConcurrent:     cfg.MaxConcurrent,
			SourcePath:        cfg.SourcePath,
		})
	}
	return reg
}

// Resolve a provider by name (case-insensitive).
func (r *ProviderRegistry) Resolve(name string) *ProviderDefinition {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range r.providers {
		if strings.ToLower(strings.TrimSpace(p.Name)) == name {
			return p
		}
	}
	return nil
}

// ProviderRequiresAuth checks if a provider kind needs authentication.
func ProviderRequiresAuth(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "ollama", "lmstudio":
		return false
	case "openai_compatible", "":
		return true
	default:
		return true
	}
}

var validProviderPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// validateProviderName returns an error if the name is invalid.
func validateProviderName(name string) error {
	if !validProviderPattern.MatchString(name) {
		return fmt.Errorf("provider name %q must match %s", name, validProviderPattern.String())
	}
	return nil
}

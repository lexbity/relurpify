package runtime

import (
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
)

// providerDefinitionFromResolved converts a catalog ResolvedProvider into a
// leaf-ward ProviderDefinition. This converter is identity-checked by
// struct_identity_check to ensure all fields are mapped.
func providerDefinitionFromResolved(r *model.ResolvedProvider) llm.ProviderDefinition {
	if r == nil {
		return llm.ProviderDefinition{}
	}
	return llm.ProviderDefinition{
		Name:                  r.Name,
		Kind:                  r.Kind,
		Endpoint:              r.Endpoint,
		RequestTimeoutSeconds: r.RequestTimeoutSeconds,
		AvailableModels:       append([]string(nil), r.AvailableModels...),
		NativeToolCalling:     r.NativeToolCalling,
		MaxConcurrent:         r.MaxConcurrent,
		Description:           r.Description,
		SetupHint:             r.SetupHint,
		SourcePath:            r.SourcePath,
	}
}

// buildProviderRegistry builds a ProviderRegistry from a list of catalog providers.
// Returns a nil registry when the list is empty (safe for Resolve).
func buildProviderRegistry(providers []*model.ResolvedProvider) (*llm.ProviderRegistry, error) {
	defs := make([]llm.ProviderDefinition, 0, len(providers))
	for _, r := range providers {
		defs = append(defs, providerDefinitionFromResolved(r))
	}
	return llm.NewProviderRegistryFromDefinitions(defs)
}

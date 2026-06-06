package capability

import (
	"fmt"

	agentspec "codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// LLMToolSpecFromDescriptor extracts the fields needed for LLM tool calling
// from a CapabilityDescriptor. The full descriptor stays in framework/core;
// only Name, Description, and InputSchema are passed to the LLM layer.
//
// Remote descriptions are wrapped in a provenance-labelled data fence so the
// model treats them as untrusted data, not instructions (SEC-6).
func LLMToolSpecFromDescriptor(d CapabilityDescriptor) contracts.LLMToolSpec {
	name := d.Name
	if name == "" {
		name = d.ID
	}
	desc := fencedDescription(d)
	return contracts.LLMToolSpec{
		Name:        name,
		Description: desc,
		InputSchema: d.InputSchema,
	}
}

// fencedDescription returns the description wrapped in a provenance fence for
// remote capabilities. For local capabilities the description is returned as-is.
func fencedDescription(d CapabilityDescriptor) string {
	if d.Source.Scope != agentspec.CapabilityScopeRemote {
		return d.Description
	}
	provider := d.Source.ProviderID
	if provider == "" {
		provider = "remote"
	}
	return fmt.Sprintf("[Remote tool description from provider %q: %s]", provider, d.Description)
}

// LLMToolSpecsFromDescriptors converts a slice of CapabilityDescriptors to
// LLMToolSpec values for passing to ChatWithTools.
func LLMToolSpecsFromDescriptors(descs []CapabilityDescriptor) []contracts.LLMToolSpec {
	if len(descs) == 0 {
		return nil
	}
	specs := make([]contracts.LLMToolSpec, len(descs))
	for i, d := range descs {
		specs[i] = LLMToolSpecFromDescriptor(d)
	}
	return specs
}

package core

import "codeburg.org/lexbit/relurpify/platform/contracts"

// LLMToolSpecFromDescriptor extracts the fields needed for LLM tool calling
// from a CapabilityDescriptor. The full descriptor stays in framework/core;
// only Name, Description, and InputSchema are passed to the LLM layer.
func LLMToolSpecFromDescriptor(d CapabilityDescriptor) contracts.LLMToolSpec {
	name := d.Name
	if name == "" {
		name = d.ID
	}
	return contracts.LLMToolSpec{
		Name:        name,
		Description: d.Description,
		InputSchema: d.InputSchema,
	}
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

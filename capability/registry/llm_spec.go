package registry

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
	"codeburg.org/lexbit/relurpify/model"
)

// LLMToolSpecFromDescriptor extracts the fields needed for LLM tool calling
// from a descriptor.CapabilityDescriptor. The full descriptor stays in framework/core;
// only Name, Description, and InputSchema are passed to the LLM layer.
//
// Remote descriptions are wrapped in a provenance-labelled data fence so the
// model treats them as untrusted data, not instructions (SEC-6).
func LLMToolSpecFromDescriptor(d descriptor.CapabilityDescriptor) model.LLMToolSpec {
	name := d.Name
	if name == "" {
		name = d.ID
	}
	desc := fencedDescription(d)
	return model.LLMToolSpec{
		Name:        name,
		Description: desc,
		InputSchema: convertSchema(d.InputSchema),
	}
}

// fencedDescription returns the description wrapped in a provenance fence for
// remote capabilities. For local capabilities the description is returned as-is.
func fencedDescription(d descriptor.CapabilityDescriptor) string {
	if d.Source.Scope != taxonomy.CapabilityScopeRemote {
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
func LLMToolSpecsFromDescriptors(descs []descriptor.CapabilityDescriptor) []model.LLMToolSpec {
	if len(descs) == 0 {
		return nil
	}
	specs := make([]model.LLMToolSpec, len(descs))
	for i, d := range descs {
		specs[i] = LLMToolSpecFromDescriptor(d)
	}
	return specs
}

// convertSchema copies a schemacoerce.Schema to a model.Schema.
func convertSchema(src *schemacoerce.Schema) *model.Schema {
	if src == nil {
		return nil
	}
	dst := &model.Schema{
		Type:        src.Type,
		Properties:  make(map[string]*model.Schema, len(src.Properties)),
		Required:    append([]string(nil), src.Required...),
		Default:     src.Default,
		Enum:        append([]interface{}(nil), src.Enum...),
		Title:       src.Title,
		Description: src.Description,
		Format:      src.Format,
	}
	for k, v := range src.Properties {
		dst.Properties[k] = convertSchema(v)
	}
	if src.Items != nil {
		dst.Items = convertSchema(src.Items)
	}
	return dst
}

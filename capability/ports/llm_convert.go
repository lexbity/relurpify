package ports

import (
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
)

// LLMToolSpecFromTool converts a Tool to an LLMToolSpec.
func LLMToolSpecFromTool(t Tool) LLMToolSpec {
	spec := LLMToolSpec{
		Name:        t.Name(),
		Description: t.Description(),
	}
	params := t.Parameters()
	if len(params) > 0 {
		props := make(map[string]*schemacoerce.Schema, len(params))
		var required []string
		for _, p := range params {
			prop := &schemacoerce.Schema{
				Type:        string(p.Type),
				Description: p.Description,
			}
			if p.Default != nil {
				prop.Default = p.Default
			}
			props[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		spec.InputSchema = &schemacoerce.Schema{
			Type:       "object",
			Properties: props,
			Required:   required,
		}
	}
	return spec
}

// LLMToolSpecsFromTools converts a slice of Tool to LLMToolSpec values.
func LLMToolSpecsFromTools(tools []Tool) []LLMToolSpec {
	if len(tools) == 0 {
		return nil
	}
	specs := make([]LLMToolSpec, len(tools))
	for i, t := range tools {
		specs[i] = LLMToolSpecFromTool(t)
	}
	return specs
}

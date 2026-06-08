package ports

import "codeburg.org/lexbit/relurpify/model"

// LLMToolSpecFromTool converts a Tool to an LLMToolSpec.
func LLMToolSpecFromTool(t Tool) model.LLMToolSpec {
	spec := model.LLMToolSpec{
		Name:        t.Name(),
		Description: t.Description(),
	}
	params := t.Parameters()
	if len(params) > 0 {
		props := make(map[string]*model.Schema, len(params))
		var required []string
		for _, p := range params {
			prop := &model.Schema{
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
		spec.InputSchema = &model.Schema{
			Type:       "object",
			Properties: props,
			Required:   required,
		}
	}
	return spec
}

// LLMToolSpecsFromTools converts a slice of Tool to LLMToolSpec values.
func LLMToolSpecsFromTools(tools []Tool) []model.LLMToolSpec {
	if len(tools) == 0 {
		return nil
	}
	specs := make([]model.LLMToolSpec, len(tools))
	for i, t := range tools {
		specs[i] = LLMToolSpecFromTool(t)
	}
	return specs
}

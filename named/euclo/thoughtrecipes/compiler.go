package thoughtrecipe

import (
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func cloneStreamSpec(spec *surface.ThoughtRecipeStreamSpec) *surface.ThoughtRecipeStreamSpec {
	if spec == nil {
		return nil
	}
	cp := *spec
	return &cp
}


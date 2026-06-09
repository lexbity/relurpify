package thoughtrecipe

import (
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

func depsWithRegistry(deps *paradigm.Deps, reg *registry.CapabilityRegistry) *paradigm.Deps {
	if deps == nil {
		return &paradigm.Deps{Registry: reg}
	}
	next := *deps
	next.Registry = reg
	return &next
}

package golang

import registry "codeburg.org/lexbit/relurpify/capability/registry"

func toStringSliceValue(value any) ([]string, error) {
	return registry.NormalizeStringSlice(value)
}

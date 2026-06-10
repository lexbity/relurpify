package python

import registry "codeburg.org/lexbit/relurpify/capability/registry"

func atoiSafe(value string) int {
	var total int
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return total
		}
		total = total*10 + int(ch-'0')
	}
	return total
}

func toStringSliceValue(value any) ([]string, error) {
	return registry.NormalizeStringSlice(value)
}

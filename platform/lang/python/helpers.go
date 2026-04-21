package python

import frameworktools "codeburg.org/lexbit/relurpify/framework/capability"

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

func firstNonEmptyLine(text string) string {
	start := 0
	for i := 0; i <= len(text); i++ {
		if i < len(text) && text[i] != '\n' {
			continue
		}
		line := text[start:i]
		start = i + 1
		for len(line) > 0 && (line[0] == ' ' || line[0] == '\t' || line[0] == '\r') {
			line = line[1:]
		}
		for len(line) > 0 && (line[len(line)-1] == ' ' || line[len(line)-1] == '\t' || line[len(line)-1] == '\r') {
			line = line[:len(line)-1]
		}
		if line != "" {
			return line
		}
	}
	return ""
}

func toStringSliceValue(value interface{}) ([]string, error) {
	return frameworktools.NormalizeStringSlice(value)
}

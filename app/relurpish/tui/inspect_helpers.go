package tui

import "strings"

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func joinOrNA(values []string) string {
	if len(values) == 0 {
		return "n/a"
	}
	return strings.Join(values, ", ")
}

func joinStringMap(values map[string]string) string {
	if len(values) == 0 {
		return "n/a"
	}
	parts := make([]string, 0, len(values))
	for key, value := range values {
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ", ")
}

func renderStructuredContentPreview(block StructuredContentBlock) []string {
	lines := []string{"[" + block.Type + "] " + fallback(block.Summary, "content")}
	if body := strings.TrimSpace(block.Body); body != "" {
		for _, line := range strings.Split(body, "\n") {
			lines = append(lines, "  "+line)
		}
	}
	if len(block.Provenance) > 0 {
		lines = append(lines, "  provenance: "+joinStringMap(block.Provenance))
	}
	return lines
}

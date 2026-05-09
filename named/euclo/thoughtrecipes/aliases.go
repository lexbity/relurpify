package thoughtrecipe

import "strings"

func sanitizeComponent(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

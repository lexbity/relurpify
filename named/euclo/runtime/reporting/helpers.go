package reporting

import "strings"

func stringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	default:
		return nil
	}
}

func verificationCommandNames(v any) []string {
	switch typed := v.(type) {
	case []map[string]any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, firstNonEmpty(stringValue(item["name"]), stringValue(item["command"])))
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if record, ok := item.(map[string]any); ok {
				out = append(out, firstNonEmpty(stringValue(record["name"]), stringValue(record["command"])))
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func boolValue(v any) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

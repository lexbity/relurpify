package authorization

import "strings"

// redactAny converts arbitrary structured data into a redacted representation
// suitable for persistence or export. Ported here to remove the cross-domain
// edge from governance into capability.
func redactAny(input any) any {
	if input == nil {
		return nil
	}
	switch typed := input.(type) {
	case map[string]any:
		return redactMetadataMap(typed)
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = redactValue(key, value)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactAny(item))
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue("", item))
		}
		return out
	case string:
		return redactValue("", typed)
	default:
		return input
	}
}

// redactMetadataMap redacts sensitive values from a metadata map.
func redactMetadataMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = redactValue(key, value)
	}
	return out
}

func redactValue(key string, value any) any {
	if isSensitiveKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		return redactMetadataMap(typed)
	case map[string]string:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = redactValue(k, v)
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue(key, item))
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue(key, item))
		}
		return out
	case string:
		if looksSensitiveValue(typed) {
			return "[REDACTED]"
		}
		return typed
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, needle := range []string{
		"secret", "token", "password", "cookie", "authorization", "auth", "credential", "api_key", "apikey",
	} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func looksSensitiveValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, needle := range []string{"bearer ", "ghp_", "github_pat_", "sk-", "authorization:", "session="} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

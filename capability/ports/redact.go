package ports

import "strings"

// secretArgPatterns contains parameter name substrings that may indicate
// sensitive data.
var secretArgPatterns = []string{
	"key",
	"secret",
	"token",
	"password",
	"credential",
	"auth",
	"apikey",
	"api_key",
	"api-key",
}

func isSecretArgName(name string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range secretArgPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// RedactArgs returns a shallow copy of the argument map with secret values
// replaced by "[REDACTED]".
func RedactArgs(args map[string]any, params []ToolParameter) map[string]any {
	if len(args) == 0 {
		return args
	}
	out := make(map[string]any, len(args))
	declared := make(map[string]bool, len(params))
	for _, p := range params {
		declared[strings.ToLower(p.Name)] = isSecretArgName(p.Name)
	}
	for k, v := range args {
		redact := isSecretArgName(k)
		if !redact {
			if declaredRedact, ok := declared[strings.ToLower(k)]; ok {
				redact = declaredRedact
			}
		}
		if redact {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}

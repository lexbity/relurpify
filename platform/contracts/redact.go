package contracts

import (
	"strings"
)

// secretArgPatterns contains parameter name substrings that indicate the
// argument value may contain sensitive data (API keys, tokens, passwords,
// credentials). When a parameter name matches any of these patterns, its
// value is replaced with "[REDACTED]" in log output.
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

// isSecretArgName reports whether a parameter name matches any known secret
// indicator pattern (case-insensitive comparison).
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
// replaced by "[REDACTED]". The original map is not modified. An empty or nil
// map is returned as-is. When params is non-nil, parameter names are compared
// against the declared parameter list; otherwise all keys are checked against
// the secret patterns.
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

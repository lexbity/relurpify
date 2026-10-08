package ports

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/authorization"
)

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
// replaced by "[REDACTED]". A value is redacted when its key is secret-named,
// when the manifest declares the parameter sensitive, or when the stringified
// value matches a known secret shape (JWT, AWS keys, Slack/GitHub tokens,
// PEM blocks, URL userinfo — SBH-1 D-9). The shape classifier is the single
// canonical implementation in governance/authorization; this is the thin
// capability-facing entry point that composes it (capability → governance is
// the legal domain direction and is already used by this package).
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
		if !redact {
			redact = authorization.LooksSensitiveShape(shapeString(v))
		}
		if redact {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}

// shapeString renders a scalar value to the string form used by value-shape
// redaction. Strings are matched verbatim; non-string scalars are matched on
// their printed representation so a secret cannot slip through by type.
func shapeString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// RedactStrings is the capability-facing adapter over the canonical
// governance/authorization value-shape redaction for string collections.
func RedactStrings(values []string) []string {
	return authorization.RedactStrings(values)
}

// RedactEnvPairs is the capability-facing adapter over the canonical
// pair-aware environment redaction (K=[REDACTED] for sensitive halves).
func RedactEnvPairs(env []string) []string {
	return authorization.RedactEnvPairs(env)
}

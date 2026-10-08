package authorization

import (
	"fmt"
	"regexp"
	"strings"
)

// sensitiveKeyNeedles lists case-insensitive key substrings that mark a map
// entry or environment variable as secret-bearing regardless of its value.
var sensitiveKeyNeedles = []string{
	"secret", "token", "password", "cookie", "authorization", "auth",
	"credential", "api_key", "apikey",
}

// secretShapePatterns are anchored patterns applied to value strings (and to
// fmt.Sprint of non-string scalars) to detect secret-shaped values whose key
// name carries no signal. The list is the locked posture from SBH-1 D-9:
// explicit shapes over entropy guessing — a newly minted secret format is a
// one-line addition here with a test, not a silent race against a classifier.
var secretShapePatterns = []*regexp.Regexp{
	// JWT: three dot-separated base64url segments whose header begins "eyJ".
	regexp.MustCompile(`^eyJ[A-Za-z0-9_\-]+(\.[A-Za-z0-9_\-]+){2}$`),
	// AWS access key IDs (AKIA…) and session/window tokens (ASIA…).
	regexp.MustCompile(`^(AKIA|ASIA)[A-Z0-9]{16}$`),
	// Slack tokens (xoxb / xoxa / xoxp / xoxr / xoxs).
	regexp.MustCompile(`^xox[baprs]-[A-Za-z0-9\-]+$`),
	// GitHub tokens: classic prefixes, fine-grained PAT-v2 ("github_pat_").
	regexp.MustCompile(`^gh[pousr]_[A-Za-z0-9]+$`),
	regexp.MustCompile(`^github_pat_[A-Za-z0-9_]+$`),
	// OpenAI-style API keys.
	regexp.MustCompile(`^sk-[A-Za-z0-9_\-]+$`),
	// PEM / DER private key blocks.
	regexp.MustCompile(`^-----BEGIN`),
	// URL userinfo carries an embedded credential: scheme://user:pass@host.
	regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://[^/@\s]+:[^@\s]+@`),
}

// sensitiveValuePrefixes are cheap case-insensitive substring checks that run
// before the anchored regexes (fast path) and also catch inline embeddings a
// full-match regex would miss (e.g. "Authorization: Bearer sk-abc" mid-string).
var sensitiveValuePrefixes = []string{
	"bearer ",
	"basic ",
	"ghp_",
	"gho_",
	"ghu_",
	"ghs_",
	"ghr_",
	"github_pat_",
	"sk-",
	"authorization:",
	"session=",
}

// LooksSensitiveShape reports whether a value string carries a secret-shaped
// payload under the SBH-1 D-9 shape table. It is the canonical value-shape
// classifier shared by the capability-facing and governance-facing redaction
// surfaces. Matching is explicitly positive-bias: false positives degrade to
// "[REDACTED]" for a handful of benign shapes; false negatives are the locked
// debt of the shape list and require a one-line table addition with a test.
func LooksSensitiveShape(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range sensitiveValuePrefixes {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	for _, re := range secretShapePatterns {
		if re.MatchString(trimmed) {
			return true
		}
	}
	return false
}

// RedactStrings returns a copy of values with every entry that matches a
// secret value shape replaced by "[REDACTED]". It is the canonical
// shape-aware redaction for string collections (audit args, argv tails).
func RedactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, v := range values {
		if LooksSensitiveShape(v) {
			out[i] = "[REDACTED]"
		} else {
			out[i] = v
		}
	}
	return out
}

// RedactEnvPairs returns a copy of an environment slice (K=V entries) with
// each entry emitted as K=[REDACTED] when the key is sensitive or the value
// matches a secret shape, and verbatim otherwise. Pair-unaware redaction was
// the P-7 leak — an entry like "API_KEY=sk-abc" matches no known shape as a
// whole string; pair-aware redaction classifies the halves independently.
func RedactEnvPairs(env []string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, len(env))
	for i, e := range env {
		key, value, ok := strings.Cut(e, "=")
		if !ok {
			out[i] = e
			continue
		}
		if isSensitiveKey(key) || LooksSensitiveShape(value) {
			out[i] = key + "=[REDACTED]"
		} else {
			out[i] = e
		}
	}
	return out
}

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
		return redactSlice("", typed)
	case []string:
		return redactStringsToAny("", typed)
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
		// []string under a map value: one shared slice walk keeps the
		// recursion in a single place (no duplicated key/anthill walks).
		return redactStringsToAny(key, typed)
	case []any:
		return redactSlice(key, typed)
	case string:
		if LooksSensitiveShape(typed) {
			return "[REDACTED]"
		}
		return typed
	default:
		// Non-string scalars are matched on their printed representation so a
		// numeric or boolean secret cannot slip through by type. Identical
		// scalar passthrough otherwise.
		if LooksSensitiveShape(fmt.Sprint(typed)) {
			return "[REDACTED]"
		}
		return value
	}
}

// redactSlice walks a heterogeneous slice under a shared parent key.
func redactSlice(key string, items []any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, redactValue(key, item))
	}
	return out
}

// redactStringsToAny walks a homogeneous string slice under a shared parent
// key, producing the redacted representation used by both redactAny and the
// map-value path.
func redactStringsToAny(key string, items []string) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, redactValue(key, item))
	}
	return out
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, needle := range sensitiveKeyNeedles {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

// looksSensitiveValue is retained as the private alias over the canonical
// shape classifier for existing callers inside this package.
func looksSensitiveValue(value string) bool {
	return LooksSensitiveShape(value)
}

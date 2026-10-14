package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzRedaction drives the redaction helpers with arbitrary untrusted key and
// value strings. Invariants: redaction is total (never panics), sensitive keys
// always redact their value regardless of the value's content, sensitive
// values are redacted under insensitive keys and inside string slices, and
// non-sensitive scalar strings survive untouched.
func FuzzRedaction(f *testing.F) {
	f.Add("api_key", "sk-abc123")
	f.Add("username", "alex")
	f.Add("Authorization", "")
	f.Add("token", "plain value with no markers")
	f.Add("weird key", "Bearer \x00\xff binary")
	f.Add("note", "session=abc; path=/")

	f.Fuzz(func(t *testing.T, key, value string) {
		sensitiveKey := isSensitiveKey(key)
		sensitiveValue := looksSensitiveValue(value)

		redactedMap := RedactMetadataMap(map[string]any{key: value})
		if sensitiveKey {
			assert.Equal(t, "[REDACTED]", redactedMap[key],
				"sensitive key %q must always redact its value", key)
		} else if sensitiveValue {
			assert.Equal(t, "[REDACTED]", redactedMap[key],
				"sensitive value under key %q must be redacted", key)
		} else {
			assert.Equal(t, value, redactedMap[key],
				"non-sensitive key/value pair %q=%q must survive redaction", key, value)
		}

		// Values inside string slices carry an empty key: only value-based
		// detection applies.
		redactedSlice := RedactAny([]string{value})
		if sensitiveValue {
			assert.Equal(t, []any{"[REDACTED]"}, redactedSlice)
		} else {
			assert.Equal(t, []any{value}, redactedSlice)
		}

		// The string-map dispatch must behave identically to the any-map.
		redactedStringMap, ok := RedactAny(map[string]string{key: value}).(map[string]any)
		require.True(t, ok, "map[string]string input must produce a map[string]any output")
		assert.Equal(t, redactedMap[key], redactedStringMap[key])
	})
}

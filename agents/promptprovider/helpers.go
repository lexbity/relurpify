package promptprovider

import (
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// envelopeGet reads a key from envelope working memory.
func envelopeGet(env *contextdata.Envelope, key string) (any, bool) {
	if env == nil {
		return nil, false
	}
	return env.GetWorkingValue(key)
}

// envelopeGetString reads a key and converts to string.
func envelopeGetString(env *contextdata.Envelope, key string) string {
	raw, ok := envelopeGet(env, key)
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

// truncate caps a string at max bytes with an ellipsis.
func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}

// marshalJSON encodes v to indented JSON, returning "" on error.
func marshalJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// extractStringField extracts field from a map[string]any via JSON round-trip.
// Used when a value is stored as a concrete struct type we can't import.
func extractStringField(v any, field string) string {
	if v == nil {
		return ""
	}
	// Fast path: already a map.
	if m, ok := v.(map[string]any); ok {
		if val, ok := m[field]; ok {
			return strings.TrimSpace(fmt.Sprint(val))
		}
		return ""
	}
	// Round-trip through JSON.
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return ""
	}
	if val, ok := m[field]; ok {
		return strings.TrimSpace(fmt.Sprint(val))
	}
	return ""
}

// toSliceOfAny converts a value to []any via JSON round-trip if needed.
func toSliceOfAny(v any) []any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out []any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

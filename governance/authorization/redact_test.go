package authorization

import (
	"testing"
)

// redactGoldenVectors defines a shared set of (input, expected) pairs that
// both governance/authorization.redactAny and the original capability-level
// redactor must agree on. Add vectors here when adding new sensitive patterns.
var redactGoldenVectors = []struct {
	name     string
	input    any
	expected any
}{
	{
		name:     "nil input",
		input:    nil,
		expected: nil,
	},
	{
		name:     "plain string (safe)",
		input:    "hello world",
		expected: "hello world",
	},
	{
		name:     "int passthrough",
		input:    42,
		expected: 42,
	},
	{
		name:     "float passthrough",
		input:    3.14,
		expected: 3.14,
	},
	{
		name:     "bool passthrough",
		input:    true,
		expected: true,
	},
	{
		name: "map with sensitive key",
		input: map[string]any{
			"token": "my-secret-token",
		},
		expected: map[string]any{
			"token": "[REDACTED]",
		},
	},
	{
		name: "map with sensitive nested key",
		input: map[string]any{
			"nested": map[string]any{
				"api_key": "abc123",
			},
		},
		expected: map[string]any{
			"nested": map[string]any{
				"api_key": "[REDACTED]",
			},
		},
	},
	{
		name: "map with safe values passed through",
		input: map[string]any{
			"name":  "test",
			"count": 100,
		},
		expected: map[string]any{
			"name":  "test",
			"count": 100,
		},
	},
	{
		name: "map with sensitive value (bearer token)",
		input: map[string]any{
			"authorization_header": "Bearer ghp_abc123",
		},
		expected: map[string]any{
			"authorization_header": "[REDACTED]",
		},
	},
	{
		name: "sensitive value patterns",
		input: map[string]any{
			"header":       "bearer xyz",
			"gh_token":     "ghp_abcdef123456",
			"github_token": "github_pat_abc123",
			"openai_key":   "sk-proj-test",
			"auth_str":     "authorization: basic",
			"session":      "session=abc123",
			"safe":         "hello world",
		},
		expected: map[string]any{
			"header":       "[REDACTED]",
			"gh_token":     "[REDACTED]",
			"github_token": "[REDACTED]",
			"openai_key":   "[REDACTED]",
			"auth_str":     "[REDACTED]",
			"session":      "[REDACTED]",
			"safe":         "hello world",
		},
	},
	{
		name: "slice of strings",
		input: []string{
			"safe-item",
			"ghp_secret",
		},
		expected: []any{
			"safe-item",
			"[REDACTED]",
		},
	},
	{
		name: "slice of interfaces",
		input: []any{
			map[string]any{"token": "secret"},
			"safe",
		},
		expected: []any{
			map[string]any{"token": "[REDACTED]"},
			"safe",
		},
	},
	{
		name: "map[string]string",
		input: map[string]string{
			"safe":  "value",
			"token": "should-redact",
		},
		expected: map[string]any{
			"safe":  "value",
			"token": "[REDACTED]",
		},
	},
	{
		name:     "empty map",
		input:    map[string]any{},
		expected: map[string]any{},
	},
	{
		name: "nested maps and slices",
		input: map[string]any{
			"metadata": map[string]any{
				"items": []any{
					map[string]any{"secret": "my-password"},
					"visible",
				},
			},
		},
		expected: map[string]any{
			"metadata": map[string]any{
				"items": []any{
					map[string]any{"secret": "[REDACTED]"},
					"visible",
				},
			},
		},
	},
}

func TestRedactAny_goldenVectors(t *testing.T) {
	for _, tc := range redactGoldenVectors {
		t.Run(tc.name, func(t *testing.T) {
			got := redactAny(tc.input)
			assertRedactEqual(t, tc.expected, got)
		})
	}
}

func TestRedactMetadataMap_goldenVectors(t *testing.T) {
	for _, tc := range redactGoldenVectors {
		m, ok := tc.input.(map[string]any)
		if !ok {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			got := redactMetadataMap(m)
			assertRedactEqual(t, tc.expected, got)
		})
	}
}

func assertRedactEqual(t *testing.T, expected, got any) {
	t.Helper()
	switch exp := expected.(type) {
	case map[string]any:
		gm, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]interface{}, got %T", got)
		}
		if len(exp) != len(gm) {
			t.Fatalf("expected %d entries, got %d", len(exp), len(gm))
		}
		for k, ev := range exp {
			gv, ok := gm[k]
			if !ok {
				t.Errorf("key %q missing in result", k)
				continue
			}
			assertRedactEqual(t, ev, gv)
		}
	case []any:
		gs, ok := got.([]any)
		if !ok {
			t.Fatalf("expected []interface{}, got %T", got)
		}
		if len(exp) != len(gs) {
			t.Fatalf("expected %d items, got %d", len(exp), len(gs))
		}
		for i := range exp {
			assertRedactEqual(t, exp[i], gs[i])
		}
	default:
		if expected != got {
			t.Errorf("expected %v (%T), got %v (%T)", expected, expected, got, got)
		}
	}
}

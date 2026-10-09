package authorization

import (
	"testing"
)

// Package-level sentinel assignments tell gosec G101 these strings are
// intended test data, not leaked credentials.
var (
	_ = "ghp_abcdef123456"
	_ = "github_pat_abc123"
	_ = "sk-proj-test"
	_ = "bearer xyz"
	_ = "authorization: basic"
	_ = "session=abc123"
)

// redactGoldenVectors defines a shared set of (input, expected) pairs that
// both governance/authorization.redactAny and the original capability-level
// redactor must agree on. Add vectors here when adding new sensitive patterns.
var redactGoldenVectors = []struct { //nolint:gochecknoglobals // golden test vectors
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
		input: func() map[string]any {
			return map[string]any{ //nolint:gosec // test fixture intentionally carries a secret-shaped key for redaction coverage
				"header":       "bearer xyz",
				"gh_token":     "ghp_abcdef123456",
				"github_token": "github_pat_abc123",
				"openai_key":   "sk-proj-test",
				"auth_str":     "authorization: basic",
				"session":      "session=abc123",
				"safe":         "hello world",
			}
		}(),
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

// Package-level sentinel assignments keep gosec G101 quiet about the literal
// secret-shaped fixtures below.
var (
	_ = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	_ = "AKIAIOSFODNN7EXAMPLE"
	_ = "ASIAABCDEFGHIJKLMNOP"
	_ = "xoxb-1234567890-abcdef"
	_ = "postgres://user:hunter2@db.example/x"
	_ = "-----BEGIN RSA PRIVATE KEY-----"
	_ = "gho_def456ghi789"
	_ = "github_pat_11ABC123xyz"
)

// shapeTable is the SBH-1 D-9 value-shape matrix. Each positive row must be
// classified secret-shaped; each negative row (harmless values that merely
// look like a secret *name*) must NOT be redacted by value.
var shapeTable = []struct {
	name  string
	value string
	want  bool
}{
	// D-9 value shapes.
	{"jwt", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", true},
	{"jwt with padding-less header", "eyJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJqb2UiLCJpYXQiOjE1MTYyMzkwMjJ9.4STCw-Nw8OSqpfipQa5V8tCTpR9cm7HTKMViE0Lbe1Q", true},
	{"aws access key", "AKIAIOSFODNN7EXAMPLE", true},
	{"aws session token", "ASIAABCDEFGHIJKLMNOP", true},
	{"slack bot token", "xoxb-1234567890-abcdefghijkl", true},
	{"slack app token", "xoxa-1234-5678-9012", true},
	{"github commit token", "ghp_abcdef123456", true},
	{"github oauth token", "gho_def456ghi789", true},
	{"github fine-grained pat", "github_pat_11ABC123xyz", true},
	{"openai sk prefix", "sk-proj-0123456789abcdef", true},
	{"pem private key", "-----BEGIN RSA PRIVATE KEY-----", true},
	{"pem generic key", "-----BEGIN PRIVATE KEY-----", true},
	{"url userinfo", "postgres://user:hunter2@db.example/x", true},
	{"url userinfo http", "http://admin:supersecret@10.0.0.1:8080/api", true},
	{"bearer prefix", "Bearer eyJhbGciOiJIUzI1NiJ9.zz.zz", true},
	{"basic prefix", "Basic dXNlcjpwYXNz", true},
	// Negatives: name-lookalikes that must survive as values.
	{"skylight value", "skylight", false},
	{"tokenize value", "tokenize this text", false},
	{"path value", "/etc/hosts", false},
	{"short word sk", "sky", false},
	{"userinfo without colon", "https://user@example.com/x", false},
}

func TestLooksSensitiveShape_shapeTable(t *testing.T) {
	for _, tc := range shapeTable {
		t.Run(tc.name, func(t *testing.T) {
			if got := LooksSensitiveShape(tc.value); got != tc.want {
				t.Fatalf("LooksSensitiveShape(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestRedactStrings_ShapesRedacted(t *testing.T) {
	got := RedactStrings([]string{
		"curl", "http://example.com", "Authorization: Bearer ghp_abc123", "sk-proj-abc", "plain",
	})
	want := []string{
		"curl", "http://example.com", "[REDACTED]", "[REDACTED]", "plain",
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRedactStrings_NilAndEmpty(t *testing.T) {
	if got := RedactStrings(nil); got != nil {
		t.Fatalf("nil input: got %v, want nil", got)
	}
	if got := RedactStrings([]string{}); got != nil {
		t.Fatalf("empty input: got %v, want nil", got)
	}
}

func TestRedactEnvPairs_PairAwareRedaction(t *testing.T) {
	// P-7: pair-unaware redaction leaks "API_KEY=sk-abc" because the whole
	// "K=V" string matches no known shape. Pair-aware redaction classifies the
	// halves independently.
	env := []string{
		"API_KEY=sk-abc123",
		"TOKEN=eyJhbGciOiJIUzI1NiJ9.x.y",
		"AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE",
		"HOME=/home/lex",
		"PATH=/usr/bin:/bin",
		"MALFORMED_NO_EQUALS",
	}
	got := RedactEnvPairs(env)
	want := []string{
		"API_KEY=[REDACTED]",
		"TOKEN=[REDACTED]",
		"AWS_ACCESS_KEY=[REDACTED]",
		"HOME=/home/lex",
		"PATH=/usr/bin:/bin",
		"MALFORMED_NO_EQUALS",
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRedactEnvPairs_NilAndEmpty(t *testing.T) {
	if got := RedactEnvPairs(nil); got != nil {
		t.Fatalf("nil input: got %v, want nil", got)
	}
	if got := RedactEnvPairs([]string{}); got != nil {
		t.Fatalf("empty input: got %v, want nil", got)
	}
}

func TestRedactEnvPairs_NonSecretShapeValueSurvives(t *testing.T) {
	env := []string{"APP_ENV=skylight", "GREETING=hello world"}
	got := RedactEnvPairs(env)
	want := []string{"APP_ENV=skylight", "GREETING=hello world"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestRedactValue_NonStringScalarShape catches a secret that would otherwise
// slip through by type (SBH-1 D-9: shapes matched on fmt.Sprint of non-string
// scalars).
func TestRedactValue_NonStringScalarShape(t *testing.T) {
	// A JWT-shaped token smuggled as a non-string scalar must still redact.
	got := redactMetadataMap(map[string]any{
		"jwt_scalar": "eyJhbGciOiJIUzI1NiJ9.x.y",
	})
	if got["jwt_scalar"] != "[REDACTED]" {
		t.Fatalf("jwt under a scalar key: %v", got["jwt_scalar"])
	}
	// Plain scalars pass through unchanged.
	kept := redactMetadataMap(map[string]any{"port": 8080, "enabled": true})
	if kept["port"] != 8080 || kept["enabled"] != true {
		t.Fatalf("plain scalars must pass through: %v", kept)
	}
}

package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/userconfig/tools/manifest"
)

var (
	_ = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.zzzz"
	_ = "AKIAIOSFODNN7EXAMPLE"
	_ = "ghp_abcdef123456"
)

func TestRedactArgs_KeyNameRedaction(t *testing.T) {
	args := map[string]any{
		"api_key":  "x",
		"password": "hunter2",
		"path":     "/etc/hosts",
	}
	redacted := RedactArgs(args, nil)
	require.Equal(t, "[REDACTED]", redacted["api_key"])
	require.Equal(t, "[REDACTED]", redacted["password"])
	require.Equal(t, "/etc/hosts", redacted["path"])
}

func TestRedactArgs_DeclaredSensitiveParam(t *testing.T) {
	params := []ToolParameter{
		{Name: "connection_token", Type: manifest.ToolParamString},
	}
	redacted := RedactArgs(map[string]any{"connection_token": "abc123"}, params)
	// The declared parameter name does not trip the substring matcher, so the
	// declaration itself must drive redaction.
	require.Equal(t, "[REDACTED]", redacted["connection_token"])
}

func TestRedactArgs_ValueShapeRedaction(t *testing.T) {
	// SBH-1 D-9: a value under an innocuous key that matches a known secret
	// shape must redact on the value alone.
	args := map[string]any{
		"config": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.zzzz",
		"token":  "AKIAIOSFODNN7EXAMPLE", // key match + shape both fire
		"header": "Authorization: Bearer ghp_abcdef123456",
	}
	redacted := RedactArgs(args, nil)
	require.Equal(t, "[REDACTED]", redacted["config"])
	require.Equal(t, "[REDACTED]", redacted["token"])
	require.Equal(t, "[REDACTED]", redacted["header"])
}

func TestRedactArgs_NonSecretValuesSurvive(t *testing.T) {
	// Name-lookalikes must survive as values (shape list is value-anchored).
	args := map[string]any{
		"tool":    "skylight",
		"topic":   "tokenize",
		"command": "curl https://example.com",
		"port":    8080,
	}
	redacted := RedactArgs(args, nil)
	assert.Equal(t, "skylight", redacted["tool"])
	assert.Equal(t, "tokenize", redacted["topic"])
	assert.Equal(t, "curl https://example.com", redacted["command"])
	assert.Equal(t, 8080, redacted["port"])
}

func TestRedactArgs_OriginalUnmodified(t *testing.T) {
	args := map[string]any{"api_key": "secret-value", "path": "/tmp/x"}
	redacted := RedactArgs(args, nil)
	require.Equal(t, "secret-value", args["api_key"], "RedactArgs must not mutate its input")
	require.Equal(t, "[REDACTED]", redacted["api_key"])
}

func TestRedactStringsAdapter(t *testing.T) {
	got := RedactStrings([]string{"curl", "ghp_abc", "sk-proj-1", "/tmp/x"})
	assert.Equal(t, []string{"curl", "[REDACTED]", "[REDACTED]", "/tmp/x"}, got)
}

func TestRedactEnvPairsAdapter(t *testing.T) {
	got := RedactEnvPairs([]string{"API_KEY=abc", "PATH=/usr/bin", "TOKEN=sk-abc"})
	assert.Equal(t, []string{"API_KEY=[REDACTED]", "PATH=/usr/bin", "TOKEN=[REDACTED]"}, got)
}

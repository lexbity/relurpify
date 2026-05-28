package contracts

import (
	"testing"
)

func TestCoerceStringToString(t *testing.T) {
	result, err := coerceParameterValue(ToolParameter{Type: ToolParamString}, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %v", result)
	}
}

func TestCoerceNumericStringToInteger(t *testing.T) {
	result, err := coerceParameterValue(ToolParameter{Type: ToolParamInteger}, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != int64(42) {
		t.Fatalf("expected int64(42), got %T(%v)", result, result)
	}
}

func TestCoerceNonNumericStringToIntegerFails(t *testing.T) {
	_, err := coerceParameterValue(ToolParameter{Type: ToolParamInteger}, "abc")
	if err == nil {
		t.Fatal("expected error for non-numeric string")
	}
}

func TestCoerceFloat64ToInteger(t *testing.T) {
	result, err := coerceParameterValue(ToolParameter{Type: ToolParamInteger}, float64(7))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != int64(7) {
		t.Fatalf("expected int64(7), got %T(%v)", result, result)
	}
}

func TestCoerceFloat64ToIntegerLossyFails(t *testing.T) {
	_, err := coerceParameterValue(ToolParameter{Type: ToolParamInteger}, float64(7.5))
	if err == nil {
		t.Fatal("expected error for lossy float64->int64 conversion")
	}
}

func TestCoerceTrueStringToBoolean(t *testing.T) {
	result, err := coerceParameterValue(ToolParameter{Type: ToolParamBoolean}, "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != true {
		t.Fatalf("expected true, got %v", result)
	}
}

func TestCoerceTrueStringToBooleanUpper(t *testing.T) {
	result, err := coerceParameterValue(ToolParameter{Type: ToolParamBoolean}, "TRUE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != true {
		t.Fatalf("expected true, got %v", result)
	}
}

func TestCoerceFalseStringToBoolean(t *testing.T) {
	result, err := coerceParameterValue(ToolParameter{Type: ToolParamBoolean}, "false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != false {
		t.Fatalf("expected false, got %v", result)
	}
}

func TestCoerceInvalidStringToBooleanFails(t *testing.T) {
	_, err := coerceParameterValue(ToolParameter{Type: ToolParamBoolean}, "yep")
	if err == nil {
		t.Fatal("expected error for 'yep' (not in supported boolean strings)")
	}
}

func TestValidateArgsRequiredParamNilValueRejected(t *testing.T) {
	manifest := ToolManifest{
		Parameters: []ToolParameter{
			{Name: "path", Type: ToolParamString, Required: true},
		},
	}
	err := ValidateToolArguments(manifest, map[string]any{"path": nil})
	if err == nil {
		t.Fatal("expected error for nil required param")
	}
}

func TestValidateArgsEmptyManifestSkipsValidation(t *testing.T) {
	err := ValidateToolArguments(ToolManifest{}, map[string]any{"whatever": "value"})
	if err != nil {
		t.Fatalf("expected no error for empty manifest, got: %v", err)
	}
}

func TestValidateArgsCoercesTypes(t *testing.T) {
	manifest := ToolManifest{
		Parameters: []ToolParameter{
			{Name: "count", Type: ToolParamInteger, Required: true},
			{Name: "verbose", Type: ToolParamBoolean, Required: false},
		},
	}
	args := map[string]any{"count": "42", "verbose": "true"}
	err := ValidateToolArguments(manifest, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["count"] != int64(42) {
		t.Fatalf("expected count=42, got %T(%v)", args["count"], args["count"])
	}
	if args["verbose"] != true {
		t.Fatalf("expected verbose=true, got %v", args["verbose"])
	}
}

func TestValidateArgsCoerceError(t *testing.T) {
	manifest := ToolManifest{
		Parameters: []ToolParameter{
			{Name: "count", Type: ToolParamInteger, Required: true},
		},
	}
	err := ValidateToolArguments(manifest, map[string]any{"count": "not_a_number"})
	if err == nil {
		t.Fatal("expected coercion error")
	}
}

func TestRedactArgsNoSecrets(t *testing.T) {
	args := map[string]any{"path": "/tmp/test", "pattern": "*.go"}
	redacted := RedactArgs(args, nil)
	if redacted["path"] != "/tmp/test" {
		t.Fatalf("expected path to be unchanged, got %v", redacted["path"])
	}
	if redacted["pattern"] != "*.go" {
		t.Fatalf("expected pattern to be unchanged, got %v", redacted["pattern"])
	}
}

func TestRedactArgsDoesNotMutateOriginal(t *testing.T) {
	original := map[string]any{"api_key": "sk-secret123", "name": "test"}
	_ = RedactArgs(original, nil)
	if original["api_key"] != "sk-secret123" {
		t.Fatal("RedactArgs must not mutate the original map")
	}
}

func TestSecretArgRedactedInLog(t *testing.T) {
	// Simulate what happens when args are redacted before logging:
	// args with names matching secret patterns are replaced.
	args := map[string]any{"api_key": "sk-secret123", "name": "test"}
	redacted := RedactArgs(args, nil)
	if redacted["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key to be redacted, got %v", redacted["api_key"])
	}
	if redacted["name"] != "test" {
		t.Fatalf("expected name unchanged, got %v", redacted["name"])
	}
}

func TestSecretArgRedactedMultiplePatterns(t *testing.T) {
	args := map[string]any{
		"password":   "hunter2",
		"token":      "tok-abc-123",
		"credential": "cred-xyz",
		"user":       "admin",
	}
	redacted := RedactArgs(args, nil)
	for _, k := range []string{"password", "token", "credential"} {
		if redacted[k] != "[REDACTED]" {
			t.Fatalf("expected %s to be redacted, got %v", k, redacted[k])
		}
	}
	if redacted["user"] != "admin" {
		t.Fatalf("expected user to be unchanged, got %v", redacted["user"])
	}
}

func TestNonSecretArgNotRedacted(t *testing.T) {
	args := map[string]any{"query": "hello world", "path": "/etc/config"}
	redacted := RedactArgs(args, nil)
	if redacted["query"] != "hello world" {
		t.Fatalf("expected query to be unchanged, got %v", redacted["query"])
	}
	if redacted["path"] != "/etc/config" {
		t.Fatalf("expected path to be unchanged, got %v", redacted["path"])
	}
}

func TestRedactArgsWithParams(t *testing.T) {
	args := map[string]any{"api_key": "sk-secret123", "token_secret": "tok-abc", "name": "hello"}
	params := []ToolParameter{
		{Name: "api_key"},
		{Name: "name"},
	}
	redacted := RedactArgs(args, params)
	if redacted["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key to be redacted, got %v", redacted["api_key"])
	}
	if redacted["token_secret"] != "[REDACTED]" {
		t.Fatalf("expected token_secret to be redacted, got %v", redacted["token_secret"])
	}
	if redacted["name"] != "hello" {
		t.Fatalf("expected name to be unchanged, got %v", redacted["name"])
	}
}

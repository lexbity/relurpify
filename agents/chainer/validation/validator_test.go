package validation_test

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer/validation"
)

func TestNewJSONValidator(t *testing.T) {
	schema := `{"type": "object"}`
	validator := validation.NewJSONValidator(schema)

	if validator == nil {
		t.Fatal("expected validator")
	}
	if validator.Mode != validation.SchemaModeJSON {
		t.Errorf("expected mode JSON, got %v", validator.Mode)
	}
	if validator.Schema != schema {
		t.Errorf("expected schema %s, got %s", schema, validator.Schema)
	}
}

func TestNewCustomValidator(t *testing.T) {
	called := false
	fn := func(parsed any) error {
		called = true
		return nil
	}

	validator := validation.NewCustomValidator(fn)

	if validator == nil {
		t.Fatal("expected validator")
	}
	if validator.Mode != validation.SchemaModeCustom {
		t.Errorf("expected mode Custom, got %v", validator.Mode)
	}

	// Verify custom function is callable
	err := validator.Custom(nil)
	if err != nil {
		t.Fatalf("custom function failed: %v", err)
	}
	if !called {
		t.Fatal("custom function not called")
	}
}

func TestValidateJSON_ValidObject(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "object"}`)

	parsed := map[string]any{"name": "test", "value": 42}
	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("expected valid object, got error: %v", err)
	}
}

func TestValidateJSON_ValidArray(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "array"}`)

	parsed := []any{"a", "b", "c"}
	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("expected valid array, got error: %v", err)
	}
}

func TestValidateJSON_ValidString(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "string"}`)

	parsed := "hello"
	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("expected valid string, got error: %v", err)
	}
}

func TestValidateJSON_ValidNumber(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "number"}`)

	parsed := 42.5
	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("expected valid number, got error: %v", err)
	}
}

func TestValidateJSON_ValidInteger(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "integer"}`)

	parsed := int64(42)
	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("expected valid integer, got error: %v", err)
	}
}

func TestValidateJSON_ValidBoolean(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "boolean"}`)

	parsed := true
	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("expected valid boolean, got error: %v", err)
	}
}

func TestValidateJSON_TypeMismatch(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "object"}`)

	// Try to validate string as object
	parsed := "not an object"
	err := validator.Validate(parsed)

	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestValidateJSON_NilValue(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "object"}`)

	err := validator.Validate(nil)

	if err == nil {
		t.Fatal("expected error for nil value")
	}
}

func TestValidateJSON_InvalidSchema(t *testing.T) {
	validator := validation.NewJSONValidator(`not valid json`)

	parsed := map[string]any{"key": "value"}
	err := validator.Validate(parsed)

	if err == nil {
		t.Fatal("expected error for invalid schema")
	}
}

func TestValidateJSON_NoSchema(t *testing.T) {
	validator := validation.NewJSONValidator("")

	parsed := map[string]any{"key": "value"}
	err := validator.Validate(parsed)

	// Should pass with no schema
	if err != nil {
		t.Fatalf("expected no error with empty schema, got: %v", err)
	}
}

func TestValidateCustom(t *testing.T) {
	validator := validation.NewCustomValidator(func(parsed any) error {
		str, ok := parsed.(string)
		if !ok {
			return nil // Not a string, pass
		}
		if len(str) < 5 {
			return nil // Too short, fail
		}
		return nil // Valid
	})

	// Valid
	err := validator.Validate("valid string with good length")
	if err != nil {
		t.Fatalf("expected valid output, got: %v", err)
	}
}

func TestValidateCustom_Failure(t *testing.T) {
	validator := validation.NewCustomValidator(func(parsed any) error {
		return nil // Always pass for this test
	})

	err := validator.Validate("anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNilValidator(t *testing.T) {
	var validator *validation.Validator

	err := validator.Validate(map[string]any{"key": "value"})

	if err != nil {
		t.Fatalf("nil validator should not error, got: %v", err)
	}
}

func TestValidateUnknownMode(t *testing.T) {
	validator := &validation.Validator{
		Mode: "unknown",
	}

	err := validator.Validate(map[string]any{})

	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestValidateArrayType(t *testing.T) {
	validator := validation.NewJSONValidator(`{"type": "array"}`)

	// Valid array
	parsed := []any{1, 2, 3}
	err := validator.Validate(parsed)
	if err != nil {
		t.Fatalf("valid array should pass: %v", err)
	}

	// Object should fail
	validator2 := validation.NewJSONValidator(`{"type": "array"}`)
	err = validator2.Validate(map[string]any{"key": "value"})
	if err == nil {
		t.Fatal("object should fail array validation")
	}
}

func TestValidateNestedSchema(t *testing.T) {
	// Nested schema with properties
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		}
	}`

	validator := validation.NewJSONValidator(schema)

	parsed := map[string]any{
		"name": "John",
		"age":  30,
	}

	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("valid nested object should pass: %v", err)
	}
}

func TestValidateComplexArray(t *testing.T) {
	schema := `{"type": "array"}`

	validator := validation.NewJSONValidator(schema)

	parsed := []any{
		map[string]any{"id": 1, "name": "item1"},
		map[string]any{"id": 2, "name": "item2"},
	}

	err := validator.Validate(parsed)

	if err != nil {
		t.Fatalf("complex array should pass: %v", err)
	}
}

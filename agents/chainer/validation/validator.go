package validation

import (
	"encoding/json"
	"fmt"
)

// SchemaMode defines how schema validation is performed.
type SchemaMode string

const (
	// SchemaModeJSON validates output as JSON against a JSON Schema
	SchemaModeJSON SchemaMode = "json"
	// SchemaModeCustom uses a custom validation function
	SchemaModeCustom SchemaMode = "custom"
)

// Validator validates output according to a schema.
type Validator struct {
	Mode   SchemaMode
	Schema string                 // JSON Schema as string
	Custom func(parsed any) error // Custom validation function
}

// Validate checks parsed output against the schema.
//
// For SchemaModeJSON, validates that parsed is a valid JSON structure
// matching the declared schema.
//
// For SchemaModeCustom, calls the custom validation function.
func (v *Validator) Validate(parsed any) error {
	if v == nil {
		return nil // No schema defined
	}

	switch v.Mode {
	case SchemaModeJSON:
		return v.validateJSON(parsed)
	case SchemaModeCustom:
		return v.validateCustom(parsed)
	default:
		return fmt.Errorf("unknown schema mode: %s", v.Mode)
	}
}

// validateJSON checks that parsed value is valid JSON matching declared schema.
func (v *Validator) validateJSON(parsed any) error {
	if parsed == nil {
		return fmt.Errorf("parsed value is nil")
	}

	// For Phase 5, we do lightweight validation:
	// - Check that it's a valid JSON structure (object/array/primitive)
	// - Full JSON Schema validation deferred to Phase 6+

	// Convert parsed value to JSON to verify structure
	jsonBytes, err := json.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("output not valid JSON: %w", err)
	}

	// Verify schema is valid JSON schema (basic check)
	if v.Schema != "" {
		var schemaObj map[string]any
		if err := json.Unmarshal([]byte(v.Schema), &schemaObj); err != nil {
			return fmt.Errorf("invalid JSON schema: %w", err)
		}

		// Phase 5: Basic type checking
		// Check if schema specifies "type" and verify parsed matches
		if typeSpec, hasType := schemaObj["type"]; hasType {
			expectedType := typeSpec.(string)
			if !v.matchesType(parsed, expectedType) {
				return fmt.Errorf("output type mismatch: expected %s, got %T", expectedType, parsed)
			}
		}
	}

	// Ensure decoded JSON is parseable
	var decoded any
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		return fmt.Errorf("output JSON not valid: %w", err)
	}

	return nil
}

// matchesType checks if a value matches the expected JSON type.
func (v *Validator) matchesType(value any, expectedType string) bool {
	switch expectedType {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case float64, int, int64:
			return true
		}
		return false
	case "integer":
		_, ok := value.(int)
		if !ok {
			_, ok = value.(int64)
		}
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

// validateCustom calls the custom validation function.
func (v *Validator) validateCustom(parsed any) error {
	if v.Custom == nil {
		return fmt.Errorf("custom validator not provided")
	}
	return v.Custom(parsed)
}

// NewJSONValidator creates a validator for JSON schema validation.
func NewJSONValidator(schema string) *Validator {
	return &Validator{
		Mode:   SchemaModeJSON,
		Schema: schema,
	}
}

// NewCustomValidator creates a validator with a custom validation function.
func NewCustomValidator(fn func(parsed any) error) *Validator {
	return &Validator{
		Mode:   SchemaModeCustom,
		Custom: fn,
	}
}

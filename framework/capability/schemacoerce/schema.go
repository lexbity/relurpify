package schemacoerce

// Schema represents a JSON Schema used for tool parameters and result shapes.
type Schema struct {
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
	Default     interface{}        `json:"default,omitempty" yaml:"default,omitempty"`
	Enum        []interface{}      `json:"enum,omitempty" yaml:"enum,omitempty"`
	Title       string             `json:"title,omitempty" yaml:"title,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
}

// ValidateAndCoerce validates and coerces a value against a schema.
// It returns the coerced value or an error if validation fails.
func ValidateAndCoerce(schema *Schema, value interface{}) (interface{}, error) {
	if schema == nil {
		return value, nil
	}
	return coerce(schema, value)
}

func coerce(schema *Schema, value interface{}) (interface{}, error) {
	if value == nil {
		if len(schema.Required) > 0 {
			return nil, &ValidationError{Message: "value is required"}
		}
		return schema.Default, nil
	}
	return value, nil
}

// ValidationError describes a schema validation failure.
type ValidationError struct {
	Message string
	Path    string
}

func (e *ValidationError) Error() string {
	if e.Path != "" {
		return e.Path + ": " + e.Message
	}
	return e.Message
}

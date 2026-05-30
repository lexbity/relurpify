package core

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ValidateAndCoerce is the single validate→coerce→reject pipeline for tool and
// capability arguments. It:
//  1. Rejects unknown top-level keys (except executor keys "args", "working_directory", "stdin")
//  2. Injects declared defaults for missing optional parameters
//  3. Coerces values to declared types
//  4. Validates enum/required/nested via the schema
//
// When params is non-empty its ToolParameter type + default is used for coercion
// and default injection. When params is empty the schema's own property types
// are used (capability-descriptor path).
// The args map is mutated in place with coerced values and injected defaults.
func ValidateAndCoerce(args map[string]any, schema *contracts.Schema, params []contracts.ToolParameter) error {
	// Build declared key index from schema properties and ToolParameter list.
	declared := make(map[string]bool)
	if schema != nil {
		for name := range schema.Properties {
			declared[contracts.NormalizeToolName(name)] = true
		}
	}
	for _, param := range params {
		name := contracts.NormalizeToolName(param.Name)
		if name != "" {
			declared[name] = true
		}
	}

	if len(declared) == 0 {
		return nil
	}

	// Step 1: Reject unknown top-level keys (excluding executor keys).
	for key := range args {
		if key == "args" || key == "working_directory" || key == "stdin" {
			continue
		}
		if !declared[contracts.NormalizeToolName(key)] {
			return fmt.Errorf("unknown parameter %q", key)
		}
	}

	// Step 2: Inject defaults and coerce using ToolParameter list.
	if len(params) > 0 {
		for _, param := range params {
			name := contracts.NormalizeToolName(param.Name)
			if name == "" {
				continue
			}
			raw, hasKey := args[name]
			if !hasKey || raw == nil {
				if param.Default != nil {
					args[name] = param.Default
				}
				continue
			}
			coerced, err := coerceParameterValue(param, raw)
			if err != nil {
				return fmt.Errorf("parameter %q: %w", name, err)
			}
			args[name] = coerced
		}
	}

	// Step 3: Schema validation (required, enum, type, nested).
	if schema != nil {
		return validateArgsAgainstSchema(args, schema, "$")
	}
	return nil
}

// validateArgsAgainstSchema validates a flat args map against a schema.
// Unlike validateValueAgainstSchema, this handles the top-level object
// differently (args is always an object) and rejects unknown schema keys.
func validateArgsAgainstSchema(args map[string]any, schema *contracts.Schema, path string) error {
	if schema == nil {
		return nil
	}

	// Only object schemas are valid for top-level args.
	if schema.Type != "" && strings.ToLower(strings.TrimSpace(schema.Type)) != "object" {
		return fmt.Errorf("%s must be object", path)
	}

	// Check required fields.
	for _, key := range schema.Required {
		val, ok := args[key]
		if !ok || val == nil {
			return fmt.Errorf("%s.%s required", path, key)
		}
	}

	// Validate each property against its sub-schema.
	for key, prop := range schema.Properties {
		child, ok := args[key]
		if !ok {
			continue
		}
		if err := validateValueAgainstSchema(child, prop, path+"."+key); err != nil {
			return err
		}
	}
	return nil
}

// ValidateValueAgainstSchema performs lightweight runtime validation for the
// framework-owned schema subset used by tool and capability descriptors.
// Deprecated: use ValidateAndCoerce instead.
func ValidateValueAgainstSchema(value any, schema *contracts.Schema) error {
	if schema == nil {
		return nil
	}
	return validateValueAgainstSchema(value, schema, "$")
}

func validateValueAgainstSchema(value any, schema *contracts.Schema, path string) error {
	if schema == nil {
		return nil
	}
	if len(schema.Enum) > 0 && !schemaEnumContains(schema.Enum, value) {
		return fmt.Errorf("%s must match schema enum", path)
	}
	switch strings.ToLower(strings.TrimSpace(schema.Type)) {
	case "", "any":
		return nil
	case "object":
		obj, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s must be object", path)
		}
		for _, key := range schema.Required {
			if _, ok := obj[key]; !ok {
				return fmt.Errorf("%s.%s required", path, key)
			}
		}
		for key, prop := range schema.Properties {
			child, ok := obj[key]
			if !ok {
				continue
			}
			if err := validateValueAgainstSchema(child, prop, path+"."+key); err != nil {
				return err
			}
		}
		return nil
	case "array":
		items, ok := value.([]interface{})
		if !ok {
			return fmt.Errorf("%s must be array", path)
		}
		for idx, item := range items {
			if err := validateValueAgainstSchema(item, schema.Items, fmt.Sprintf("%s[%d]", path, idx)); err != nil {
				return err
			}
		}
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be string", path)
		}
		return nil
	case "boolean", "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be boolean", path)
		}
		return nil
	case "integer":
		if !isIntegerValue(value) {
			return fmt.Errorf("%s must be integer", path)
		}
		return nil
	case "number":
		if !isNumberValue(value) {
			return fmt.Errorf("%s must be number", path)
		}
		return nil
	default:
		return nil
	}
}

// coerceParameterValue delegates to contracts.CoerceParameterValue (reuses the
// canonical coercion implementation from the platform layer).
func coerceParameterValue(param contracts.ToolParameter, v any) (any, error) {
	return contracts.CoerceParameterValue(param, v)
}

func schemaEnumContains(values []any, candidate any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}

func isIntegerValue(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return math.Mod(float64(typed), 1) == 0
	case float64:
		return math.Mod(typed, 1) == 0
	default:
		return false
	}
}

func isNumberValue(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	default:
		return false
	}
}

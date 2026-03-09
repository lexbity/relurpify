package core

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

// ValidateValueAgainstSchema performs lightweight runtime validation for the
// framework-owned schema subset used by tool and capability descriptors.
func ValidateValueAgainstSchema(value any, schema *Schema) error {
	if schema == nil {
		return nil
	}
	return validateValueAgainstSchema(value, schema, "$")
}

func validateValueAgainstSchema(value any, schema *Schema, path string) error {
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

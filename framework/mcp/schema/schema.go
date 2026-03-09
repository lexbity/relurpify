package schema

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

func FromMap(input map[string]any) (*core.Schema, error) {
	if len(input) == 0 {
		return nil, nil
	}
	value, err := fromAny(input)
	if err != nil {
		return nil, err
	}
	result, ok := value.(*core.Schema)
	if !ok {
		return nil, fmt.Errorf("schema root must decode to object")
	}
	return result, nil
}

func fromAny(input any) (any, error) {
	switch typed := input.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		schema := &core.Schema{}
		if value, ok := typed["type"].(string); ok {
			schema.Type = strings.TrimSpace(value)
		}
		if value, ok := typed["title"].(string); ok {
			schema.Title = value
		}
		if value, ok := typed["description"].(string); ok {
			schema.Description = value
		}
		if value, ok := typed["format"].(string); ok {
			schema.Format = value
		}
		if rawProps, ok := typed["properties"].(map[string]any); ok {
			schema.Properties = make(map[string]*core.Schema, len(rawProps))
			for key, raw := range rawProps {
				child, err := fromAny(raw)
				if err != nil {
					return nil, err
				}
				if childSchema, ok := child.(*core.Schema); ok {
					schema.Properties[key] = childSchema
				}
			}
		}
		if rawItems, ok := typed["items"]; ok {
			items, err := fromAny(rawItems)
			if err != nil {
				return nil, err
			}
			if itemSchema, ok := items.(*core.Schema); ok {
				schema.Items = itemSchema
			}
		}
		if rawRequired, ok := typed["required"].([]any); ok {
			schema.Required = make([]string, 0, len(rawRequired))
			for _, item := range rawRequired {
				if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
					schema.Required = append(schema.Required, value)
				}
			}
		}
		if rawEnum, ok := typed["enum"].([]any); ok {
			schema.Enum = append([]any(nil), rawEnum...)
		}
		if value, ok := typed["default"]; ok {
			schema.Default = value
		}
		return schema, nil
	default:
		return nil, fmt.Errorf("unsupported schema value %T", input)
	}
}

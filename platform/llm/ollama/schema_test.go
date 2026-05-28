package ollama

// Schema conformance tests for the Ollama backend.
//
// These tests document the known lossiness of schema conversion when
// round-tripping from contracts.Tool → LLMToolSpec → Ollama native format.
// They assert that features known to survive the conversion still work,
// and document (without failing on) features that are lost.

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// fullFeaturesTool declares parameters with every supported feature to verify
// which survive the Ollama schema conversion path.
type fullFeaturesTool struct{}

func (f *fullFeaturesTool) Name() string        { return "full_features" }
func (f *fullFeaturesTool) Description() string { return "Tool with all parameter features" }
func (f *fullFeaturesTool) Category() string    { return "test" }
func (f *fullFeaturesTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "str_param", Type: contracts.ToolParamString, Description: "A string", Required: true},
		{Name: "int_param", Type: contracts.ToolParamInteger, Description: "An integer", Required: false, Default: int64(42)},
		{Name: "bool_param", Type: contracts.ToolParamBoolean, Description: "A boolean", Required: false},
	}
}
func (f *fullFeaturesTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	return &contracts.ToolResult{Success: true}, nil
}
func (f *fullFeaturesTool) IsAvailable(ctx context.Context) bool { return true }
func (f *fullFeaturesTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{{Binary: "echo"}},
	}}
}
func (f *fullFeaturesTool) Tags() []string { return nil }

func TestOllamaSchemaTopLevelStringPreserved(t *testing.T) {
	tool := &fullFeaturesTool{}
	spec := contracts.LLMToolSpecFromTool(tool)
	ollamaParams := schemaToOllamaParameters(spec.InputSchema)

	props, ok := ollamaParams["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	strProp, ok := props["str_param"].(map[string]interface{})
	if !ok {
		t.Fatal("expected str_param property")
	}
	if strProp["type"] != "string" {
		t.Fatalf("expected type 'string', got %v", strProp["type"])
	}
	if strProp["description"] != "A string" {
		t.Fatalf("expected description 'A string', got %v", strProp["description"])
	}
}

func TestOllamaSchemaIntegerPreserved(t *testing.T) {
	tool := &fullFeaturesTool{}
	spec := contracts.LLMToolSpecFromTool(tool)
	ollamaParams := schemaToOllamaParameters(spec.InputSchema)

	props, ok := ollamaParams["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	intProp, ok := props["int_param"].(map[string]interface{})
	if !ok {
		t.Fatal("expected int_param property")
	}
	if intProp["type"] != "integer" {
		t.Fatalf("expected type 'integer', got %v", intProp["type"])
	}
	switch v := intProp["default"].(type) {
	case int64:
		if v != 42 {
			t.Fatalf("expected default 42, got %d", v)
		}
	case float64:
		if v != 42 {
			t.Fatalf("expected default 42, got %f", v)
		}
	case int:
		if v != 42 {
			t.Fatalf("expected default 42, got %d", v)
		}
	default:
		t.Fatalf("expected default of numeric type, got %T(%v)", intProp["default"], intProp["default"])
	}
}

func TestOllamaSchemaBooleanPreserved(t *testing.T) {
	tool := &fullFeaturesTool{}
	spec := contracts.LLMToolSpecFromTool(tool)
	ollamaParams := schemaToOllamaParameters(spec.InputSchema)

	props, ok := ollamaParams["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}
	boolProp, ok := props["bool_param"].(map[string]interface{})
	if !ok {
		t.Fatal("expected bool_param property")
	}
	if boolProp["type"] != "boolean" {
		t.Fatalf("expected type 'boolean', got %v", boolProp["type"])
	}
}

func TestOllamaSchemaRequiredPreserved(t *testing.T) {
	tool := &fullFeaturesTool{}
	spec := contracts.LLMToolSpecFromTool(tool)
	ollamaParams := schemaToOllamaParameters(spec.InputSchema)

	required, ok := ollamaParams["required"].([]string)
	if !ok {
		t.Fatal("expected required array")
	}
	if len(required) != 1 || required[0] != "str_param" {
		t.Fatalf("expected required=['str_param'], got %v", required)
	}
}

func TestOllamaSchemaNestedObjectDocumentedLoss(t *testing.T) {
	t.Log("KNOWN LOSS: Nested object schemas are NOT converted through the")
	t.Log("ToolParameter system because ToolParameter.Type is a flat type.")
	t.Log("This test documents the limitation — nested objects require")
	t.Log("the contracts.Schema path, not the ToolParameter path.")
}

package mapping

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	"github.com/stretchr/testify/require"
)

func TestExportedTool(t *testing.T) {
	t.Run("basic tool conversion", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			ID:          "tool:test",
			Name:        "test.tool",
			Description: "A test tool",
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"input": {Type: "string"},
				},
			},
			OutputSchema: &core.Schema{
				Type: "string",
			},
		}
		tool := ExportedTool(desc)
		require.Equal(t, "test.tool", tool.Name)
		require.Equal(t, "test.tool", tool.Title)
		require.Equal(t, "A test tool", tool.Description)
		require.NotNil(t, tool.InputSchema)
		require.Equal(t, "object", tool.InputSchema["type"])
		require.NotNil(t, tool.OutputSchema)
		require.Equal(t, "string", tool.OutputSchema["type"])
	})

	t.Run("empty schema", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Name: "simple.tool",
		}
		tool := ExportedTool(desc)
		require.Equal(t, "simple.tool", tool.Name)
		require.Nil(t, tool.InputSchema)
		require.Nil(t, tool.OutputSchema)
	})
}

func TestExportedPrompt(t *testing.T) {
	t.Run("prompt with arguments", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			ID:          "prompt:test",
			Name:        "test.prompt",
			Description: "A test prompt",
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"name": {Type: "string", Description: "The name"},
					"age":  {Type: "number", Description: "The age"},
				},
				Required: []string{"name"},
			},
		}
		prompt := ExportedPrompt(desc)
		require.Equal(t, "test.prompt", prompt.Name)
		require.Equal(t, "A test prompt", prompt.Description)
		require.Len(t, prompt.Arguments, 2)

		// Find name argument and check it's required
		var nameArg *protocol.PromptArgument
		for i := range prompt.Arguments {
			if prompt.Arguments[i].Name == "name" {
				nameArg = &prompt.Arguments[i]
				break
			}
		}
		require.NotNil(t, nameArg)
		require.True(t, nameArg.Required)
		require.Equal(t, "The name", nameArg.Description)
	})

	t.Run("empty input schema", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Name: "simple.prompt",
		}
		prompt := ExportedPrompt(desc)
		require.Equal(t, "simple.prompt", prompt.Name)
		require.Empty(t, prompt.Arguments)
	})

	t.Run("non-object input schema", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Name:        "test.prompt",
			InputSchema: &core.Schema{Type: "string"},
		}
		prompt := ExportedPrompt(desc)
		require.Empty(t, prompt.Arguments)
	})

	t.Run("empty property name skipped", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Name: "test.prompt",
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"":     {Type: "string"},
					"name": {Type: "string"},
				},
			},
		}
		prompt := ExportedPrompt(desc)
		require.Len(t, prompt.Arguments, 1)
		require.Equal(t, "name", prompt.Arguments[0].Name)
	})

	t.Run("case insensitive required check", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Name: "test.prompt",
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"Name": {Type: "string"},
				},
				Required: []string{"NAME"},
			},
		}
		prompt := ExportedPrompt(desc)
		require.Len(t, prompt.Arguments, 1)
		require.True(t, prompt.Arguments[0].Required)
	})
}

func TestResourceResultFromAnnotation(t *testing.T) {
	t.Run("resource with text", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Annotations: map[string]any{
				"resource_text": "Hello, World!",
				"mime_type":     "text/plain",
			},
		}
		result, ok := ResourceResultFromAnnotation(desc)
		require.True(t, ok)
		require.Len(t, result.Contents, 1)
		require.Equal(t, "text", result.Contents[0].Type)
		require.Equal(t, "Hello, World!", result.Contents[0].Text)
		require.Equal(t, "text/plain", result.Contents[0].MIMEType)
	})

	t.Run("nil annotations", func(t *testing.T) {
		desc := core.CapabilityDescriptor{}
		result, ok := ResourceResultFromAnnotation(desc)
		require.False(t, ok)
		require.Nil(t, result)
	})

	t.Run("missing resource_text", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Annotations: map[string]any{
				"mime_type": "text/plain",
			},
		}
		result, ok := ResourceResultFromAnnotation(desc)
		require.False(t, ok)
		require.Nil(t, result)
	})

	t.Run("empty resource_text", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Annotations: map[string]any{
				"resource_text": "   ",
			},
		}
		result, ok := ResourceResultFromAnnotation(desc)
		require.False(t, ok)
		require.Nil(t, result)
	})

	t.Run("resource without mime type", func(t *testing.T) {
		desc := core.CapabilityDescriptor{
			Annotations: map[string]any{
				"resource_text": "Hello",
			},
		}
		result, ok := ResourceResultFromAnnotation(desc)
		require.True(t, ok)
		require.Empty(t, result.Contents[0].MIMEType)
	})
}

func TestSchemaToMap(t *testing.T) {
	t.Run("nil schema", func(t *testing.T) {
		m := schemaToMap(nil)
		require.Nil(t, m)
	})

	t.Run("empty schema", func(t *testing.T) {
		m := schemaToMap(&core.Schema{})
		require.Empty(t, m)
	})

	t.Run("basic fields", func(t *testing.T) {
		schema := &core.Schema{
			Type:        "string",
			Title:       "Test Field",
			Description: "A test field",
			Format:      "email",
		}
		m := schemaToMap(schema)
		require.Equal(t, "string", m["type"])
		require.Equal(t, "Test Field", m["title"])
		require.Equal(t, "A test field", m["description"])
		require.Equal(t, "email", m["format"])
	})

	t.Run("nested properties", func(t *testing.T) {
		schema := &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"name": {
					Type:        "string",
					Description: "The name",
				},
				"address": {
					Type: "object",
					Properties: map[string]*core.Schema{
						"street": {Type: "string"},
						"city":   {Type: "string"},
					},
				},
			},
		}
		m := schemaToMap(schema)
		props := m["properties"].(map[string]any)
		require.Len(t, props, 2)
		require.Equal(t, "string", props["name"].(map[string]any)["type"])

		addressProps := props["address"].(map[string]any)["properties"].(map[string]any)
		require.Len(t, addressProps, 2)
	})

	t.Run("array items", func(t *testing.T) {
		schema := &core.Schema{
			Type: "array",
			Items: &core.Schema{
				Type: "string",
			},
		}
		m := schemaToMap(schema)
		require.Equal(t, "array", m["type"])
		require.NotNil(t, m["items"])
		require.Equal(t, "string", m["items"].(map[string]any)["type"])
	})

	t.Run("required and enum", func(t *testing.T) {
		schema := &core.Schema{
			Type:     "string",
			Required: []string{"field1", "field2"},
			Enum:     []any{"a", "b", "c"},
		}
		m := schemaToMap(schema)
		required := m["required"].([]any)
		require.Len(t, required, 2)
		require.Contains(t, required, "field1")
		require.Contains(t, required, "field2")

		enum := m["enum"].([]any)
		require.Len(t, enum, 3)
	})

	t.Run("default value", func(t *testing.T) {
		schema := &core.Schema{
			Type:    "string",
			Default: "default_value",
		}
		m := schemaToMap(schema)
		require.Equal(t, "default_value", m["default"])
	})
}

func TestContentBlocksFromCore(t *testing.T) {
	t.Run("text block", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.TextContentBlock{Text: "Hello"},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 1)
		require.Equal(t, "text", result[0].Type)
		require.Equal(t, "Hello", result[0].Text)
	})

	t.Run("structured block with map", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.StructuredContentBlock{Data: map[string]any{"key": "value"}},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 1)
		require.Equal(t, "structured", result[0].Type)
		require.Equal(t, map[string]any{"key": "value"}, result[0].Data)
	})

	t.Run("structured block with non-map", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.StructuredContentBlock{Data: "string data"},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 1)
		require.Equal(t, "text", result[0].Type)
		require.Equal(t, "string data", result[0].Text)
	})

	t.Run("resource link block", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.ResourceLinkContentBlock{
				URI:      "file:///test.txt",
				Name:     "test.txt",
				MIMEType: "text/plain",
			},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 1)
		require.Equal(t, "resource", result[0].Type)
		require.Equal(t, "file:///test.txt", result[0].URI)
		require.Equal(t, "test.txt", result[0].Name)
		require.Equal(t, "text/plain", result[0].MIMEType)
	})

	t.Run("embedded resource block", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.EmbeddedResourceContentBlock{Resource: "resource-id"},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 1)
		require.Equal(t, "resource", result[0].Type)
		require.Equal(t, "resource-id", result[0].URI)
	})

	t.Run("binary reference block", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.BinaryReferenceContentBlock{
				Ref:      "blob-ref-1",
				MIMEType: "application/octet-stream",
			},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 1)
		require.Equal(t, "blob", result[0].Type)
		require.Equal(t, "blob-ref-1", result[0].Blob)
		require.Equal(t, "application/octet-stream", result[0].MIMEType)
	})

	t.Run("error block", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.ErrorContentBlock{Message: "Something went wrong"},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 1)
		require.Equal(t, "text", result[0].Type)
		require.Equal(t, "Something went wrong", result[0].Text)
	})

	t.Run("empty blocks", func(t *testing.T) {
		result := contentBlocksFromCore([]core.ContentBlock{})
		require.Nil(t, result)
	})

	t.Run("nil blocks", func(t *testing.T) {
		result := contentBlocksFromCore(nil)
		require.Nil(t, result)
	})

	t.Run("multiple blocks", func(t *testing.T) {
		blocks := []core.ContentBlock{
			core.TextContentBlock{Text: "First"},
			core.TextContentBlock{Text: "Second"},
		}
		result := contentBlocksFromCore(blocks)
		require.Len(t, result, 2)
	})
}

func TestCoreContentBlocksFromProtocol(t *testing.T) {
	t.Run("text block", func(t *testing.T) {
		blocks := []protocol.ContentBlock{
			{Type: "text", Text: "Hello"},
		}
		result := CoreContentBlocksFromProtocol(blocks)
		require.Len(t, result, 1)
		require.IsType(t, core.TextContentBlock{}, result[0])
		require.Equal(t, "Hello", result[0].(core.TextContentBlock).Text)
	})

	t.Run("structured block", func(t *testing.T) {
		blocks := []protocol.ContentBlock{
			{Type: "structured", Data: map[string]any{"key": "value"}},
		}
		result := CoreContentBlocksFromProtocol(blocks)
		require.Len(t, result, 1)
		require.IsType(t, core.StructuredContentBlock{}, result[0])
		require.Equal(t, map[string]any{"key": "value"}, result[0].(core.StructuredContentBlock).Data)
	})

	t.Run("resource block", func(t *testing.T) {
		blocks := []protocol.ContentBlock{
			{Type: "resource", URI: "file:///test.txt", Name: "test.txt", MIMEType: "text/plain"},
		}
		result := CoreContentBlocksFromProtocol(blocks)
		require.Len(t, result, 1)
		require.IsType(t, core.ResourceLinkContentBlock{}, result[0])
		link := result[0].(core.ResourceLinkContentBlock)
		require.Equal(t, "file:///test.txt", link.URI)
		require.Equal(t, "test.txt", link.Name)
		require.Equal(t, "text/plain", link.MIMEType)
	})

	t.Run("blob block", func(t *testing.T) {
		blocks := []protocol.ContentBlock{
			{Type: "blob", Blob: "blob-1", MIMEType: "application/octet-stream"},
		}
		result := CoreContentBlocksFromProtocol(blocks)
		require.Len(t, result, 1)
		require.IsType(t, core.BinaryReferenceContentBlock{}, result[0])
		blob := result[0].(core.BinaryReferenceContentBlock)
		require.Equal(t, "blob-1", blob.Ref)
		require.Equal(t, "application/octet-stream", blob.MIMEType)
	})

	t.Run("default case with text", func(t *testing.T) {
		blocks := []protocol.ContentBlock{
			{Type: "unknown", Text: "fallback text"},
		}
		result := CoreContentBlocksFromProtocol(blocks)
		require.Len(t, result, 1)
		require.IsType(t, core.TextContentBlock{}, result[0])
		require.Equal(t, "fallback text", result[0].(core.TextContentBlock).Text)
	})

	t.Run("default case empty text", func(t *testing.T) {
		blocks := []protocol.ContentBlock{
			{Type: "unknown", Text: "   "},
		}
		result := CoreContentBlocksFromProtocol(blocks)
		require.Empty(t, result)
	})

	t.Run("empty blocks", func(t *testing.T) {
		result := CoreContentBlocksFromProtocol([]protocol.ContentBlock{})
		require.Nil(t, result)
	})

	t.Run("nil blocks", func(t *testing.T) {
		result := CoreContentBlocksFromProtocol(nil)
		require.Nil(t, result)
	})
}

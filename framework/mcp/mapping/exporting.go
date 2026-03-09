package mapping

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/mcp/protocol"
)

func ExportedTool(desc core.CapabilityDescriptor) protocol.Tool {
	return protocol.Tool{
		Name:         desc.Name,
		Title:        desc.Name,
		Description:  desc.Description,
		InputSchema:  schemaToMap(desc.InputSchema),
		OutputSchema: schemaToMap(desc.OutputSchema),
	}
}

func ExportedPrompt(desc core.CapabilityDescriptor) protocol.Prompt {
	prompt := protocol.Prompt{
		Name:        desc.Name,
		Description: desc.Description,
	}
	if desc.InputSchema != nil && strings.EqualFold(desc.InputSchema.Type, "object") {
		for name, prop := range desc.InputSchema.Properties {
			if strings.TrimSpace(name) == "" {
				continue
			}
			promptArg := protocol.PromptArgument{
				Name:        name,
				Description: prop.Description,
			}
			for _, required := range desc.InputSchema.Required {
				if strings.EqualFold(required, name) {
					promptArg.Required = true
					break
				}
			}
			prompt.Arguments = append(prompt.Arguments, promptArg)
		}
	}
	return prompt
}

func ExportedResource(desc core.CapabilityDescriptor) protocol.Resource {
	uri := exportedResourceURI(desc)
	name := desc.Name
	if strings.TrimSpace(name) == "" {
		name = desc.ID
	}
	resource := protocol.Resource{
		URI:         uri,
		Name:        name,
		Description: desc.Description,
	}
	if desc.Annotations != nil {
		if mimeType, ok := desc.Annotations["mime_type"].(string); ok {
			resource.MIMEType = mimeType
		}
	}
	return resource
}

func PromptResultFromAnnotation(desc core.CapabilityDescriptor, args map[string]any) (*protocol.GetPromptResult, bool) {
	if desc.Annotations == nil {
		return nil, false
	}
	template, ok := desc.Annotations["prompt_text"].(string)
	if !ok || strings.TrimSpace(template) == "" {
		return nil, false
	}
	content := template
	for key, value := range args {
		content = strings.ReplaceAll(content, "{"+key+"}", fmt.Sprint(value))
	}
	return &protocol.GetPromptResult{
		Description: desc.Description,
		Messages: []protocol.ContentBlock{{
			Type: "text",
			Text: content,
		}},
	}, true
}

func ResourceResultFromAnnotation(desc core.CapabilityDescriptor) (*protocol.ReadResourceResult, bool) {
	if desc.Annotations == nil {
		return nil, false
	}
	text, ok := desc.Annotations["resource_text"].(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, false
	}
	block := protocol.ContentBlock{
		Type: "text",
		Text: text,
	}
	if mimeType, ok := desc.Annotations["mime_type"].(string); ok {
		block.MIMEType = mimeType
	}
	return &protocol.ReadResourceResult{
		Contents: []protocol.ContentBlock{block},
	}, true
}

func PromptResultFromCore(result *core.PromptRenderResult) *protocol.GetPromptResult {
	if result == nil {
		return &protocol.GetPromptResult{}
	}
	out := &protocol.GetPromptResult{
		Description: result.Description,
	}
	for _, message := range result.Messages {
		out.Messages = append(out.Messages, contentBlocksFromCore(message.Content)...)
	}
	return out
}

func ResourceResultFromCore(result *core.ResourceReadResult) *protocol.ReadResourceResult {
	if result == nil {
		return &protocol.ReadResourceResult{}
	}
	return &protocol.ReadResourceResult{
		Contents: contentBlocksFromCore(result.Contents),
	}
}

func exportedResourceURI(desc core.CapabilityDescriptor) string {
	if desc.Annotations != nil {
		if uri, ok := desc.Annotations["mcp_uri"].(string); ok && strings.TrimSpace(uri) != "" {
			return uri
		}
	}
	return "relurpify://capability/" + strings.ReplaceAll(desc.ID, ":", "/")
}

func schemaToMap(schema *core.Schema) map[string]any {
	if schema == nil {
		return nil
	}
	out := map[string]any{}
	if schema.Type != "" {
		out["type"] = schema.Type
	}
	if schema.Title != "" {
		out["title"] = schema.Title
	}
	if schema.Description != "" {
		out["description"] = schema.Description
	}
	if schema.Format != "" {
		out["format"] = schema.Format
	}
	if len(schema.Required) > 0 {
		required := make([]any, 0, len(schema.Required))
		for _, item := range schema.Required {
			required = append(required, item)
		}
		out["required"] = required
	}
	if len(schema.Enum) > 0 {
		out["enum"] = append([]any(nil), schema.Enum...)
	}
	if schema.Default != nil {
		out["default"] = schema.Default
	}
	if schema.Items != nil {
		out["items"] = schemaToMap(schema.Items)
	}
	if len(schema.Properties) > 0 {
		props := make(map[string]any, len(schema.Properties))
		for key, prop := range schema.Properties {
			props[key] = schemaToMap(prop)
		}
		out["properties"] = props
	}
	return out
}

func contentBlocksFromCore(blocks []core.ContentBlock) []protocol.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]protocol.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case core.TextContentBlock:
			out = append(out, protocol.ContentBlock{Type: "text", Text: typed.Text})
		case core.StructuredContentBlock:
			if data, ok := typed.Data.(map[string]any); ok {
				out = append(out, protocol.ContentBlock{Type: "structured", Data: data})
				continue
			}
			out = append(out, protocol.ContentBlock{Type: "text", Text: fmt.Sprint(typed.Data)})
		case core.ResourceLinkContentBlock:
			out = append(out, protocol.ContentBlock{
				Type:     "resource",
				URI:      typed.URI,
				Name:     typed.Name,
				MIMEType: typed.MIMEType,
			})
		case core.EmbeddedResourceContentBlock:
			out = append(out, protocol.ContentBlock{Type: "resource", URI: fmt.Sprint(typed.Resource)})
		case core.BinaryReferenceContentBlock:
			out = append(out, protocol.ContentBlock{Type: "blob", Blob: typed.Ref, MIMEType: typed.MIMEType})
		case core.ErrorContentBlock:
			out = append(out, protocol.ContentBlock{Type: "text", Text: typed.Message})
		default:
			out = append(out, protocol.ContentBlock{Type: "text", Text: fmt.Sprint(typed)})
		}
	}
	return out
}

func CoreContentBlocksFromProtocol(blocks []protocol.ContentBlock) []core.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]core.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text":
			out = append(out, core.TextContentBlock{Text: block.Text})
		case "structured":
			out = append(out, core.StructuredContentBlock{Data: block.Data})
		case "resource":
			out = append(out, core.ResourceLinkContentBlock{URI: block.URI, Name: block.Name, MIMEType: block.MIMEType})
		case "blob":
			out = append(out, core.BinaryReferenceContentBlock{Ref: block.Blob, MIMEType: block.MIMEType})
		default:
			if strings.TrimSpace(block.Text) != "" {
				out = append(out, core.TextContentBlock{Text: block.Text})
			}
		}
	}
	return out
}

package mapping

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	mschema "codeburg.org/lexbit/relurpify/relurpnet/mcp/schema"
)

func ImportedToolDescriptor(providerID, sessionID, negotiatedVersion string, tool protocol.Tool, trust core.TrustClass) (core.CapabilityDescriptor, error) {
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		return core.CapabilityDescriptor{}, fmt.Errorf("remote tool name required")
	}
	inputSchema, err := mschema.FromMap(tool.InputSchema)
	if err != nil {
		return core.CapabilityDescriptor{}, err
	}
	outputSchema, err := mschema.FromMap(tool.OutputSchema)
	if err != nil {
		return core.CapabilityDescriptor{}, err
	}
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            "mcp:" + providerID + ":tool:" + name,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Name:          name,
		Version:       strings.TrimSpace(negotiatedVersion),
		Description:   firstNonEmpty(tool.Description, tool.Title),
		Category:      "mcp",
		Tags:          []string{"mcp", "remote", "tool"},
		Source: core.CapabilitySource{
			ProviderID: providerID,
			Scope:      core.CapabilityScopeRemote,
			SessionID:  sessionID,
		},
		TrustClass:      trust,
		SessionAffinity: sessionID,
		InputSchema:     inputSchema,
		OutputSchema:    outputSchema,
		Availability:    core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"mcp_kind":         "tool",
			"mcp_remote_name":  name,
			"protocol_version": negotiatedVersion,
		},
	}), nil
}

func ImportedPromptDescriptor(providerID, sessionID, negotiatedVersion string, prompt protocol.Prompt, trust core.TrustClass) core.CapabilityDescriptor {
	name := strings.TrimSpace(prompt.Name)
	schema := &core.Schema{Type: "object"}
	if len(prompt.Arguments) > 0 {
		schema.Properties = make(map[string]*core.Schema, len(prompt.Arguments))
		for _, arg := range prompt.Arguments {
			if strings.TrimSpace(arg.Name) == "" {
				continue
			}
			schema.Properties[arg.Name] = &core.Schema{
				Type:        "string",
				Description: arg.Description,
			}
			if arg.Required {
				schema.Required = append(schema.Required, arg.Name)
			}
		}
	}
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            "mcp:" + providerID + ":prompt:" + name,
		Kind:          core.CapabilityKindPrompt,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Name:          name,
		Version:       strings.TrimSpace(negotiatedVersion),
		Description:   prompt.Description,
		Category:      "mcp",
		Tags:          []string{"mcp", "remote", "prompt"},
		Source: core.CapabilitySource{
			ProviderID: providerID,
			Scope:      core.CapabilityScopeRemote,
			SessionID:  sessionID,
		},
		TrustClass:      trust,
		SessionAffinity: sessionID,
		InputSchema:     schema,
		Availability:    core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"mcp_kind":         "prompt",
			"mcp_remote_name":  name,
			"protocol_version": negotiatedVersion,
		},
	})
}

func ImportedResourceDescriptor(providerID, sessionID, negotiatedVersion string, resource protocol.Resource, trust core.TrustClass) core.CapabilityDescriptor {
	uri := strings.TrimSpace(resource.URI)
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            "mcp:" + providerID + ":resource:" + sanitizeResourceID(uri),
		Kind:          core.CapabilityKindResource,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Name:          firstNonEmpty(resource.Name, uri),
		Version:       strings.TrimSpace(negotiatedVersion),
		Description:   resource.Description,
		Category:      "mcp",
		Tags:          []string{"mcp", "remote", "resource"},
		Source: core.CapabilitySource{
			ProviderID: providerID,
			Scope:      core.CapabilityScopeRemote,
			SessionID:  sessionID,
		},
		TrustClass:      trust,
		SessionAffinity: sessionID,
		Availability:    core.AvailabilitySpec{Available: true},
		Annotations: map[string]any{
			"mcp_kind":         "resource",
			"mcp_remote_uri":   uri,
			"mime_type":        resource.MIMEType,
			"protocol_version": negotiatedVersion,
		},
	})
}

func sanitizeResourceID(uri string) string {
	uri = strings.TrimSpace(uri)
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", "?", "_", "#", "_", "&", "_", "=", "_", ".", "_")
	if uri == "" {
		return "unnamed"
	}
	return replacer.Replace(uri)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

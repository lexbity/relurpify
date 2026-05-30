package mapping

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	mschema "codeburg.org/lexbit/relurpify/relurpnet/mcp/schema"
)

// mcpMaxDescriptionLen is the maximum length of a tool description imported
// from an MCP server. Longer descriptions are truncated at a UTF-8 boundary.
const mcpMaxDescriptionLen = 512

// mcpMaxSchemaDepth is the maximum nesting depth allowed for MCP tool
// schemas. Deeper schemas are rejected to prevent ReDoS-style attacks.
const mcpMaxSchemaDepth = 8

// mcpPromptInjectionMarkers are substrings that indicate a description may
// contain instructions to override the LLM's role or behavior. Descriptions
// containing these markers are replaced with a safe placeholder.
var mcpPromptInjectionMarkers = []string{
	"[INST]",
	"[/INST]",
	"[SYSTEM]",
	"[/SYSTEM]",
	"<<SYS>>",
	"<|im_start|>",
	"<|im_end|>",
	"<|system|>",
	"<|user|>",
	"<|assistant|>",
}

// sanitizeMCPDescription sanitizes a tool description from an MCP server.
// It strips markdown fenced code blocks, truncates at mcpMaxDescriptionLen,
// normalises whitespace, and replaces descriptions containing prompt-injection
// marker substrings with a safe placeholder.
func sanitizeMCPDescription(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// Check for prompt-injection markers first.
	for _, marker := range mcpPromptInjectionMarkers {
		if strings.Contains(s, marker) {
			return "[description sanitized]"
		}
	}
	// Strip markdown fenced code blocks (triple backtick).
	s = stripMarkdownFences(s)
	// Normalise interior whitespace runs to single space.
	s = normalizeMCPWhitespace(s)
	// Truncate at byte limit (safe at UTF-8 boundary via AppendIfValid).
	s = truncateUTF8(s, mcpMaxDescriptionLen)
	return strings.TrimSpace(s)
}

func stripMarkdownFences(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lines := strings.Split(s, "\n")
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

func normalizeMCPWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' {
			r = ' '
		}
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Find the last valid UTF-8 rune start before maxBytes.
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

// validateMCPSchemaDepth checks that a schema's nesting depth does not exceed
// maxDepth. This prevents deeply nested schemas from causing expensive
// recursion during schema processing.
func validateMCPSchemaDepth(schema *contracts.Schema, current, maxDepth int) error {
	if schema == nil {
		return nil
	}
	if current > maxDepth {
		return fmt.Errorf("schema depth exceeds maximum (%d)", maxDepth)
	}
	if schema.Items != nil {
		if err := validateMCPSchemaDepth(schema.Items, current+1, maxDepth); err != nil {
			return err
		}
	}
	for _, prop := range schema.Properties {
		if err := validateMCPSchemaDepth(prop, current+1, maxDepth); err != nil {
			return err
		}
	}
	return nil
}

// mcpInvalidNameChars contains characters that are not allowed in MCP tool
// names. These characters could be used for path traversal or injection.
const mcpInvalidNameChars = "/\\:*?\"<>|"

// validateMCPToolName checks that a tool name is safe for use as a capability
// identifier and rejects names containing path separators, glob metacharacters,
// or null bytes.
func validateMCPToolName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("remote tool name required")
	}
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("tool name contains null byte")
	}
	if strings.ContainsAny(name, mcpInvalidNameChars) {
		return fmt.Errorf("tool name contains invalid character: %q", name)
	}
	return nil
}

func ImportedToolDescriptor(providerID, sessionID, negotiatedVersion string, tool protocol.Tool, trust agentspec.TrustClass) (core.CapabilityDescriptor, error) {
	if err := validateMCPToolName(tool.Name); err != nil {
		return core.CapabilityDescriptor{}, err
	}
	name := strings.TrimSpace(tool.Name)
	inputSchema, err := mschema.FromMap(tool.InputSchema)
	if err != nil {
		return core.CapabilityDescriptor{}, err
	}
	if err := validateMCPSchemaDepth(inputSchema, 1, mcpMaxSchemaDepth); err != nil {
		return core.CapabilityDescriptor{}, err
	}
	outputSchema, err := mschema.FromMap(tool.OutputSchema)
	if err != nil {
		return core.CapabilityDescriptor{}, err
	}
	if err := validateMCPSchemaDepth(outputSchema, 1, mcpMaxSchemaDepth); err != nil {
		return core.CapabilityDescriptor{}, err
	}
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            "mcp:" + providerID + ":tool:" + name,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Name:          name,
		Version:       strings.TrimSpace(negotiatedVersion),
		Description:   sanitizeMCPDescription(firstNonEmpty(tool.Description, tool.Title)),
		Category:      "mcp",
		Tags:          []string{"mcp", "remote", "tool"},
		Source: core.CapabilitySource{
			ProviderID: providerID,
			Scope:      agentspec.CapabilityScopeRemote,
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

func ImportedPromptDescriptor(providerID, sessionID, negotiatedVersion string, prompt protocol.Prompt, trust agentspec.TrustClass) core.CapabilityDescriptor {
	name := strings.TrimSpace(prompt.Name)
	schema := &contracts.Schema{Type: "object"}
	if len(prompt.Arguments) > 0 {
		schema.Properties = make(map[string]*contracts.Schema, len(prompt.Arguments))
		for _, arg := range prompt.Arguments {
			if strings.TrimSpace(arg.Name) == "" {
				continue
			}
			schema.Properties[arg.Name] = &contracts.Schema{
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
		Kind:          agentspec.CapabilityKindPrompt,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Name:          name,
		Version:       strings.TrimSpace(negotiatedVersion),
		Description:   sanitizeMCPDescription(prompt.Description),
		Category:      "mcp",
		Tags:          []string{"mcp", "remote", "prompt"},
		Source: core.CapabilitySource{
			ProviderID: providerID,
			Scope:      agentspec.CapabilityScopeRemote,
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

func ImportedResourceDescriptor(providerID, sessionID, negotiatedVersion string, resource protocol.Resource, trust agentspec.TrustClass) core.CapabilityDescriptor {
	uri := strings.TrimSpace(resource.URI)
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            "mcp:" + providerID + ":resource:" + sanitizeResourceID(uri),
		Kind:          agentspec.CapabilityKindResource,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Name:          firstNonEmpty(resource.Name, uri),
		Version:       strings.TrimSpace(negotiatedVersion),
		Description:   sanitizeMCPDescription(resource.Description),
		Category:      "mcp",
		Tags:          []string{"mcp", "remote", "resource"},
		Source: core.CapabilitySource{
			ProviderID: providerID,
			Scope:      agentspec.CapabilityScopeRemote,
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

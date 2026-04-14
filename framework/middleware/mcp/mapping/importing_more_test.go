package mapping

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	"github.com/stretchr/testify/require"
)

func TestImportedPromptDescriptor(t *testing.T) {
	t.Run("basic prompt conversion", func(t *testing.T) {
		prompt := protocol.Prompt{
			Name:        "test.prompt",
			Description: "A test prompt",
			Arguments: []protocol.PromptArgument{
				{Name: "name", Description: "The name", Required: true},
				{Name: "age", Description: "The age", Required: false},
			},
		}
		desc := ImportedPromptDescriptor("provider-1", "session-1", protocol.Revision20250618, prompt, core.TrustClassRemoteDeclared)

		require.Equal(t, "mcp:provider-1:prompt:test.prompt", desc.ID)
		require.Equal(t, core.CapabilityKindPrompt, desc.Kind)
		require.Equal(t, "test.prompt", desc.Name)
		require.Equal(t, "A test prompt", desc.Description)
		require.Equal(t, core.CapabilityRuntimeFamilyProvider, desc.RuntimeFamily)
		require.Equal(t, core.CapabilityScopeRemote, desc.Source.Scope)
		require.Equal(t, "session-1", desc.Source.SessionID)
		require.Equal(t, core.TrustClassRemoteDeclared, desc.TrustClass)
		require.Equal(t, "session-1", desc.SessionAffinity)

		// Check schema
		require.NotNil(t, desc.InputSchema)
		require.Equal(t, "object", desc.InputSchema.Type)
		require.Len(t, desc.InputSchema.Properties, 2)
		require.Contains(t, desc.InputSchema.Properties, "name")
		require.Contains(t, desc.InputSchema.Properties, "age")

		// Check required fields
		require.Len(t, desc.InputSchema.Required, 1)
		require.Contains(t, desc.InputSchema.Required, "name")

		// Check annotations
		require.Equal(t, "prompt", desc.Annotations["mcp_kind"])
		require.Equal(t, "test.prompt", desc.Annotations["mcp_remote_name"])
		require.Equal(t, protocol.Revision20250618, desc.Annotations["protocol_version"])
	})

	t.Run("prompt with no arguments", func(t *testing.T) {
		prompt := protocol.Prompt{
			Name:        "simple.prompt",
			Description: "Simple prompt",
		}
		desc := ImportedPromptDescriptor("provider-1", "session-1", protocol.Revision20250618, prompt, core.TrustClassRemoteApproved)

		require.Equal(t, "mcp:provider-1:prompt:simple.prompt", desc.ID)
		require.NotNil(t, desc.InputSchema)
		require.Equal(t, "object", desc.InputSchema.Type)
		require.Empty(t, desc.InputSchema.Properties)
		require.Empty(t, desc.InputSchema.Required)
	})

	t.Run("empty argument name skipped", func(t *testing.T) {
		prompt := protocol.Prompt{
			Name: "test.prompt",
			Arguments: []protocol.PromptArgument{
				{Name: "", Description: "Empty name"},
				{Name: "valid", Description: "Valid name"},
			},
		}
		desc := ImportedPromptDescriptor("provider-1", "session-1", protocol.Revision20250618, prompt, core.TrustClassRemoteDeclared)

		require.Len(t, desc.InputSchema.Properties, 1)
		require.Contains(t, desc.InputSchema.Properties, "valid")
	})

	t.Run("whitespace trimmed from name", func(t *testing.T) {
		prompt := protocol.Prompt{
			Name: "  test.prompt  ",
		}
		desc := ImportedPromptDescriptor("provider-1", "session-1", protocol.Revision20250618, prompt, core.TrustClassRemoteDeclared)

		require.Equal(t, "mcp:provider-1:prompt:test.prompt", desc.ID)
		require.Equal(t, "test.prompt", desc.Name)
	})

	t.Run("version trimmed", func(t *testing.T) {
		prompt := protocol.Prompt{
			Name: "test.prompt",
		}
		desc := ImportedPromptDescriptor("provider-1", "session-1", "  2025-06-18  ", prompt, core.TrustClassRemoteDeclared)

		// Note: strings.TrimSpace is applied to version
		require.Equal(t, "2025-06-18", desc.Version)
		require.Equal(t, "  2025-06-18  ", desc.Annotations["protocol_version"]) // Annotation uses original value
	})
}

func TestImportedToolDescriptor(t *testing.T) {
	t.Run("empty tool name", func(t *testing.T) {
		tool := protocol.Tool{
			Name: "",
		}
		_, err := ImportedToolDescriptor("provider-1", "session-1", protocol.Revision20250618, tool, core.TrustClassRemoteDeclared)
		require.Error(t, err)
		require.Contains(t, err.Error(), "remote tool name required")
	})

	t.Run("whitespace only tool name", func(t *testing.T) {
		tool := protocol.Tool{
			Name: "   ",
		}
		_, err := ImportedToolDescriptor("provider-1", "session-1", protocol.Revision20250618, tool, core.TrustClassRemoteDeclared)
		require.Error(t, err)
		require.Contains(t, err.Error(), "remote tool name required")
	})

	t.Run("schema type not string ignored", func(t *testing.T) {
		tool := protocol.Tool{
			Name:        "test.tool",
			InputSchema: map[string]any{"type": 123}, // non-string type is ignored
		}
		desc, err := ImportedToolDescriptor("provider-1", "session-1", protocol.Revision20250618, tool, core.TrustClassRemoteDeclared)
		require.NoError(t, err)
		require.NotNil(t, desc.InputSchema)
		require.Empty(t, desc.InputSchema.Type) // Type is empty since 123 is not a string
	})
}

func TestSanitizeResourceID(t *testing.T) {
	t.Run("basic sanitization", func(t *testing.T) {
		result := sanitizeResourceID("file:///tmp/test.json")
		require.Equal(t, "file____tmp_test_json", result)
	})

	t.Run("empty string", func(t *testing.T) {
		result := sanitizeResourceID("")
		require.Equal(t, "unnamed", result)
	})

	t.Run("whitespace only", func(t *testing.T) {
		result := sanitizeResourceID("   ")
		require.Equal(t, "unnamed", result)
	})

	t.Run("special characters", func(t *testing.T) {
		result := sanitizeResourceID("path?query=value&foo=bar#fragment")
		require.Equal(t, "path_query_value_foo_bar_fragment", result)
	})

	t.Run("backslash replaced", func(t *testing.T) {
		result := sanitizeResourceID("path\\to\\file")
		require.Equal(t, "path_to_file", result)
	})

	t.Run("dots replaced", func(t *testing.T) {
		result := sanitizeResourceID("file.txt")
		require.Equal(t, "file_txt", result)
	})

	t.Run("whitespace trimmed before sanitization", func(t *testing.T) {
		result := sanitizeResourceID("  file.txt  ")
		require.Equal(t, "file_txt", result)
	})
}

func TestFirstNonEmpty(t *testing.T) {
	t.Run("first value non-empty", func(t *testing.T) {
		result := firstNonEmpty("first", "second", "third")
		require.Equal(t, "first", result)
	})

	t.Run("second value non-empty", func(t *testing.T) {
		result := firstNonEmpty("", "second", "third")
		require.Equal(t, "second", result)
	})

	t.Run("third value non-empty", func(t *testing.T) {
		result := firstNonEmpty("", "", "third")
		require.Equal(t, "third", result)
	})

	t.Run("whitespace only skipped", func(t *testing.T) {
		result := firstNonEmpty("  ", "  ", "value")
		require.Equal(t, "value", result)
	})

	t.Run("all empty", func(t *testing.T) {
		result := firstNonEmpty("", "", "")
		require.Equal(t, "", result)
	})

	t.Run("all whitespace", func(t *testing.T) {
		result := firstNonEmpty("  ", "  ", "  ")
		require.Equal(t, "", result)
	})

	t.Run("no arguments", func(t *testing.T) {
		result := firstNonEmpty()
		require.Equal(t, "", result)
	})

	t.Run("whitespace trimmed in result", func(t *testing.T) {
		result := firstNonEmpty("  value  ")
		require.Equal(t, "value", result)
	})
}

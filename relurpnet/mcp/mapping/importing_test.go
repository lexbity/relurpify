package mapping

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/mcp/protocol"
	"github.com/stretchr/testify/require"
)

func TestImportedToolDescriptorBuildsStableProviderCapability(t *testing.T) {
	desc, err := ImportedToolDescriptor("remote-mcp", "session-1", protocol.Revision20250618, protocol.Tool{
		Name:        "remote.echo",
		Description: "Echo from remote",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}, core.TrustClassRemoteDeclared)
	require.NoError(t, err)
	require.Equal(t, "mcp:remote-mcp:tool:remote.echo", desc.ID)
	require.Equal(t, core.CapabilityRuntimeFamilyProvider, desc.RuntimeFamily)
	require.Equal(t, core.CapabilityScopeRemote, desc.Source.Scope)
	require.Equal(t, "session-1", desc.Source.SessionID)
	require.Equal(t, "string", desc.InputSchema.Properties["message"].Type)
}

func TestImportedResourceDescriptorSanitizesURIIntoStableID(t *testing.T) {
	desc := ImportedResourceDescriptor("remote-mcp", "session-1", protocol.Revision20250618, protocol.Resource{
		URI: "file:///tmp/report.json",
	}, core.TrustClassRemoteDeclared)
	require.Equal(t, "mcp:remote-mcp:resource:file____tmp_report_json", desc.ID)
}

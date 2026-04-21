package policybundle

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestBuildFromSpecCompilesEffectiveSessionPolicies(t *testing.T) {
	spec := &core.AgentRuntimeSpec{
		Mode:  core.AgentModePrimary,
		Model: core.AgentModelConfig{Provider: "ollama", Name: "qwen"},
		SessionPolicies: []core.SessionPolicy{{
			ID:      "owner-send",
			Name:    "Owner Send",
			Enabled: true,
			Selector: core.SessionSelector{
				Operations: []core.SessionOperation{core.SessionOperationSend},
				Scopes:     []core.SessionScope{core.SessionScopePerChannelPeer},
			},
			Effect: core.AgentPermissionAllow,
		}},
	}

	manager, err := authorization.NewPermissionManager("/workspace", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{
			Action: core.FileSystemRead,
			Path:   "src/**",
		}},
	}, core.NewInMemoryAuditLogger(32), authorization.NewHITLBroker(0))
	require.NoError(t, err)

	bundle, err := BuildFromSpec("agent-1", spec, manager)
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Engine)
	require.Len(t, bundle.Rules, 1)
	require.Equal(t, "owner-send", bundle.Rules[0].ID)
}

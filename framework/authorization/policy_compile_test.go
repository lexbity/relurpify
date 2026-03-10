package authorization

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/stretchr/testify/require"
)

func TestCompileManifestPolicyRulesIncludesSessionAndProviderPolicies(t *testing.T) {
	ownerOnly := true
	rules, err := CompileManifestPolicyRules(&manifest.AgentManifest{
		Metadata: manifest.ManifestMetadata{Name: "test-agent"},
		Spec: manifest.ManifestSpec{
			Agent: &core.AgentRuntimeSpec{
				Mode: core.AgentModePrimary,
				Model: core.AgentModelConfig{
					Provider: "ollama",
					Name:     "test",
				},
				ToolExecutionPolicy: map[string]core.ToolPolicy{
					"file_read": {Execute: core.AgentPermissionAsk},
				},
				ProviderPolicies: map[string]core.ProviderPolicy{
					"remote-mcp": {Activate: core.AgentPermissionDeny},
				},
				SessionPolicies: []core.SessionPolicy{{
					ID:      "owner-send",
					Name:    "Owner send",
					Enabled: true,
					Selector: core.SessionSelector{
						Operations:       []core.SessionOperation{core.SessionOperationSend},
						RequireOwnership: &ownerOnly,
					},
					Effect: core.AgentPermissionAllow,
				}},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, rules, 3)
	require.Equal(t, "provider:remote-mcp:activate", rules[0].ID)
	require.Equal(t, "owner-send", rules[1].ID)
	require.Equal(t, "tool:file_read", rules[2].ID)
}

func TestCompileManifestPolicyRulesRejectsUnsupportedCapabilitySelector(t *testing.T) {
	_, err := CompileManifestPolicyRules(&manifest.AgentManifest{
		Metadata: manifest.ManifestMetadata{Name: "test-agent"},
		Spec: manifest.ManifestSpec{
			Agent: &core.AgentRuntimeSpec{
				Mode: core.AgentModePrimary,
				Model: core.AgentModelConfig{
					Provider: "ollama",
					Name:     "test",
				},
				CapabilityPolicies: []core.CapabilityPolicy{{
					Selector: core.CapabilitySelector{
						Tags: []string{"search"},
					},
					Execute: core.AgentPermissionDeny,
				}},
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "descriptor-time evaluation")
}

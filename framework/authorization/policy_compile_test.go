package authorization

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"github.com/stretchr/testify/require"
)

func TestCompileManifestPolicyRulesIncludesSessionAndProviderPolicies(t *testing.T) {
	ownerOnly := true
	delegated := true
	hasBinding := true
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
				SessionPolicies: []agentspec.SessionPolicy{{
					ID:      "owner-send",
					Name:    "Owner send",
					Enabled: true,
					Selector: agentspec.SessionSelector{
						Operations:             []agentspec.SessionOperation{agentspec.SessionOperationSend},
						RequireOwnership:       &ownerOnly,
						RequireDelegation:      &delegated,
						RequireExternalBinding: &hasBinding,
						ExternalProviders:      []agentspec.ExternalProvider{agentspec.ExternalProviderDiscord},
					},
					Effect: agentspec.AgentPermissionAllow,
				}},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, rules, 3)
	require.Equal(t, "provider:remote-mcp:activate", rules[0].ID)
	require.Equal(t, "owner-send", rules[1].ID)
	require.Equal(t, "tool:file_read", rules[2].ID)
	require.Equal(t, &ownerOnly, rules[1].Conditions.RequireOwnership)
	require.Equal(t, &delegated, rules[1].Conditions.RequireDelegation)
	require.Equal(t, &hasBinding, rules[1].Conditions.RequireExternalBinding)
	require.Equal(t, []string{string(core.ExternalProviderDiscord)}, rules[1].Conditions.ExternalProviders)
}

func TestCompileManifestPolicyRulesRejectsUnsupportedCapabilitySelector(t *testing.T) {
	_, err := CompileManifestPolicyRules(&manifest.AgentManifest{
		Metadata: manifest.ManifestMetadata{Name: "test-agent"},
		Spec: manifest.ManifestSpec{
			Agent: &agentspec.AgentRuntimeSpec{
				Mode: agentspec.AgentModePrimary,
				Model: agentspec.AgentModelConfig{
					Provider: "ollama",
					Name:     "test",
				},
				CapabilityPolicies: []agentspec.CapabilityPolicy{{
					Selector: agentspec.CapabilitySelector{
						Tags: []string{"search"},
					},
					Execute: agentspec.AgentPermissionDeny,
				}},
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "descriptor-time evaluation")
}

func TestCompileManifestPolicyRulesAcceptsRiskClassSelectorAfterSpecClone(t *testing.T) {
	spec := &agentspec.AgentRuntimeSpec{
		Mode: agentspec.AgentModePrimary,
		Model: agentspec.AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		CapabilityPolicies: []agentspec.CapabilityPolicy{{
			Selector: agentspec.CapabilitySelector{
				Kind:        agentspec.CapabilityKindTool,
				RiskClasses: []agentspec.RiskClass{agentspec.RiskClassDestructive},
			},
			Execute: agentspec.AgentPermissionAsk,
		}},
	}

	cloned := agentspec.MergeAgentSpecs(spec, agentspec.AgentSpecOverlay{})
	rules, err := CompileAgentSpecPolicyRules(cloned)

	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, "capability-policy:0", rules[0].ID)
	require.Equal(t, []agentspec.CapabilityKind{agentspec.CapabilityKindTool}, rules[0].Conditions.CapabilityKinds)
	require.Equal(t, []agentspec.RiskClass{agentspec.RiskClassDestructive}, rules[0].Conditions.MinRiskClasses)
}

package authorization

import (
	"testing"

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
				SessionPolicies: []core.SessionPolicy{{
					ID:      "owner-send",
					Name:    "Owner send",
					Enabled: true,
					Selector: core.SessionSelector{
						Operations:             []core.SessionOperation{core.SessionOperationSend},
						RequireOwnership:       &ownerOnly,
						RequireDelegation:      &delegated,
						RequireExternalBinding: &hasBinding,
						ExternalProviders:      []core.ExternalProvider{core.ExternalProviderDiscord},
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
	require.Equal(t, &ownerOnly, rules[1].Conditions.RequireOwnership)
	require.Equal(t, &delegated, rules[1].Conditions.RequireDelegation)
	require.Equal(t, &hasBinding, rules[1].Conditions.RequireExternalBinding)
	require.Equal(t, []core.ExternalProvider{core.ExternalProviderDiscord}, rules[1].Conditions.ExternalProviders)
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

func TestCompileManifestPolicyRulesAcceptsRiskClassSelectorAfterSpecClone(t *testing.T) {
	spec := &core.AgentRuntimeSpec{
		Mode: core.AgentModePrimary,
		Model: core.AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		CapabilityPolicies: []core.CapabilityPolicy{{
			Selector: core.CapabilitySelector{
				Kind:        core.CapabilityKindTool,
				RiskClasses: []core.RiskClass{core.RiskClassDestructive},
			},
			Execute: core.AgentPermissionAsk,
		}},
	}

	cloned := core.MergeAgentSpecs(spec, core.AgentSpecOverlay{})
	rules, err := CompileAgentSpecPolicyRules(cloned)

	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, "capability-policy:0", rules[0].ID)
	require.Equal(t, []core.CapabilityKind{core.CapabilityKindTool}, rules[0].Conditions.CapabilityKinds)
	require.Equal(t, []core.RiskClass{core.RiskClassDestructive}, rules[0].Conditions.MinRiskClasses)
}

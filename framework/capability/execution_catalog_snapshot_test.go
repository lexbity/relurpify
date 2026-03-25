package capability

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestCaptureExecutionCatalogSnapshotClonesEffectiveVisibility(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(capabilityStubTool{name: "local_keep"}))
	require.NoError(t, registry.RegisterInvocableCapability(providerInvocableCapability("remote_echo", "remote-mcp", "session-1", "adapted")))

	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		Mode:  core.AgentModePrimary,
		Model: core.AgentModelConfig{Provider: "test", Name: "test"},
		ExposurePolicies: []core.CapabilityExposurePolicy{{
			Selector: core.CapabilitySelector{
				Name:            "remote_echo",
				RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider},
			},
			Access: core.CapabilityExposureCallable,
		}},
	})

	snapshot := registry.CaptureExecutionCatalogSnapshot()
	require.NotNil(t, snapshot)
	require.Len(t, snapshot.CallableCapabilities(), 2)
	require.Len(t, snapshot.InspectableCapabilities(), 2)

	specs := snapshot.ModelCallableLLMToolSpecs()
	require.Len(t, specs, 2)
	require.ElementsMatch(t, []string{"local_keep", "remote_echo"}, []string{specs[0].Name, specs[1].Name})

	tool, ok := snapshot.GetModelTool("local_keep")
	require.True(t, ok)
	require.Equal(t, "local_keep", tool.Name())

	entry, ok := snapshot.GetCapability("provider:remote_echo")
	require.True(t, ok)
	require.True(t, entry.Callable)
	require.True(t, entry.ModelCallable)
	require.False(t, entry.LocalTool)

	registry.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{
			Name:            "remote_echo",
			RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider},
		},
		Access: core.CapabilityExposureHidden,
	}})

	require.Len(t, registry.ModelCallableLLMToolSpecs(), 1)
	require.Len(t, snapshot.ModelCallableLLMToolSpecs(), 2)
	entry, ok = snapshot.GetCapability("remote_echo")
	require.True(t, ok)
	require.Equal(t, core.CapabilityExposureCallable, entry.Exposure)
}

func TestExecutionCatalogSnapshotPolicySnapshotRemainsStable(t *testing.T) {
	registry := NewCapabilityRegistry()
	registry.UseAgentSpec("agent-1", &AgentRuntimeSpec{
		ToolExecutionPolicy: map[string]ToolPolicy{
			"cli_git": {Execute: AgentPermissionAsk},
		},
		GlobalPolicies: map[string]AgentPermissionLevel{
			string(core.RiskClassExecute): AgentPermissionDeny,
		},
	})

	snapshot := registry.CaptureExecutionCatalogSnapshot()
	require.NotNil(t, snapshot)

	policy := snapshot.PolicySnapshot()
	require.NotNil(t, policy)
	require.Equal(t, "agent-1", policy.AgentID)
	require.Equal(t, AgentPermissionAsk, policy.ToolPolicies["cli_git"].Execute)
	require.Equal(t, AgentPermissionDeny, policy.GlobalPolicies[string(core.RiskClassExecute)])

	registry.UpdateToolPolicy("cli_git", ToolPolicy{Execute: AgentPermissionAllow})
	registry.UpdateClassPolicy(string(core.RiskClassExecute), AgentPermissionAllow)

	live := registry.CapturePolicySnapshot()
	require.Equal(t, AgentPermissionAllow, live.ToolPolicies["cli_git"].Execute)
	require.Equal(t, AgentPermissionAllow, live.GlobalPolicies[string(core.RiskClassExecute)])

	policy = snapshot.PolicySnapshot()
	require.Equal(t, AgentPermissionAsk, policy.ToolPolicies["cli_git"].Execute)
	require.Equal(t, AgentPermissionDeny, policy.GlobalPolicies[string(core.RiskClassExecute)])
}

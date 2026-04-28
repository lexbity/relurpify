package capability

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
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
			Selector: agentspec.CapabilitySelector{
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
		Selector: agentspec.CapabilitySelector{
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
			"cli_git": {Execute: agentspec.AgentPermissionAsk},
		},
		GlobalPolicies: map[string]AgentPermissionLevel{
			string(core.RiskClassExecute): agentspec.AgentPermissionDeny,
		},
	})

	snapshot := registry.CaptureExecutionCatalogSnapshot()
	require.NotNil(t, snapshot)

	policy := snapshot.PolicySnapshot()
	require.NotNil(t, policy)
	require.Equal(t, "agent-1", policy.AgentID)
	require.Equal(t, agentspec.AgentPermissionAsk, policy.ToolPolicies["cli_git"].Execute)
	require.Equal(t, agentspec.AgentPermissionDeny, policy.GlobalPolicies[string(core.RiskClassExecute)])

	registry.UpdateToolPolicy("cli_git", ToolPolicy{Execute: agentspec.AgentPermissionAllow})
	registry.UpdateClassPolicy(string(core.RiskClassExecute), agentspec.AgentPermissionAllow)

	live := registry.CapturePolicySnapshot()
	require.Equal(t, agentspec.AgentPermissionAllow, live.ToolPolicies["cli_git"].Execute)
	require.Equal(t, agentspec.AgentPermissionAllow, live.GlobalPolicies[string(core.RiskClassExecute)])

	policy = snapshot.PolicySnapshot()
	require.Equal(t, agentspec.AgentPermissionAsk, policy.ToolPolicies["cli_git"].Execute)
	require.Equal(t, agentspec.AgentPermissionDeny, policy.GlobalPolicies[string(core.RiskClassExecute)])
}

func TestExecutionCatalogSnapshotAllowedCapabilitiesRemainStable(t *testing.T) {
	longRunning := true
	registry := NewCapabilityRegistry()
	registry.UseAgentSpec("agent-1", &AgentRuntimeSpec{
		AllowedCapabilities: []agentspec.CapabilitySelector{{
			Name:                        "reviewer",
			RuntimeFamilies:             []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyRelurpic},
			Tags:                        []string{"lang:go"},
			CoordinationRoles:           []agentspec.CoordinationRole{agentspec.CoordinationRoleReviewer},
			CoordinationLongRunning:     &longRunning,
			CoordinationDirectInsertion: capabilityBoolPtr(false),
		}},
	})

	snapshot := registry.CaptureExecutionCatalogSnapshot()
	require.NotNil(t, snapshot)

	allowed := snapshot.AllowedCapabilities()
	require.Len(t, allowed, 1)
	allowed[0].Tags[0] = "mutated"
	allowed[0].RuntimeFamilies[0] = core.CapabilityRuntimeFamilyProvider
	*allowed[0].CoordinationLongRunning = false
	*allowed[0].CoordinationDirectInsertion = true

	stable := snapshot.AllowedCapabilities()
	require.Equal(t, "lang:go", stable[0].Tags[0])
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, stable[0].RuntimeFamilies[0])
	require.True(t, *stable[0].CoordinationLongRunning)
	require.False(t, *stable[0].CoordinationDirectInsertion)
}

func capabilityBoolPtr(value bool) *bool {
	return &value
}

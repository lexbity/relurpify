package skills

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type resolverStubTool struct {
	name string
	tags []string
}

func (t resolverStubTool) Name() string        { return t.name }
func (t resolverStubTool) Description() string { return t.name }
func (t resolverStubTool) Category() string    { return "test" }
func (t resolverStubTool) Parameters() []core.ToolParameter {
	return nil
}
func (t resolverStubTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t resolverStubTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t resolverStubTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t resolverStubTool) Tags() []string                                  { return t.tags }

type resolverInvocableCapability struct {
	desc core.CapabilityDescriptor
}

func (c resolverInvocableCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c resolverInvocableCapability) Invoke(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}

func TestResolveSkillPolicyResolvesSelectorsByTags(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "go_build", tags: []string{"execute", "lang:go", "build"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "file_read", tags: []string{"read-only", "file"}}))

	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{
		PhaseCapabilitySelectors: map[string][]core.SkillCapabilitySelector{
			"verify": {
				{Tags: []string{"lang:go", "test"}},
				{Capability: "file_read"},
			},
		},
		Verification: core.AgentVerificationPolicy{
			SuccessCapabilitySelectors: []core.SkillCapabilitySelector{{Tags: []string{"lang:go", "build"}}},
		},
		Recovery: core.AgentRecoveryPolicy{
			FailureProbeCapabilitySelectors: []core.SkillCapabilitySelector{{Tags: []string{"file"}}},
		},
		Planning: core.AgentPlanningPolicy{
			RequiredBeforeEdit:          []core.SkillCapabilitySelector{{Tags: []string{"lang:go", "test"}}},
			PreferredVerifyCapabilities: []core.SkillCapabilitySelector{{Tags: []string{"lang:go", "build"}}},
			StepTemplates:               []core.SkillStepTemplate{{Kind: "verify", Description: "Run tests"}},
			RequireVerificationStep:     true,
		},
		Review: core.AgentReviewPolicy{
			Criteria:  []string{"correctness"},
			FocusTags: []string{"verification"},
			ApprovalRules: core.AgentReviewApprovalRules{
				RequireVerificationEvidence: true,
			},
			SeverityWeights: map[string]float64{"high": 1},
		},
	})

	require.Equal(t, []string{"go_test", "file_read"}, policy.PhaseCapabilities["verify"])
	require.Equal(t, []string{"go_build"}, policy.VerificationSuccessCapabilities)
	require.Equal(t, []string{"file_read"}, policy.RecoveryProbeCapabilities)
	require.Equal(t, []string{"go_test"}, policy.Planning.RequiredBeforeEdit)
	require.Equal(t, []string{"go_build"}, policy.Planning.PreferredVerifyCapabilities)
	require.True(t, policy.Planning.RequireVerificationStep)
	require.Equal(t, []string{"correctness"}, policy.Review.Criteria)
	require.True(t, policy.Review.ApprovalRules.RequireVerificationEvidence)
}

func TestResolveSkillPolicyMergesPhaseCapabilitiesAndSelectors(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "cli_go", tags: []string{"execute", "lang:go"}}))

	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{
		PhaseCapabilities: map[string][]string{
			"verify": {"cli_go"},
		},
		PhaseCapabilitySelectors: map[string][]core.SkillCapabilitySelector{
			"verify": {
				{Tags: []string{"lang:go", "test"}},
				{Capability: "cli_go"},
			},
		},
	})

	require.Equal(t, []string{"cli_go", "go_test"}, policy.PhaseCapabilities["verify"])
}

func TestResolveSkillPolicySupportsPhaseCapabilitiesAndCapabilitySelectors(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "cli_go", tags: []string{"execute", "lang:go"}}))

	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{
		PhaseCapabilities: map[string][]string{
			"verify": {"cli_go"},
		},
		PhaseCapabilitySelectors: map[string][]core.SkillCapabilitySelector{
			"verify": {
				{Tags: []string{"lang:go", "test"}},
				{Capability: "cli_go"},
			},
		},
	})

	require.Equal(t, []string{"cli_go", "go_test"}, policy.PhaseCapabilities["verify"])
}

func TestResolveSkillPolicyIgnoresSecurityTagsForGrouping(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))

	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{
		PhaseCapabilitySelectors: map[string][]core.SkillCapabilitySelector{
			"verify": {{Tags: []string{"execute"}}},
		},
	})

	require.Empty(t, policy.PhaseCapabilities["verify"])
}

func TestResolveSkillPolicySupportsRuntimeFamilySelectors(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(resolverInvocableCapability{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:planner.plan",
			Name:          "planner.plan",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
	}))

	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{
		PhaseCapabilitySelectors: map[string][]core.SkillCapabilitySelector{
			"explore": {{
				RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyRelurpic},
			}},
		},
	})

	require.Equal(t, []string{"planner.plan"}, policy.PhaseCapabilities["explore"])
}

func TestResolveEffectiveSkillPolicyPrefersTaskAgentSpec(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "cli_go", tags: []string{"execute", "lang:go"}}))

	fallback := &core.AgentRuntimeSpec{
		SkillConfig: core.AgentSkillConfig{
			Verification: core.AgentVerificationPolicy{
				SuccessTools: []string{"cli_go"},
			},
		},
	}
	override := &core.AgentRuntimeSpec{
		SkillConfig: core.AgentSkillConfig{
			Verification: core.AgentVerificationPolicy{
				SuccessCapabilitySelectors: []core.SkillCapabilitySelector{{Tags: []string{"lang:go", "test"}}},
			},
		},
	}
	task := &core.Task{
		Context: map[string]any{
			"agent_spec": override,
		},
	}

	effective := ResolveEffectiveSkillPolicy(task, fallback, registry)

	require.Same(t, override, effective.Spec)
	require.Equal(t, []string{"go_test"}, effective.Policy.VerificationSuccessCapabilities)
}

func TestEffectiveAgentSpecFallsBackWhenTaskHasNoOverride(t *testing.T) {
	fallback := &core.AgentRuntimeSpec{}
	require.Same(t, fallback, EffectiveAgentSpec(&core.Task{}, fallback))
	require.Nil(t, EffectiveAgentSpec(nil, nil))
}

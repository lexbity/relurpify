package toolsys

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
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

func TestResolveSkillPolicyResolvesSelectorsByTags(t *testing.T) {
	registry := NewToolRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "go_build", tags: []string{"execute", "lang:go", "build"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "file_read", tags: []string{"read-only", "file"}}))

	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{
		PhaseSelectors: map[string][]core.SkillToolSelector{
			"verify": {
				{Tags: []string{"lang:go", "test"}},
				{Tool: "file_read"},
			},
		},
		Verification: core.AgentVerificationPolicy{
			SuccessSelectors: []core.SkillToolSelector{{Tags: []string{"lang:go", "build"}}},
		},
		Recovery: core.AgentRecoveryPolicy{
			FailureProbeSelectors: []core.SkillToolSelector{{Tags: []string{"file"}}},
		},
		Planning: core.AgentPlanningPolicy{
			RequiredBeforeEdit:      []core.SkillToolSelector{{Tags: []string{"lang:go", "test"}}},
			PreferredVerifyTools:    []core.SkillToolSelector{{Tags: []string{"lang:go", "build"}}},
			StepTemplates:           []core.SkillStepTemplate{{Kind: "verify", Description: "Run tests"}},
			RequireVerificationStep: true,
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

	require.Equal(t, []string{"go_test", "file_read"}, policy.PhaseTools["verify"])
	require.Equal(t, []string{"go_build"}, policy.VerificationSuccessTools)
	require.Equal(t, []string{"file_read"}, policy.RecoveryProbeTools)
	require.Equal(t, []string{"go_test"}, policy.Planning.RequiredBeforeEdit)
	require.Equal(t, []string{"go_build"}, policy.Planning.PreferredVerifyTools)
	require.True(t, policy.Planning.RequireVerificationStep)
	require.Equal(t, []string{"correctness"}, policy.Review.Criteria)
	require.True(t, policy.Review.ApprovalRules.RequireVerificationEvidence)
}

func TestResolveSkillPolicyMergesLegacyToolsAndSelectors(t *testing.T) {
	registry := NewToolRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "cli_go", tags: []string{"execute", "lang:go"}}))

	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{
		PhaseTools: map[string][]string{
			"verify": {"cli_go"},
		},
		PhaseSelectors: map[string][]core.SkillToolSelector{
			"verify": {
				{Tags: []string{"lang:go", "test"}},
				{Tool: "cli_go"},
			},
		},
	})

	require.Equal(t, []string{"cli_go", "go_test"}, policy.PhaseTools["verify"])
}

func TestResolveEffectiveSkillPolicyPrefersTaskAgentSpec(t *testing.T) {
	registry := NewToolRegistry()
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
				SuccessSelectors: []core.SkillToolSelector{{Tags: []string{"lang:go", "test"}}},
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
	require.Equal(t, []string{"go_test"}, effective.Policy.VerificationSuccessTools)
}

func TestEffectiveAgentSpecFallsBackWhenTaskHasNoOverride(t *testing.T) {
	fallback := &core.AgentRuntimeSpec{}
	require.Same(t, fallback, EffectiveAgentSpec(&core.Task{}, fallback))
	require.Nil(t, EffectiveAgentSpec(nil, nil))
}

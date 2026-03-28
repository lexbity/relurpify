package runtime

import (
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
)

func BuildResolvedExecutionPolicy(task *core.Task, cfg *core.Config, registry *capability.Registry, mode ModeResolution, profile ExecutionProfileSelection) ResolvedExecutionPolicy {
	fallback := (*core.AgentRuntimeSpec)(nil)
	if cfg != nil {
		fallback = cfg.AgentSpec
	}
	effective := frameworkskills.ResolveEffectiveSkillPolicy(task, fallback, registry)
	policy := ResolvedExecutionPolicy{
		ModeID:                  mode.ModeID,
		ProfileID:               profile.ProfileID,
		ContextPolicy:           contextPolicySummary(effective.Spec),
		ResolvedFromSkillPolicy: effective.Spec != nil,
	}
	if effective.Spec == nil {
		return policy
	}
	policy.PhaseCapabilityConstraints = cloneStringSliceMap(effective.Policy.PhaseCapabilities)
	policy.PreferredPlanningCapabilities = append([]string(nil), effective.Policy.Planning.RequiredBeforeEdit...)
	policy.PreferredPlanningCapabilities = uniqueStrings(append(policy.PreferredPlanningCapabilities, effective.Policy.Planning.PreferredEditCapabilities...))
	policy.PreferredVerifyCapabilities = append([]string(nil), effective.Policy.Planning.PreferredVerifyCapabilities...)
	policy.RecoveryProbeCapabilities = append([]string(nil), effective.Policy.RecoveryProbeCapabilities...)
	policy.VerificationSuccessCapabilities = append([]string(nil), effective.Policy.VerificationSuccessCapabilities...)
	policy.PlanningStepTemplates = append([]core.SkillStepTemplate(nil), effective.Policy.Planning.StepTemplates...)
	policy.RequireVerificationStep = effective.Policy.Planning.RequireVerificationStep
	policy.ReviewCriteria = append([]string(nil), effective.Policy.Review.Criteria...)
	policy.ReviewFocusTags = append([]string(nil), effective.Policy.Review.FocusTags...)
	policy.ReviewApprovalRules = effective.Policy.Review.ApprovalRules
	return policy
}

func contextPolicySummary(spec *core.AgentRuntimeSpec) ContextPolicySummary {
	if spec == nil {
		return ContextPolicySummary{}
	}
	progressive := true
	if spec.Context.ProgressiveLoading != nil {
		progressive = *spec.Context.ProgressiveLoading
	}
	summary := ContextPolicySummary{
		MaxTokens:           spec.Context.MaxTokens,
		CompressionStrategy: spec.Context.CompressionStrategy,
		ProgressiveLoading:  progressive,
		ProtectPatterns:     append([]string(nil), spec.SkillConfig.ContextHints.ProtectPatterns...),
	}
	switch strings.ToLower(spec.SkillConfig.ContextHints.PreferredDetailLevel) {
	case "minimal", "concise", "detailed", "full":
		summary.PreferredDetail = strings.ToLower(spec.SkillConfig.ContextHints.PreferredDetailLevel)
	default:
		summary.PreferredDetail = contextmgr.DetailDetailed.String()
	}
	return summary
}

func SelectExecutorDescriptor(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification, policy ResolvedExecutionPolicy, planBinding *UnitOfWorkPlanBinding) WorkUnitExecutorDescriptor {
	switch {
	case mode.ModeID == "review":
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.reflection", Family: ExecutorFamilyReflection, Reason: "review mode requires reflective correction", Compatibility: true}
	case planBinding != nil && planBinding.IsLongRunning:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.rewoo", Family: ExecutorFamilyRewoo, Reason: "long-running plan-backed execution prefers checkpointed context management", Compatibility: true}
	case mode.ModeID == "planning" || profile.ProfileID == "plan_stage_execute":
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.planner", Family: ExecutorFamilyPlanner, Reason: "planning execution prefers explicit plan-first orchestration", Compatibility: true}
	case classification.RequiresDeterministicStages || policy.RequireVerificationStep:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.htn", Family: ExecutorFamilyHTN, Reason: "deterministic stages require structured decomposition", Compatibility: true}
	default:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.react", Family: ExecutorFamilyReact, Reason: "focused direct execution defaults to react-style orchestration", Compatibility: true}
	}
}

func cloneStringSliceMap(input map[string][]string) map[string][]string {
	if input == nil {
		return nil
	}
	out := make(map[string][]string, len(input))
	for key, values := range input {
		out[key] = append([]string(nil), values...)
	}
	return out
}

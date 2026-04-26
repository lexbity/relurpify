package runtime

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/capability"

	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
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
	if spec.ArtifactWindow.ProgressiveLoading != nil {
		progressive = *spec.ArtifactWindow.ProgressiveLoading
	}
	summary := ContextPolicySummary{
		MaxTokens:           spec.ArtifactWindow.MaxTokens,
		CompressionStrategy: spec.ArtifactWindow.CompressionStrategy,
		ProgressiveLoading:  progressive,
		ProtectPatterns:     append([]string(nil), spec.SkillConfig.ContextHints.ProtectPatterns...),
	}
	switch strings.ToLower(spec.SkillConfig.ContextHints.PreferredDetailLevel) {
	case "minimal", "concise", "detailed", "full":
		summary.PreferredDetail = strings.ToLower(spec.SkillConfig.ContextHints.PreferredDetailLevel)
	default:
		summary.PreferredDetail = "detailed"
	}
	return summary
}

func SelectExecutorDescriptor(mode ModeResolution, profile ExecutionProfileSelection, classification TaskClassification, policy ResolvedExecutionPolicy, planBinding *UnitOfWorkPlanBinding, primaryCapabilityID string, supportingCapabilityIDs []string) WorkUnitExecutorDescriptor {
	reason := fmt.Sprintf("executor derived from primary relurpic capability %s", primaryCapabilityID)

	switch {
	case primaryCapabilityID == euclorelurpic.CapabilityChatAsk:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.react", Family: ExecutorFamilyReact, RecipeID: "", Reason: reason}
	case primaryCapabilityID == euclorelurpic.CapabilityChatInspect && mode.ModeID == "review":
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.reflection", Family: ExecutorFamilyReflection, RecipeID: "review.chat-inspect.reflection", Reason: reason}
	case primaryCapabilityID == euclorelurpic.CapabilityChatInspect:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.react", Family: ExecutorFamilyReact, RecipeID: "", Reason: reason}
	case primaryCapabilityID == euclorelurpic.CapabilityChatImplement:
		if classification.RequiresDeterministicStages || policy.RequireVerificationStep {
			return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.htn", Family: ExecutorFamilyHTN, RecipeID: "", Reason: reason}
		}
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.react", Family: ExecutorFamilyReact, RecipeID: "", Reason: reason}
	case primaryCapabilityID == euclorelurpic.CapabilityArchaeologyExplore:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.planner", Family: ExecutorFamilyPlanner, RecipeID: "", Reason: reason}
	case primaryCapabilityID == euclorelurpic.CapabilityArchaeologyCompilePlan:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.planner", Family: ExecutorFamilyPlanner, RecipeID: "", Reason: reason}
	case primaryCapabilityID == euclorelurpic.CapabilityArchaeologyImplement:
		if planBinding != nil && planBinding.IsLongRunning {
			return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.rewoo", Family: ExecutorFamilyRewoo, RecipeID: "", Reason: reason}
		}
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.planner", Family: ExecutorFamilyPlanner, RecipeID: "", Reason: reason}
	case primaryCapabilityID == euclorelurpic.CapabilityDebugInvestigateRepair:
		// NOTE: Now using blackboard instead of HTN for debug workflows.
		// Blackboard provides shared workspace context across knowledge sources,
		// preventing the context isolation bug that caused 17+ redundant file_list calls.
		//
		// The blackboard executor uses hypothesis-driven exploration with data-driven
		// knowledge source activation, which is better suited for debugging than HTN's
		// hierarchical task decomposition.
		//
		// TODO: Define debug-specific KnowledgeSources (FileExplorerKS, FaultLocalizerKS, etc.)
		// to fully utilize the blackboard architecture.
		// See: /docs/research/issue-blackboard-context-sharing.md
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.blackboard", Family: ExecutorFamilyBlackboard, RecipeID: "", Reason: "debug workflow with shared blackboard context"}
	case mode.ModeID == "review":
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.reflection", Family: ExecutorFamilyReflection, RecipeID: "review.fallback.reflection", Reason: "review mode fallback executor recipe"}
	case planBinding != nil && planBinding.IsLongRunning:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.rewoo", Family: ExecutorFamilyRewoo, RecipeID: "planning.fallback.rewoo", Reason: "fallback long-running plan-backed execution"}
	case mode.ModeID == "planning" || profile.ProfileID == "plan_stage_execute":
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.planner", Family: ExecutorFamilyPlanner, RecipeID: "planning.fallback.planner", Reason: "fallback planning execution"}
	case classification.RequiresDeterministicStages || policy.RequireVerificationStep:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.htn", Family: ExecutorFamilyHTN, RecipeID: "code.fallback.htn", Reason: "fallback deterministic decomposition"}
	default:
		return WorkUnitExecutorDescriptor{ExecutorID: "euclo.executor.react", Family: ExecutorFamilyReact, RecipeID: "code.fallback.react", Reason: "fallback react-style execution"}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
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

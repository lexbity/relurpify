package euclo

import (
	"context"
	"fmt"

	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

// plannerPlanCapability produces a plan artifact using multi-paradigm
// composition:
//   - PlannerAgent generates the initial plan (read-only tools via ToolScopeScoped)
//   - ReflectionAgent reviews the plan quality
//   - If reflection finds significant issues, the planner re-runs with
//     feedback (max 1 retry)
type plannerPlanCapability struct {
	env agentenv.AgentEnvironment
}

func (c *plannerPlanCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:planner.plan",
		Name:          "Planner-Plan",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "planning"},
		Annotations: map[string]any{
			"supported_profiles": []string{"plan_stage_execute", "edit_verify_repair"},
		},
	}
}

func (c *plannerPlanCapability) Contract() ArtifactContract {
	return ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindIntake, Required: true},
		},
		ProducedOutputs: []ArtifactKind{
			ArtifactKindPlan,
		},
	}
}

func (c *plannerPlanCapability) Eligible(_ ArtifactState, snapshot CapabilitySnapshot) EligibilityResult {
	if !snapshot.HasReadTools {
		return EligibilityResult{
			Eligible: false,
			Reason:   "read tools required for planning",
		}
	}
	return EligibilityResult{Eligible: true, Reason: "read tools available for planning"}
}

func (c *plannerPlanCapability) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	producerID := "euclo:planner.plan"

	planResult, err := c.runPlanner(ctx, env, "")
	if err != nil || planResult == nil || !planResult.Success {
		return ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "planning phase failed",
			FailureInfo: &CapabilityFailure{
				Code:         "plan_failed",
				Message:      errMsg(err, planResult),
				Recoverable:  true,
				FailedPhase:  "plan",
				ParadigmUsed: "planner",
			},
			RecoveryHint: &RecoveryHint{
				Strategy:          RecoveryStrategyParadigmSwitch,
				SuggestedParadigm: "react",
			},
		}
	}

	// Reflection review of the plan.
	feedback := c.reviewPlan(ctx, env, planResult)
	if feedback != "" {
		// Re-run planner with feedback (single retry).
		retryResult, retryErr := c.runPlanner(ctx, env, feedback)
		if retryErr == nil && retryResult != nil && retryResult.Success {
			planResult = retryResult
		}
		// If retry fails, use the original plan.
	}

	return ExecutionResult{
		Status:  ExecutionStatusCompleted,
		Summary: "plan generated and reviewed",
		Artifacts: []Artifact{
			{
				ID:         "planner_plan",
				Kind:       ArtifactKindPlan,
				Summary:    resultSummary(planResult),
				Payload:    planResult.Data,
				ProducerID: producerID,
				Status:     "produced",
			},
		},
	}
}

// runPlanner executes the PlannerAgent with optional feedback from a prior
// reflection review.
func (c *plannerPlanCapability) runPlanner(ctx context.Context, env ExecutionEnvelope, feedback string) (*core.Result, error) {
	planState := env.State.Clone()
	planAgent := plannerpkg.New(env.Environment)
	instruction := fmt.Sprintf("Create a detailed implementation plan for: %s", capTaskInstruction(env.Task))
	if feedback != "" {
		instruction = fmt.Sprintf("%s\n\nPrevious plan review feedback:\n%s", instruction, feedback)
	}
	planTask := &core.Task{
		ID:          "planner-plan",
		Instruction: instruction,
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	result, err := planAgent.Execute(ctx, planTask, planState)
	if err == nil {
		mergeStateArtifacts(env.State, planState)
	}
	return result, err
}

// reviewPlan uses ReflectionAgent to review the plan and returns feedback
// if significant issues are found. Returns empty string if the plan is good.
func (c *plannerPlanCapability) reviewPlan(ctx context.Context, env ExecutionEnvelope, _ *core.Result) string {
	reviewState := env.State.Clone()
	delegate := reactpkg.New(env.Environment)
	reviewAgent := reflectionpkg.New(env.Environment, delegate)
	reviewTask := &core.Task{
		ID:          "planner-review",
		Instruction: "Review the generated plan for completeness, feasibility, and potential issues.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}
	reviewResult, err := reviewAgent.Execute(ctx, reviewTask, reviewState)
	if err != nil || reviewResult == nil || !reviewResult.Success {
		return ""
	}
	// Check if the review result indicates issues.
	if reviewResult.Data != nil {
		if summary, ok := reviewResult.Data["summary"].(string); ok && summary != "" {
			return summary
		}
	}
	return ""
}

package execution

import (
	"context"
	"fmt"
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type GuidanceRequester func(context.Context, guidance.GuidanceRequest, string) guidance.GuidanceDecision

type PreflightCoordinator struct {
	Service         Service
	Plans           archaeoplans.Service
	RequestGuidance GuidanceRequester
}

func (c PreflightCoordinator) EvaluatePlanStepGate(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, graph *graphdb.Engine) (PreflightOutcome, error) {
	var gateState PreflightOutcome
	if step == nil {
		return gateState, nil
	}
	mutationEval, err := c.Service.EvaluateMutations(ctx, workflowIDForPlanTaskState(plan, task, state), handoffFromState(state), plan, step)
	if err != nil {
		return gateState, err
	}
	gateState.MutationEvaluation = mutationEval
	summary := mutationCheckpointSummary(archaeodomain.MutationCheckpointPreExecution, mutationEval, c.Service.now())
	gateState.MutationCheckpoint = &summary
	recordLiveMutationState(state, archaeodomain.MutationCheckpointPreExecution, mutationEval, c.Service.now())
	if mutationEval != nil {
		switch mutationEval.Disposition {
		case archaeodomain.DispositionInvalidateStep:
			gateState.InvalidatedStepIDs = []string{step.ID}
			gateState.ShouldInvalidate = true
			return gateState, fmt.Errorf("living plan step %s invalidated by archaeology mutation", step.ID)
		case archaeodomain.DispositionPauseForLearning:
			return gateState, fmt.Errorf("living plan step %s paused for learning due to archaeology mutation", step.ID)
		case archaeodomain.DispositionPauseForGuidance, archaeodomain.DispositionBlockExecution:
			return gateState, fmt.Errorf("living plan step %s blocked by archaeology mutation", step.ID)
		case archaeodomain.DispositionRequireReplan:
			return gateState, fmt.Errorf("active plan version requires replan before executing step %s", step.ID)
		}
	}
	assessment, err := c.Service.AssessPlanStep(ctx, task, state, step, graph)
	if err != nil {
		return gateState, err
	}
	if len(assessment.MissingSymbols) > 0 {
		gateState.InvalidatedStepIDs = c.Plans.ApplySymbolInvalidations(plan, step.ID, assessment.MissingSymbols)
		gateState.ShouldInvalidate = true
		if len(gateState.InvalidatedStepIDs) > 0 {
			_ = c.Plans.PersistAllSteps(ctx, plan)
		}
		return gateState, fmt.Errorf("living plan step %s blocked by missing required symbols: %s", step.ID, strings.Join(assessment.MissingSymbols, ", "))
	}
	if len(assessment.DriftedDependencies) > 0 {
		gateState.InvalidatedStepIDs = c.Plans.ApplyAnchorInvalidations(plan, step.ID, assessment.DriftedDependencies)
		gateState.ShouldInvalidate = true
	}
	if assessment.Confidence != step.ConfidenceScore {
		step.ConfidenceScore = assessment.Confidence
		gateState.ConfidenceUpdated = true
	}
	if assessment.Confidence < assessment.ConfidenceThreshold {
		decision := c.requestGuidance(ctx, guidance.GuidanceRequest{
			Kind:        guidance.GuidanceConfidence,
			Title:       "Low confidence on plan step",
			Description: fmt.Sprintf("Step %q has confidence %.2f (threshold %.2f).", step.ID, assessment.Confidence, assessment.ConfidenceThreshold),
			Choices: []guidance.GuidanceChoice{
				{ID: "proceed", Label: "Proceed", IsDefault: true},
				{ID: "skip", Label: "Skip this step"},
				{ID: "replan", Label: "Re-plan this step"},
			},
			TimeoutBehavior: c.Service.GuidanceTimeoutBehavior(guidance.GuidanceConfidence, len(step.Scope)),
			Context: map[string]any{
				"confidence":      assessment.Confidence,
				"threshold":       assessment.ConfidenceThreshold,
				"drifted_anchors": assessment.DriftedDependencies,
				"missing_symbols": assessment.MissingSymbols,
			},
		}, "proceed")
		if shortCircuitResult, shortCircuitErr, handled := c.Service.ApplyGuidanceDecision(plan, step, decision, "low confidence on plan step"); handled {
			gateState.Result = shortCircuitResult
			gateState.Err = shortCircuitErr
			return gateState, nil
		}
	}
	if assessment.BlastRadius != nil {
		decision := c.requestGuidance(ctx, guidance.GuidanceRequest{
			Kind:        guidance.GuidanceScopeExpansion,
			Title:       "Larger blast radius than planned",
			Description: fmt.Sprintf("Step %q affects %d symbols, above the planned scope of %d.", step.ID, assessment.BlastRadius.Actual, assessment.BlastRadius.Expected),
			Choices: []guidance.GuidanceChoice{
				{ID: "proceed", Label: "Proceed", IsDefault: true},
				{ID: "skip", Label: "Skip this step"},
				{ID: "replan", Label: "Re-plan this step"},
			},
			TimeoutBehavior: c.Service.GuidanceTimeoutBehavior(guidance.GuidanceScopeExpansion, assessment.BlastRadius.Actual),
			Context: map[string]any{
				"expected_symbols": assessment.BlastRadius.Expected,
				"actual_symbols":   assessment.BlastRadius.Actual,
				"affected":         assessment.BlastRadius.Affected,
			},
		}, "proceed")
		if shortCircuitResult, shortCircuitErr, handled := c.Service.ApplyGuidanceDecision(plan, step, decision, "blast radius larger than planned"); handled {
			gateState.Result = shortCircuitResult
			gateState.Err = shortCircuitErr
			return gateState, nil
		}
	}
	if !assessment.HasEvidence && step.EvidenceGate != nil && step.EvidenceGate.MaxTotalLoss > 0 {
		return gateState, fmt.Errorf("living plan step %s blocked by missing grounding evidence", step.ID)
	}
	if step.EvidenceGate != nil && !frameworkplan.EvidenceGateAllows(step.EvidenceGate, assessment.Evidence, assessment.ActiveAnchors, assessment.AvailableSymbolMap) {
		if len(step.EvidenceGate.RequiredAnchors) > 0 {
			return gateState, fmt.Errorf("living plan step %s blocked by inactive required anchors", step.ID)
		}
		if step.EvidenceGate.MaxTotalLoss > 0 {
			return gateState, fmt.Errorf("living plan step %s blocked by evidence derivation loss", step.ID)
		}
	}
	return gateState, nil
}

func (c PreflightCoordinator) requestGuidance(ctx context.Context, req guidance.GuidanceRequest, fallback string) guidance.GuidanceDecision {
	if c.RequestGuidance != nil {
		return c.RequestGuidance(ctx, req, fallback)
	}
	return c.Service.RequestGuidance(ctx, req, fallback)
}

func handoffFromState(state *core.Context) *archaeodomain.ExecutionHandoff {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.execution_handoff")
	if !ok || raw == nil {
		return nil
	}
	if typed, ok := raw.(*archaeodomain.ExecutionHandoff); ok {
		return typed
	}
	if typed, ok := raw.(archaeodomain.ExecutionHandoff); ok {
		return &typed
	}
	return nil
}

func workflowIDForPlanTaskState(plan *frameworkplan.LivingPlan, task *core.Task, state *core.Context) string {
	if plan != nil && strings.TrimSpace(plan.WorkflowID) != "" {
		return strings.TrimSpace(plan.WorkflowID)
	}
	if state != nil {
		if workflowID := strings.TrimSpace(state.GetString("euclo.workflow_id")); workflowID != "" {
			return workflowID
		}
	}
	if task != nil && task.Context != nil {
		if workflowID, ok := task.Context["workflow_id"].(string); ok {
			return strings.TrimSpace(workflowID)
		}
	}
	return ""
}

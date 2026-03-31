package execution

import (
	"context"
	"fmt"
	"strings"
	"time"

	archaeodeferred "github.com/lexcodex/relurpify/archaeo/deferred"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeorequests "github.com/lexcodex/relurpify/archaeo/requests"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type LiveMutationCoordinator struct {
	Service         Service
	Plans           archaeoplans.Service
	RequestGuidance GuidanceRequester
}

func (c LiveMutationCoordinator) CheckpointExecution(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (*MutationEvaluation, error) {
	return c.CheckpointExecutionAt(ctx, archaeodomain.MutationCheckpointPreVerification, task, state, plan, step)
}

func (c LiveMutationCoordinator) CheckpointExecutionAt(ctx context.Context, checkpoint archaeodomain.MutationCheckpoint, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (*MutationEvaluation, error) {
	if plan == nil || step == nil {
		recordLiveMutationState(state, checkpoint, nil, c.Service.now())
		return nil, nil
	}
	workflowID := workflowIDForPlanTaskState(plan, task, state)
	eval, err := c.Service.EvaluateMutations(ctx, workflowID, handoffFromState(state), plan, step)
	if err != nil || eval == nil {
		recordLiveMutationState(state, checkpoint, eval, c.Service.now())
		return eval, err
	}
	recordLiveMutationState(state, checkpoint, eval, c.Service.now())
	switch eval.Disposition {
	case archaeodomain.DispositionContinue:
		return eval, nil
	case archaeodomain.DispositionContinueOnStalePlan:
		if state != nil {
			state.Set("euclo.execution_on_stale_plan", true)
			state.Set("euclo.execution_requires_replan", false)
		}
		return eval, nil
	case archaeodomain.DispositionPauseForGuidance:
		c.createDeferredDraftRequest(ctx, task, state, plan, step, eval)
		return eval, c.resolveGuidanceMutation(ctx, step, eval)
	case archaeodomain.DispositionInvalidateStep:
		c.createDeferredDraftRequest(ctx, task, state, plan, step, eval)
		c.Plans.RecordBlockedStep(plan, step, liveMutationReason(step, eval, "step invalidated by archaeology mutation"), true)
		_ = c.Plans.PersistStep(ctx, plan, step.ID)
		return eval, fmt.Errorf("living plan step %s invalidated by execution-time archaeology mutation", step.ID)
	case archaeodomain.DispositionPauseForLearning:
		c.createDeferredDraftRequest(ctx, task, state, plan, step, eval)
		c.Plans.RecordBlockedStep(plan, step, liveMutationReason(step, eval, "paused for learning"), false)
		_ = c.Plans.PersistStep(ctx, plan, step.ID)
		return eval, fmt.Errorf("living plan step %s paused for learning due to execution-time archaeology mutation", step.ID)
	case archaeodomain.DispositionBlockExecution:
		c.createDeferredDraftRequest(ctx, task, state, plan, step, eval)
		c.Plans.RecordBlockedStep(plan, step, liveMutationReason(step, eval, "execution blocked"), false)
		_ = c.Plans.PersistStep(ctx, plan, step.ID)
		return eval, fmt.Errorf("living plan step %s blocked by execution-time archaeology mutation", step.ID)
	case archaeodomain.DispositionRequireReplan:
		c.createDeferredDraftRequest(ctx, task, state, plan, step, eval)
		if state != nil {
			state.Set("euclo.execution_requires_replan", true)
		}
		c.Plans.RecordBlockedStep(plan, step, liveMutationReason(step, eval, "replan required"), false)
		_ = c.Plans.PersistStep(ctx, plan, step.ID)
		return eval, fmt.Errorf("active plan version requires replan due to execution-time archaeology mutation on step %s", step.ID)
	default:
		return eval, nil
	}
}

func (c LiveMutationCoordinator) createDeferredDraftRequest(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep, eval *MutationEvaluation) {
	if c.Service.WorkflowStore == nil || plan == nil || step == nil || eval == nil {
		return
	}
	workspaceID := strings.TrimSpace(workspaceIDForTaskState(task, state))
	workflowID := workflowIDForPlanTaskState(plan, task, state)
	if workspaceID == "" || workflowID == "" {
		return
	}
	ambiguityKey := deferredAmbiguityKey(step, eval)
	request, err := (archaeorequests.Service{Store: c.Service.WorkflowStore}).Create(ctx, archaeorequests.CreateInput{
		WorkflowID:     workflowID,
		PlanID:         plan.ID,
		PlanVersion:    planVersionPtr(plan.Version),
		Kind:           archaeodomain.RequestPlanReformation,
		Title:          "Deferred draft requested from execution ambiguity",
		Description:    liveMutationReason(step, eval, "execution ambiguity requires deferred draft"),
		RequestedBy:    "archaeo.execution.live",
		CorrelationID:  fmt.Sprintf("deferred:%s:%s", workflowID, ambiguityKey),
		IdempotencyKey: fmt.Sprintf("deferred:%s:%d:%s", workflowID, plan.Version, ambiguityKey),
		SubjectRefs:    append([]string{step.ID}, mutationIDs(eval.RelevantMutations)...),
		Input: map[string]any{
			"workspace_id":   workspaceID,
			"step_id":        step.ID,
			"ambiguity_key":  ambiguityKey,
			"mutation_ids":   mutationIDs(eval.RelevantMutations),
			"disposition":    string(eval.Disposition),
			"highest_impact": string(eval.HighestImpact),
		},
		BasedOnRevision: stateValue(state, "euclo.based_on_revision"),
	})
	if err != nil || request == nil {
		return
	}
	_, _ = (archaeodeferred.Service{Store: c.Service.WorkflowStore}).CreateOrUpdate(ctx, archaeodeferred.CreateInput{
		WorkspaceID:  workspaceID,
		WorkflowID:   workflowID,
		PlanID:       plan.ID,
		PlanVersion:  planVersionPtr(plan.Version),
		RequestID:    request.ID,
		AmbiguityKey: ambiguityKey,
		Title:        "Deferred draft pending review",
		Description:  liveMutationReason(step, eval, "long-running execution discovered ambiguity"),
		Metadata: map[string]any{
			"step_id":        step.ID,
			"mutation_ids":   mutationIDs(eval.RelevantMutations),
			"disposition":    string(eval.Disposition),
			"highest_impact": string(eval.HighestImpact),
		},
	})
}

func deferredAmbiguityKey(step *frameworkplan.PlanStep, eval *MutationEvaluation) string {
	if step == nil {
		return "execution-ambiguity"
	}
	parts := []string{strings.TrimSpace(step.ID), string(eval.Disposition)}
	parts = append(parts, mutationIDs(eval.RelevantMutations)...)
	return strings.Join(parts, ":")
}

func workspaceIDForTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString("euclo.workspace")); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(fmt.Sprint(task.Context["workspace"])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func stateValue(state *core.Context, key string) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString(key))
}

func planVersionPtr(version int) *int {
	if version <= 0 {
		return nil
	}
	copy := version
	return &copy
}

func (c LiveMutationCoordinator) resolveGuidanceMutation(ctx context.Context, step *frameworkplan.PlanStep, eval *MutationEvaluation) error {
	if step == nil || eval == nil {
		return nil
	}
	req := guidance.GuidanceRequest{
		Kind:        guidance.GuidanceContradiction,
		Title:       "Execution-time archaeology contradiction",
		Description: liveMutationReason(step, eval, "contradiction detected during execution"),
		Choices: []guidance.GuidanceChoice{
			{ID: "proceed", Label: "Proceed", IsDefault: true},
			{ID: "replan", Label: "Re-plan this step"},
			{ID: "stop", Label: "Stop execution"},
		},
		TimeoutBehavior: c.Service.GuidanceTimeoutBehavior(guidance.GuidanceContradiction, blastRadiusCount(eval)),
		Context: map[string]any{
			"step_id":               step.ID,
			"mutation_disposition":  string(eval.Disposition),
			"mutation_impact":       string(eval.HighestImpact),
			"relevant_mutation_ids": mutationIDs(eval.RelevantMutations),
		},
	}
	decision := c.requestGuidance(ctx, req, "stop")
	switch decision.ChoiceID {
	case "proceed":
		return nil
	case "replan":
		return fmt.Errorf("guidance requested replan after execution-time archaeology contradiction on step %s", step.ID)
	default:
		return fmt.Errorf("execution stopped for step %s after execution-time archaeology contradiction", step.ID)
	}
}

func (c LiveMutationCoordinator) requestGuidance(ctx context.Context, req guidance.GuidanceRequest, fallback string) guidance.GuidanceDecision {
	if c.RequestGuidance != nil {
		return c.RequestGuidance(ctx, req, fallback)
	}
	return c.Service.RequestGuidance(ctx, req, fallback)
}

func recordLiveMutationState(state *core.Context, checkpoint archaeodomain.MutationCheckpoint, eval *MutationEvaluation, now time.Time) {
	if state == nil {
		return
	}
	summary := mutationCheckpointSummary(checkpoint, eval, now)
	if eval != nil {
		state.Set("euclo.execution_mutation_evaluation", *eval)
		state.Set("euclo.execution_mutation_disposition", string(eval.Disposition))
		state.Set("euclo.execution_mutation_impact", string(eval.HighestImpact))
		state.Set("euclo.execution_requires_replan", eval.RequireReplan)
		state.Set("euclo.execution_on_stale_plan", eval.ContinueOnStale)
	}
	state.Set("euclo.execution_mutation_latest_checkpoint", string(checkpoint))
	state.Set("euclo.execution_mutation_checkpoint_summary", summary)
	history := MutationCheckpointSummaries(state)
	history = append(history, summary)
	state.Set("euclo.execution_mutation_checkpoints", history)
}

func mutationCheckpointSummary(checkpoint archaeodomain.MutationCheckpoint, eval *MutationEvaluation, now time.Time) archaeodomain.MutationCheckpointSummary {
	summary := archaeodomain.MutationCheckpointSummary{
		Checkpoint:    checkpoint,
		Disposition:   archaeodomain.DispositionContinue,
		HighestImpact: archaeodomain.ImpactInformational,
		CreatedAt:     now.UTC(),
	}
	if eval == nil {
		return summary
	}
	summary.WorkflowID = eval.WorkflowID
	summary.HandoffRef = eval.HandoffRef
	summary.ActiveStepID = eval.ActiveStepID
	summary.Disposition = eval.Disposition
	summary.HighestImpact = eval.HighestImpact
	summary.Blocking = eval.Blocking
	summary.RequireReplan = eval.RequireReplan
	summary.ContinueOnStale = eval.ContinueOnStale
	summary.MutationIDs = mutationIDs(eval.RelevantMutations)
	return summary
}

func MutationCheckpointSummaries(state *core.Context) []archaeodomain.MutationCheckpointSummary {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.execution_mutation_checkpoints")
	if !ok || raw == nil {
		return nil
	}
	if typed, ok := raw.([]archaeodomain.MutationCheckpointSummary); ok {
		return append([]archaeodomain.MutationCheckpointSummary(nil), typed...)
	}
	return nil
}

func blastRadiusCount(eval *MutationEvaluation) int {
	if eval == nil {
		return 0
	}
	count := 0
	for _, mutation := range eval.RelevantMutations {
		count += mutation.BlastRadius.EstimatedCount
	}
	if count > 0 {
		return count
	}
	return len(eval.RelevantMutations)
}

func mutationIDs(values []archaeodomain.MutationEvent) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value.ID); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func liveMutationReason(step *frameworkplan.PlanStep, eval *MutationEvaluation, prefix string) string {
	if step == nil {
		return prefix
	}
	if eval == nil || len(eval.RelevantMutations) == 0 {
		return fmt.Sprintf("%s for step %s", prefix, step.ID)
	}
	parts := make([]string, 0, len(eval.RelevantMutations))
	for _, mutation := range eval.RelevantMutations {
		description := strings.TrimSpace(mutation.Description)
		if description == "" {
			description = strings.TrimSpace(mutation.SourceRef)
		}
		if description != "" {
			parts = append(parts, description)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s for step %s", prefix, step.ID)
	}
	return fmt.Sprintf("%s for step %s: %s", prefix, step.ID, strings.Join(parts, "; "))
}

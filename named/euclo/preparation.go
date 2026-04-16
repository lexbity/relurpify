package euclo

import (
	"context"
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	archaeoprojections "github.com/lexcodex/relurpify/archaeo/projections"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statebus"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statekeys"
)

type executionPreparation struct {
	workflowID      string
	readBundle      *executionReadBundle
	livingPlan      *frameworkplan.LivingPlan
	activeStep      *frameworkplan.PlanStep
	preflightResult *core.Result
	err             error
	summaryFastPath bool
	skipReason      string
	preparationNote string
}

type executionReadBundle struct {
	workflowID    string
	learningQueue *archaeoprojections.LearningQueueProjection
	activePlan    *archaeoprojections.ActivePlanProjection
}

func (a *Agent) shortCircuitResult(state *core.Context, prep executionPreparation) *core.Result {
	data := map[string]any{}
	if state != nil {
		if raw, ok := statebus.GetAny(state, statekeys.KeyLearningQueue); ok && raw != nil {
			data["learning_queue"] = raw
		}
		if raw, ok := statebus.GetAny(state, statekeys.KeyPendingLearningIDs); ok && raw != nil {
			data["pending_learning_ids"] = raw
		}
		if raw, ok := statebus.GetAny(state, statekeys.KeyPendingGuidanceIDs); ok && raw != nil {
			data["pending_guidance_ids"] = raw
		}
		if raw, ok := statebus.GetAny(state, statekeys.KeyPhaseState); ok && raw != nil {
			data["phase_state"] = raw
		} else if raw, ok := statebus.GetAny(state, statekeys.KeyArchaeoPhaseState); ok && raw != nil {
			data["phase_state"] = raw
		}
	}
	message := "no active plan step"
	if ids, ok := data["pending_learning_ids"]; ok && ids != nil {
		message = "pending learning requires review before execution"
	}
	if prep.skipReason != "" {
		message = prep.skipReason
	}
	if prep.summaryFastPath {
		if prep.skipReason == "" {
			message = "summary/status request completed without active execution step"
		}
	}
	return &core.Result{
		Success: true,
		Data:    data,
		Metadata: map[string]any{
			"summary": message,
		},
	}
}

func shouldShortCircuitExecution(state *core.Context) bool {
	if state == nil {
		return false
	}
	raw, ok := statebus.GetAny(state, statekeys.KeyArchaeoPhaseState)
	if !ok || raw == nil {
		return false
	}
	phaseState, ok := raw.(*archaeodomain.WorkflowPhaseState)
	if !ok || phaseState == nil {
		if typed, ok := raw.(archaeodomain.WorkflowPhaseState); ok {
			phaseState = &typed
		} else {
			return false
		}
	}
	switch phaseState.CurrentPhase {
	case archaeodomain.PhaseIntentElicitation, archaeodomain.PhaseSurfacing:
		return true
	default:
		return false
	}
}

func hasTerminalExecutionPreparation(prep executionPreparation) bool {
	return prep.summaryFastPath || prep.skipReason != ""
}

func (a *Agent) prepareExecution(ctx context.Context, task *core.Task, state *core.Context, classification eucloruntime.TaskClassification, profile euclotypes.ExecutionProfileSelection) executionPreparation {
	prep := executionPreparation{
		workflowID: workflowIDFromTaskState(task, state),
	}
	if !a.shouldUseSummaryStatusFastPath(task, classification, profile) {
		return a.finishExecutionPreparation(ctx, task, state, prep)
	}
	if !taskHasExplicitWorkflow(task) {
		return a.markSummaryExecutionWithoutWorkflow(prep)
	}
	prep = a.prepareSummaryFastPathExecution(ctx, task, state, prep)
	if prep.summaryFastPath {
		return prep
	}
	return a.finishExecutionPreparation(ctx, task, state, prep)
}

func (a *Agent) prepareSummaryFastPathExecution(ctx context.Context, task *core.Task, state *core.Context, prep executionPreparation) executionPreparation {
	bundle, ok := a.loadExecutionReadBundle(ctx, prep.workflowID)
	if !ok {
		return prep
	}
	return a.finalizeSummaryFastPathExecution(task, state, prep, bundle)
}

func (a *Agent) finalizeSummaryFastPathExecution(task *core.Task, state *core.Context, prep executionPreparation, bundle *executionReadBundle) executionPreparation {
	prep.readBundle = bundle
	a.seedExecutionReadBundleState(state, bundle)
	if bundleHasBlockingWork(task, bundle) {
		return prep
	}
	return a.markSummaryExecutionFromCachedState(prep)
}

func (a *Agent) markSummaryExecutionWithoutWorkflow(prep executionPreparation) executionPreparation {
	prep.summaryFastPath = true
	prep.skipReason = "summary/status request completed without explicit workflow"
	return prep
}

func (a *Agent) markSummaryExecutionFromCachedState(prep executionPreparation) executionPreparation {
	prep.summaryFastPath = true
	prep.skipReason = "summary/status request completed from cached execution state"
	return prep
}

func (a *Agent) finishExecutionPreparation(ctx context.Context, task *core.Task, state *core.Context, prep executionPreparation) executionPreparation {
	prep.livingPlan, prep.activeStep, prep.preflightResult, prep.err, prep.skipReason, prep.preparationNote = a.prepareLivingPlan(ctx, task, state)
	return prep
}

func (a *Agent) prepareLivingPlan(ctx context.Context, task *core.Task, state *core.Context) (*frameworkplan.LivingPlan, *frameworkplan.PlanStep, *core.Result, error, string, string) {
	if note := a.executionPreparationNote(task, state); note != "" {
		return nil, nil, nil, nil, "", note
	}
	workflowID := workflowIDFromTaskState(task, state)
	result := a.archaeologyService().PrepareLivingPlan(ctx, task, state, workflowID)
	return result.Plan, result.Step, result.Result, result.Err, "", ""
}

func (a *Agent) executionPreparationNote(task *core.Task, state *core.Context) string {
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		return "execution preparation note: workflow id unavailable"
	}
	if a == nil || a.PlanStore == nil {
		return "execution preparation note: plan store unavailable"
	}
	return ""
}

func (a *Agent) shouldUseSummaryStatusFastPath(task *core.Task, classification eucloruntime.TaskClassification, profile euclotypes.ExecutionProfileSelection) bool {
	if task == nil {
		return false
	}
	if profile.ProfileID != "plan_stage_execute" {
		return false
	}
	if classification.RequiresEvidenceBeforeMutation {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(task.Instruction))
	for _, token := range []string{"summary", "summarize", "status", "status update", "current status", "report status"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func (a *Agent) loadExecutionReadBundle(ctx context.Context, workflowID string) (*executionReadBundle, bool) {
	if a == nil || strings.TrimSpace(workflowID) == "" || a.WorkflowStore == nil {
		return nil, false
	}
	svc := a.projectionService()
	if svc == nil || svc.Store == nil {
		return nil, false
	}
	learningQueue, err := svc.LearningQueue(ctx, workflowID)
	if err != nil {
		return nil, false
	}
	activePlan, err := svc.ActivePlan(ctx, workflowID)
	if err != nil {
		return nil, false
	}
	return &executionReadBundle{
		workflowID:    workflowID,
		learningQueue: learningQueue,
		activePlan:    activePlan,
	}, true
}

func (a *Agent) seedExecutionReadBundleState(state *core.Context, bundle *executionReadBundle) {
	if state == nil || bundle == nil {
		return
	}
		statebus.SetAny(state, statekeys.KeyExecutionReadBundle, bundle)
	if bundle.activePlan != nil {
		if bundle.activePlan.PhaseState != nil {
			statebus.SetAny(state, statekeys.KeyPhaseState, bundle.activePlan.PhaseState)
		}
		if bundle.activePlan.ActivePlanVersion != nil {
			statebus.SetAny(state, statekeys.KeyActivePlanVersion, bundle.activePlan.ActivePlanVersion)
			statebus.SetAny(state, statekeys.KeyPreloadedActivePlanVersion, bundle.activePlan.ActivePlanVersion)
			statebus.SetAny(state, statekeys.KeyLivingPlan, &bundle.activePlan.ActivePlanVersion.Plan)
		}
	}
	if bundle.learningQueue != nil && len(bundle.learningQueue.PendingLearning) > 0 {
		statebus.SetAny(state, statekeys.KeyLearningQueue, bundle.learningQueue.PendingLearning)
		statebus.SetAny(state, statekeys.KeyPreloadedPendingLearning, bundle.learningQueue.PendingLearning)
		ids := make([]string, 0, len(bundle.learningQueue.PendingLearning))
		for _, interaction := range bundle.learningQueue.PendingLearning {
			ids = append(ids, interaction.ID)
		}
		statebus.SetAny(state, statekeys.KeyPendingLearningIDs, ids)
		statebus.SetAny(state, statekeys.KeyBlockingLearningIDs, append([]string(nil), bundle.learningQueue.BlockingLearning...))
		statebus.SetAny(state, statekeys.KeyPreloadedBlockingLearningIDs, append([]string(nil), bundle.learningQueue.BlockingLearning...))
	}
	if bundle.learningQueue != nil && len(bundle.learningQueue.PendingGuidanceIDs) > 0 {
		statebus.SetAny(state, statekeys.KeyPendingGuidanceIDs, append([]string(nil), bundle.learningQueue.PendingGuidanceIDs...))
	}
}

func bundleHasBlockingWork(task *core.Task, bundle *executionReadBundle) bool {
	if bundle == nil {
		return false
	}
	if bundle.learningQueue != nil && len(bundle.learningQueue.PendingGuidanceIDs) > 0 {
		return true
	}
	if bundle.learningQueue != nil && len(bundle.learningQueue.BlockingLearning) > 0 {
		return true
	}
	if bundle.activePlan == nil || bundle.activePlan.ActivePlanVersion == nil {
		return false
	}
	return archaeoplans.ActiveStep(task, &bundle.activePlan.ActivePlanVersion.Plan) != nil
}

func taskHasExplicitWorkflow(task *core.Task) bool {
	if task == nil || task.Context == nil {
		return false
	}
	return strings.TrimSpace(stringValue(task.Context["workflow_id"])) != ""
}

func shouldHydratePersistedArtifacts(task *core.Task, state *core.Context, envelope eucloruntime.TaskEnvelope) bool {
	if len(envelope.PreviousArtifactKinds) > 0 {
		return true
	}
	if state != nil {
	if strings.TrimSpace(statebus.GetString(state, statekeys.KeyRunID)) != "" {
			return true
		}
	}
	if task == nil || task.Context == nil {
		return false
	}
	if strings.TrimSpace(stringValue(task.Context["run_id"])) != "" {
		return true
	}
	if _, ok := task.Context["euclo.interaction_state"]; ok {
		return true
	}
	return false
}

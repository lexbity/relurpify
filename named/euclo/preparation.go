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
)

type executionPreparation struct {
	workflowID      string
	readBundle      *executionReadBundle
	livingPlan      *frameworkplan.LivingPlan
	activeStep      *frameworkplan.PlanStep
	preflightResult *core.Result
	err             error
	summaryFastPath bool
}

type executionReadBundle struct {
	workflowID    string
	learningQueue *archaeoprojections.LearningQueueProjection
	activePlan    *archaeoprojections.ActivePlanProjection
}

func (a *Agent) shortCircuitResult(state *core.Context, prep executionPreparation) *core.Result {
	data := map[string]any{}
	if state != nil {
		if raw, ok := state.Get("euclo.learning_queue"); ok && raw != nil {
			data["learning_queue"] = raw
		}
		if raw, ok := state.Get("euclo.pending_learning_ids"); ok && raw != nil {
			data["pending_learning_ids"] = raw
		}
		if raw, ok := state.Get("euclo.pending_guidance_ids"); ok && raw != nil {
			data["pending_guidance_ids"] = raw
		}
		if raw, ok := state.Get("euclo.phase_state"); ok && raw != nil {
			data["phase_state"] = raw
		} else if raw, ok := state.Get("euclo.archaeo_phase_state"); ok && raw != nil {
			data["phase_state"] = raw
		}
	}
	message := "no active plan step"
	if ids, ok := data["pending_learning_ids"]; ok && ids != nil {
		message = "pending learning requires review before execution"
	}
	if prep.summaryFastPath {
		message = "summary/status request completed without active execution step"
	}
	return &core.Result{
		Success: true,
		Data:    data,
		Metadata: map[string]any{
			"summary": message,
		},
	}
}

func shouldShortCircuitExecution(prep executionPreparation, state *core.Context) bool {
	if prep.summaryFastPath {
		return true
	}
	if state == nil {
		return false
	}
	raw, ok := state.Get("euclo.archaeo_phase_state")
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

func (a *Agent) prepareExecution(ctx context.Context, task *core.Task, state *core.Context, classification eucloruntime.TaskClassification, profile euclotypes.ExecutionProfileSelection) executionPreparation {
	prep := executionPreparation{
		workflowID: workflowIDFromTaskState(task, state),
	}
	if a.shouldUseSummaryStatusFastPath(task, classification, profile) {
		if !taskHasExplicitWorkflow(task) {
			prep.summaryFastPath = true
			return prep
		}
		if bundle, ok := a.loadExecutionReadBundle(ctx, prep.workflowID); ok {
			prep.readBundle = bundle
			a.seedExecutionReadBundleState(state, bundle)
			if !bundleHasBlockingWork(task, bundle) {
				prep.summaryFastPath = true
				return prep
			}
		}
	}
	prep.livingPlan, prep.activeStep, prep.preflightResult, prep.err = a.prepareLivingPlan(ctx, task, state)
	return prep
}

func (a *Agent) prepareLivingPlan(ctx context.Context, task *core.Task, state *core.Context) (*frameworkplan.LivingPlan, *frameworkplan.PlanStep, *core.Result, error) {
	if a == nil || a.PlanStore == nil {
		return nil, nil, nil, nil
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		return nil, nil, nil, nil
	}
	result := a.archaeologyService().PrepareLivingPlan(ctx, task, state, workflowID)
	return result.Plan, result.Step, result.Result, result.Err
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
	state.Set("euclo.execution_read_bundle", bundle)
	if bundle.activePlan != nil {
		if bundle.activePlan.PhaseState != nil {
			state.Set("euclo.phase_state", bundle.activePlan.PhaseState)
		}
		if bundle.activePlan.ActivePlanVersion != nil {
			state.Set("euclo.active_plan_version", bundle.activePlan.ActivePlanVersion)
			state.Set("euclo.preloaded_active_plan_version", bundle.activePlan.ActivePlanVersion)
			state.Set("euclo.living_plan", &bundle.activePlan.ActivePlanVersion.Plan)
		}
	}
	if bundle.learningQueue != nil && len(bundle.learningQueue.PendingLearning) > 0 {
		state.Set("euclo.learning_queue", bundle.learningQueue.PendingLearning)
		state.Set("euclo.preloaded_pending_learning", bundle.learningQueue.PendingLearning)
		ids := make([]string, 0, len(bundle.learningQueue.PendingLearning))
		for _, interaction := range bundle.learningQueue.PendingLearning {
			ids = append(ids, interaction.ID)
		}
		state.Set("euclo.pending_learning_ids", ids)
		state.Set("euclo.blocking_learning_ids", append([]string(nil), bundle.learningQueue.BlockingLearning...))
		state.Set("euclo.preloaded_blocking_learning_ids", append([]string(nil), bundle.learningQueue.BlockingLearning...))
	}
	if bundle.learningQueue != nil && len(bundle.learningQueue.PendingGuidanceIDs) > 0 {
		state.Set("euclo.pending_guidance_ids", append([]string(nil), bundle.learningQueue.PendingGuidanceIDs...))
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
		if strings.TrimSpace(state.GetString("euclo.run_id")) != "" {
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

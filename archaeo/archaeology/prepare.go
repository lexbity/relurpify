package archaeology

import (
	"context"
	"log"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
)

func (s Service) PrepareLivingPlan(ctx context.Context, task *core.Task, state *core.Context, workflowID string) PrepareResult {
	if s.Plans.Store == nil || workflowID == "" {
		return PrepareResult{}
	}
	if err := s.prepareExploration(ctx, task, state, workflowID); err != nil {
		s.persistPhase(ctx, task, state, archaeodomain.PhaseBlocked, err.Error(), nil)
		return PrepareResult{Err: err}
	}
	active, err := s.loadActiveContext(ctx, task, state, workflowID)
	if err != nil || active == nil || active.Plan == nil {
		s.persistPhase(ctx, task, state, archaeodomain.PhaseArchaeology, errorString(err), nil)
		return PrepareResult{Err: err}
	}
	plan := active.Plan
	if version, err := s.Plans.EnsureActiveVersion(ctx, workflowID, plan, archaeoplans.DraftVersionInput{
		WorkflowID:             workflowID,
		DerivedFromExploration: state.GetString("euclo.active_exploration_id"),
		BasedOnRevision:        basedOnRevisionFromTask(task, state),
		SemanticSnapshotRef:    state.GetString("euclo.active_exploration_snapshot_id"),
	}); err == nil && version != nil {
		state.Set("euclo.active_plan_version", version.Version)
		plan.Version = version.Version
	} else if err != nil {
		s.persistPhase(ctx, task, state, archaeodomain.PhaseBlocked, err.Error(), nil)
		return PrepareResult{Err: err}
	}
	state.Set("euclo.living_plan", plan)
	refresh, learningErr := s.preloadRefresh(ctx, task, state, workflowID)
	if learningErr != nil {
		s.persistPhase(ctx, task, state, archaeodomain.PhaseBlocked, learningErr.Error(), nil)
		return PrepareResult{Plan: plan, Err: learningErr}
	}
	blockingLearning, totalLearning, learningErr := s.syncLearning(ctx, task, state, workflowID, refresh)
	if learningErr != nil {
		s.persistPhase(ctx, task, state, archaeodomain.PhaseBlocked, learningErr.Error(), nil)
		return PrepareResult{Plan: plan, Err: learningErr}
	}
	step := active.Step
	if step == nil {
		if blockingLearning > 0 {
			s.persistPhase(ctx, task, state, archaeodomain.PhaseIntentElicitation, "", nil)
			return PrepareResult{Plan: plan}
		}
		if totalLearning > 0 {
			state.Set("euclo.has_nonblocking_learning", true)
		}
		s.persistPhase(ctx, task, state, archaeodomain.PhaseSurfacing, "", nil)
		return PrepareResult{Plan: plan}
	}
	if s.ResetDoom != nil {
		s.ResetDoom()
	}
	gateState, gateErr := s.evaluateGate(ctx, task, state, plan, step)
	if gateErr != nil {
		log.Printf("euclo: blocking plan step %s: %v", step.ID, gateErr)
		state.Set("euclo.living_plan", plan)
		result, err := s.Plans.PersistPreflightBlocked(ctx, plan, step, gateErr.Error(), gateState.ShouldInvalidate, gateState.InvalidatedStepIDs)
		if err != nil {
			gateErr = err
		}
		s.persistPhase(ctx, task, state, archaeodomain.PhaseBlocked, gateErr.Error(), step)
		return PrepareResult{Plan: plan, Result: result, Err: gateErr}
	}
	if gateState.Result != nil {
		state.Set("euclo.living_plan", plan)
		result, err := s.Plans.PersistPreflightShortCircuit(ctx, plan, step, gateState.Result, gateState.Err)
		if err != nil {
			gateState.Err = err
		}
		if gateState.Err != nil {
			s.persistPhase(ctx, task, state, archaeodomain.PhaseBlocked, gateState.Err.Error(), step)
		}
		return PrepareResult{Plan: plan, Result: result, Err: gateState.Err}
	}
	if gateState.ConfidenceUpdated {
		state.Set("euclo.living_plan", plan)
		_ = s.Plans.PersistPreflightConfidenceUpdate(ctx, plan, step)
	}
	state.Set("euclo.current_plan_step_id", step.ID)
	return PrepareResult{Plan: plan, Step: step}
}

func (s Service) loadActiveContext(ctx context.Context, task *core.Task, state *core.Context, workflowID string) (*archaeoplans.ActiveContext, error) {
	if state != nil {
		if raw, ok := state.Get("euclo.preloaded_active_plan_version"); ok && raw != nil {
			if version, ok := raw.(*archaeodomain.VersionedLivingPlan); ok && version != nil && version.WorkflowID == workflowID {
				plan := version.Plan
				plan.Version = version.Version
				return &archaeoplans.ActiveContext{
					WorkflowID: workflowID,
					Plan:       &plan,
					Step:       archaeoplans.ActiveStep(task, &plan),
				}, nil
			}
		}
	}
	return s.Plans.LoadActiveContext(ctx, workflowID, task)
}

func (s Service) prepareExploration(ctx context.Context, task *core.Task, state *core.Context, workflowID string) error {
	workspaceID := workspaceIDFromTaskState(task, state)
	if workspaceID == "" {
		return nil
	}
	session, err := s.EnsureExplorationSession(ctx, workflowID, workspaceID, basedOnRevisionFromTask(task, state))
	if err != nil || session == nil {
		return err
	}
	state.Set("euclo.active_exploration_id", session.ID)
	state.Set("euclo.exploration_recompute_required", session.RecomputeRequired)
	if session.StaleReason != "" {
		state.Set("euclo.exploration_stale_reason", session.StaleReason)
	}
	snapshot, err := s.CreateExplorationSnapshot(ctx, session, SnapshotInput{
		WorkflowID:          workflowID,
		WorkspaceID:         workspaceID,
		BasedOnRevision:     basedOnRevisionFromTask(task, state),
		OpenLearningIDs:     stringSliceFromState(state, "euclo.pending_learning_ids"),
		Summary:             taskInstruction(task),
		SemanticSnapshotRef: state.GetString("euclo.semantic_snapshot_ref"),
	})
	if err != nil || snapshot == nil {
		return err
	}
	state.Set("euclo.active_exploration_snapshot_id", snapshot.ID)
	state.Set("euclo.based_on_revision", snapshot.BasedOnRevision)
	return nil
}

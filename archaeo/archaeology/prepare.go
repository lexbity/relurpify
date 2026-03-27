package archaeology

import (
	"context"
	"log"
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
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
	version, err := cachedActiveVersion(state, workflowID, plan)
	if err == nil && version == nil {
		version, err = s.Plans.EnsureActiveVersion(ctx, workflowID, plan, archaeoplans.DraftVersionInput{
			WorkflowID:             workflowID,
			DerivedFromExploration: state.GetString("euclo.active_exploration_id"),
			BasedOnRevision:        basedOnRevisionFromTask(task, state),
			SemanticSnapshotRef:    state.GetString("euclo.active_exploration_snapshot_id"),
		})
	}
	if err == nil && version != nil {
		state.Set("euclo.active_plan_version", version.Version)
		state.Set("euclo.preloaded_active_plan_version", version)
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

func cachedActiveVersion(state *core.Context, workflowID string, plan *frameworkplan.LivingPlan) (*archaeodomain.VersionedLivingPlan, error) {
	if state == nil || plan == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	raw, ok := state.Get("euclo.preloaded_active_plan_version")
	if !ok || raw == nil {
		return nil, nil
	}
	version, ok := raw.(*archaeodomain.VersionedLivingPlan)
	if !ok || version == nil {
		return nil, nil
	}
	if strings.TrimSpace(version.WorkflowID) != strings.TrimSpace(workflowID) {
		return nil, nil
	}
	if strings.TrimSpace(version.Plan.ID) != strings.TrimSpace(plan.ID) || version.Version != plan.Version {
		return nil, nil
	}
	return version, nil
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
	snapshotInput := SnapshotInput{
		WorkflowID:          workflowID,
		WorkspaceID:         workspaceID,
		BasedOnRevision:     basedOnRevisionFromTask(task, state),
		OpenLearningIDs:     stringSliceFromState(state, "euclo.pending_learning_ids"),
		Summary:             taskInstruction(task),
		SemanticSnapshotRef: state.GetString("euclo.semantic_snapshot_ref"),
	}
	snapshot, err := s.ensurePreparedSnapshot(ctx, workflowID, session, snapshotInput)
	if err != nil || snapshot == nil {
		return err
	}
	state.Set("euclo.active_exploration_snapshot_id", snapshot.ID)
	state.Set("euclo.based_on_revision", snapshot.BasedOnRevision)
	return nil
}

func (s Service) ensurePreparedSnapshot(ctx context.Context, workflowID string, session *archaeodomain.ExplorationSession, input SnapshotInput) (*archaeodomain.ExplorationSnapshot, error) {
	if session == nil {
		return nil, nil
	}
	if snapshot := s.reusablePreparedSnapshot(ctx, workflowID, session, input); snapshot != nil {
		return snapshot, nil
	}
	return s.CreateExplorationSnapshot(ctx, session, input)
}

func (s Service) reusablePreparedSnapshot(ctx context.Context, workflowID string, session *archaeodomain.ExplorationSession, input SnapshotInput) *archaeodomain.ExplorationSnapshot {
	if session == nil || strings.TrimSpace(session.LatestSnapshotID) == "" {
		return nil
	}
	snapshot, err := s.LoadExplorationSnapshotByWorkflow(ctx, workflowID, session.LatestSnapshotID)
	if err != nil || snapshot == nil {
		return nil
	}
	if strings.TrimSpace(snapshot.ExplorationID) != strings.TrimSpace(session.ID) {
		return nil
	}
	if strings.TrimSpace(snapshot.WorkspaceID) != strings.TrimSpace(firstNonEmpty(strings.TrimSpace(input.WorkspaceID), session.WorkspaceID)) {
		return nil
	}
	if strings.TrimSpace(snapshot.BasedOnRevision) != strings.TrimSpace(input.BasedOnRevision) {
		return nil
	}
	if strings.TrimSpace(snapshot.SemanticSnapshotRef) != strings.TrimSpace(input.SemanticSnapshotRef) {
		return nil
	}
	if strings.TrimSpace(snapshot.Summary) != strings.TrimSpace(input.Summary) {
		return nil
	}
	if !sameStringSet(snapshot.OpenLearningIDs, input.OpenLearningIDs) {
		return nil
	}
	if len(snapshot.CandidatePatternRefs) > 0 || len(snapshot.CandidateAnchorRefs) > 0 || len(snapshot.TensionIDs) > 0 {
		return nil
	}
	return snapshot
}

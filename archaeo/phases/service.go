package phases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

const (
	phaseStateArtifactKind = "archaeo_phase_state"
	phaseStateArtifactID   = "archaeo_phase_state"
)

type Service struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
}

func (s Service) RecordState(ctx context.Context, task *core.Task, state *core.Context, broker *guidance.GuidanceBroker, phase archaeodomain.EucloPhase, blockedReason string, step *frameworkplan.PlanStep) (*archaeodomain.WorkflowPhaseState, error) {
	if state == nil {
		return nil, nil
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		return nil, nil
	}
	initial := archaeodomain.PhaseArchaeology
	transition := archaeodomain.PhaseTransition{
		To:              phase,
		BlockedReason:   blockedReason,
		PendingGuidance: pendingGuidanceIDs(broker),
	}
	if plan, ok := state.Get("euclo.living_plan"); ok && plan != nil {
		if typed, ok := plan.(*frameworkplan.LivingPlan); ok && typed != nil {
			transition.ActivePlanID = typed.ID
			if typed.Version > 0 {
				version := typed.Version
				transition.ActivePlanVersion = &version
			}
		}
	}
	if phase == archaeodomain.PhaseBlocked && blockedReason == "" {
		transition.BlockedReason = "blocked"
	}
	persisted, err := s.Transition(ctx, workflowID, initial, transition)
	if err == nil && persisted != nil {
		state.Set("euclo.archaeo_phase_state", persisted)
	}
	return persisted, err
}

func (s Service) Load(ctx context.Context, workflowID string) (*archaeodomain.WorkflowPhaseState, bool, error) {
	if s.Store == nil || workflowID == "" {
		return nil, false, nil
	}
	artifacts, err := s.Store.ListWorkflowArtifacts(ctx, workflowID, "")
	if err != nil {
		return nil, false, err
	}
	for _, artifact := range artifacts {
		if artifact.Kind != phaseStateArtifactKind {
			continue
		}
		var state archaeodomain.WorkflowPhaseState
		if err := json.Unmarshal([]byte(artifact.InlineRawText), &state); err != nil {
			return nil, false, err
		}
		return &state, true, nil
	}
	return nil, false, nil
}

func (s Service) Ensure(ctx context.Context, workflowID string, initial archaeodomain.EucloPhase) (*archaeodomain.WorkflowPhaseState, error) {
	if workflowID == "" {
		return nil, nil
	}
	state, ok, err := s.Load(ctx, workflowID)
	if err != nil || ok {
		return state, err
	}
	now := s.now()
	state = &archaeodomain.WorkflowPhaseState{
		WorkflowID:       workflowID,
		CurrentPhase:     initial,
		EnteredAt:        now,
		LastTransitionAt: now,
	}
	return state, s.save(ctx, state)
}

func (s Service) Transition(ctx context.Context, workflowID string, initial archaeodomain.EucloPhase, transition archaeodomain.PhaseTransition) (*archaeodomain.WorkflowPhaseState, error) {
	if workflowID == "" {
		return nil, nil
	}
	state, err := s.Ensure(ctx, workflowID, initial)
	if err != nil || state == nil {
		return state, err
	}
	if transition.To == "" {
		return state, nil
	}
	if state.CurrentPhase != "" && !validTransition(state.CurrentPhase, transition.To) && state.CurrentPhase != transition.To {
		return nil, fmt.Errorf("invalid archaeo phase transition %s -> %s", state.CurrentPhase, transition.To)
	}
	if state.CurrentPhase != transition.To {
		at := transition.At
		if at.IsZero() {
			at = s.now()
		}
		state.CurrentPhase = transition.To
		state.EnteredAt = at
		state.LastTransitionAt = at
	}
	if transition.ActiveExplorationID != "" {
		state.ActiveExplorationID = transition.ActiveExplorationID
	}
	if transition.ActivePlanID != "" {
		state.ActivePlanID = transition.ActivePlanID
	}
	if transition.ActivePlanVersion != nil {
		version := *transition.ActivePlanVersion
		state.ActivePlanVersion = &version
	}
	if transition.BlockedReason != "" || transition.To == archaeodomain.PhaseBlocked {
		state.BlockedReason = transition.BlockedReason
	} else if transition.To != archaeodomain.PhaseBlocked {
		state.BlockedReason = ""
	}
	if transition.PendingGuidance != nil {
		state.PendingGuidance = append([]string(nil), transition.PendingGuidance...)
		sort.Strings(state.PendingGuidance)
	}
	if transition.PendingLearning != nil {
		state.PendingLearning = append([]string(nil), transition.PendingLearning...)
		sort.Strings(state.PendingLearning)
	}
	if transition.RecomputeRequired != nil {
		state.RecomputeRequired = *transition.RecomputeRequired
	}
	if transition.BasedOnRevision != "" {
		state.BasedOnRevision = transition.BasedOnRevision
	}
	if err := s.save(ctx, state); err != nil {
		return nil, err
	}
	if state.CurrentPhase != "" && s.Store != nil {
		if err := archaeoevents.AppendWorkflowEvent(ctx, s.Store, workflowID, archaeoevents.EventWorkflowPhaseTransitioned, fmt.Sprintf("%s", state.CurrentPhase), map[string]any{
			"phase":               state.CurrentPhase,
			"blocked_reason":      state.BlockedReason,
			"active_exploration":  state.ActiveExplorationID,
			"active_plan_id":      state.ActivePlanID,
			"active_plan_version": state.ActivePlanVersion,
			"pending_guidance":    state.PendingGuidance,
			"pending_learning":    state.PendingLearning,
		}, s.now()); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (s Service) SyncPendingLearning(ctx context.Context, workflowID string, pending []string) (*archaeodomain.WorkflowPhaseState, error) {
	if workflowID == "" {
		return nil, nil
	}
	state, err := s.Ensure(ctx, workflowID, archaeodomain.PhaseArchaeology)
	if err != nil || state == nil {
		return state, err
	}
	return s.Transition(ctx, workflowID, archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{
		To:              state.CurrentPhase,
		PendingLearning: pending,
	})
}

func (s Service) save(ctx context.Context, state *archaeodomain.WorkflowPhaseState) error {
	if s.Store == nil || state == nil {
		return nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, ok, err := s.Store.GetWorkflow(ctx, state.WorkflowID); err != nil {
		return err
	} else if !ok {
		return errors.New("workflow record required before saving archaeo phase state")
	}
	return s.Store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("%s:%s", phaseStateArtifactID, state.WorkflowID),
		WorkflowID:      state.WorkflowID,
		Kind:            phaseStateArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("archaeo phase: %s", state.CurrentPhase),
		SummaryMetadata: map[string]any{"phase": state.CurrentPhase},
		InlineRawText:   string(raw),
		CreatedAt:       s.now(),
	})
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func validTransition(from, to archaeodomain.EucloPhase) bool {
	if from == "" || from == to {
		return true
	}
	allowed := map[archaeodomain.EucloPhase][]archaeodomain.EucloPhase{
		archaeodomain.PhaseArchaeology:       {archaeodomain.PhaseIntentElicitation, archaeodomain.PhasePlanFormation, archaeodomain.PhaseSurfacing, archaeodomain.PhaseBlocked, archaeodomain.PhaseDeferred},
		archaeodomain.PhaseIntentElicitation: {archaeodomain.PhaseArchaeology, archaeodomain.PhasePlanFormation, archaeodomain.PhaseSurfacing, archaeodomain.PhaseBlocked, archaeodomain.PhaseDeferred},
		archaeodomain.PhasePlanFormation:     {archaeodomain.PhaseIntentElicitation, archaeodomain.PhaseExecution, archaeodomain.PhaseSurfacing, archaeodomain.PhaseBlocked, archaeodomain.PhaseDeferred},
		archaeodomain.PhaseExecution:         {archaeodomain.PhaseVerification, archaeodomain.PhaseBlocked, archaeodomain.PhaseDeferred, archaeodomain.PhaseArchaeology, archaeodomain.PhaseIntentElicitation},
		archaeodomain.PhaseVerification:      {archaeodomain.PhaseExecution, archaeodomain.PhaseIntentElicitation, archaeodomain.PhaseSurfacing, archaeodomain.PhaseCompleted, archaeodomain.PhaseBlocked, archaeodomain.PhaseDeferred},
		archaeodomain.PhaseSurfacing:         {archaeodomain.PhaseArchaeology, archaeodomain.PhaseIntentElicitation, archaeodomain.PhasePlanFormation, archaeodomain.PhaseExecution, archaeodomain.PhaseVerification, archaeodomain.PhaseCompleted, archaeodomain.PhaseDeferred},
		archaeodomain.PhaseBlocked:           {archaeodomain.PhaseArchaeology, archaeodomain.PhaseIntentElicitation, archaeodomain.PhasePlanFormation, archaeodomain.PhaseExecution, archaeodomain.PhaseVerification, archaeodomain.PhaseSurfacing, archaeodomain.PhaseDeferred},
		archaeodomain.PhaseDeferred:          {archaeodomain.PhaseArchaeology, archaeodomain.PhaseIntentElicitation, archaeodomain.PhasePlanFormation, archaeodomain.PhaseExecution, archaeodomain.PhaseVerification, archaeodomain.PhaseSurfacing, archaeodomain.PhaseCompleted},
		archaeodomain.PhaseCompleted:         {archaeodomain.PhaseArchaeology, archaeodomain.PhaseSurfacing},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func workflowIDFromTaskState(task *core.Task, state *core.Context) string {
	if state != nil {
		if workflowID := strings.TrimSpace(state.GetString("euclo.workflow_id")); workflowID != "" {
			return workflowID
		}
	}
	if task != nil && task.Context != nil {
		if workflowID := strings.TrimSpace(stringValue(task.Context["workflow_id"])); workflowID != "" {
			return workflowID
		}
	}
	return ""
}

func pendingGuidanceIDs(broker *guidance.GuidanceBroker) []string {
	if broker == nil {
		return nil
	}
	requests := broker.PendingRequests()
	if len(requests) == 0 {
		return nil
	}
	out := make([]string, 0, len(requests))
	for _, req := range requests {
		if strings.TrimSpace(req.ID) == "" {
			continue
		}
		out = append(out, req.ID)
	}
	return out
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}

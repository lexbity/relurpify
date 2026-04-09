package euclo

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
	euclowork "github.com/lexcodex/relurpify/named/euclo/runtime/work"
)

func (a *Agent) runtimeState(task *core.Task, state *core.Context) (eucloruntime.TaskEnvelope, eucloruntime.TaskClassification, euclotypes.ModeResolution, euclotypes.ExecutionProfileSelection, eucloruntime.UnitOfWork) {
	envelope := eucloruntime.NormalizeTaskEnvelope(task, state, a.CapabilityRegistry())
	classification := eucloruntime.ClassifyTask(envelope)
	mode := eucloruntime.ResolveMode(envelope, classification, a.ModeRegistry)
	profile := eucloruntime.SelectExecutionProfile(envelope, classification, mode, a.ProfileRegistry)
	envelope.ResolvedMode = mode.ModeID
	envelope.ExecutionProfile = profile.ProfileID
	skillPolicy := eucloruntime.BuildResolvedExecutionPolicy(task, a.Config, a.CapabilityRegistry(), mode, profile)
	semanticInputs := a.semanticInputBundle(task, state, mode)
	work := euclowork.BuildUnitOfWork(task, state, envelope, classification, mode, profile, a.ModeRegistry, semanticInputs, skillPolicy, eucloruntime.WorkUnitExecutorDescriptor{})
	return envelope, classification, mode, profile, work
}

func (a *Agent) seedRuntimeState(state *core.Context, envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution, profile euclotypes.ExecutionProfileSelection, work eucloruntime.UnitOfWork) {
	if state == nil {
		return
	}
	history := []eucloruntime.UnitOfWorkHistoryEntry(nil)
	if raw, ok := state.Get("euclo.unit_of_work_history"); ok && raw != nil {
		if typed, ok := raw.([]eucloruntime.UnitOfWorkHistoryEntry); ok {
			history = append(history, typed...)
		}
	}
	if len(history) == 0 {
		if raw, ok := state.Get("euclo.unit_of_work"); ok && raw != nil {
			if existing, ok := raw.(eucloruntime.UnitOfWork); ok && existing.ID != "" {
				history = eucloruntime.UpdateUnitOfWorkHistory(history, existing, existing.UpdatedAt)
			}
		}
	}
	state.Set("euclo.envelope", envelope)
	state.Set("euclo.classification", classification)
	state.Set("euclo.mode_resolution", mode)
	state.Set("euclo.execution_profile_selection", profile)
	state.Set("euclo.mode", mode.ModeID)
	state.Set("euclo.execution_profile", profile.ProfileID)
	state.Set("euclo.semantic_inputs", work.SemanticInputs)
	state.Set("euclo.resolved_execution_policy", work.ResolvedPolicy)
	state.Set("euclo.executor_descriptor", work.ExecutorDescriptor)
	state.Set("euclo.unit_of_work", work)
	state.Set("euclo.unit_of_work_id", work.ID)
	state.Set("euclo.root_unit_of_work_id", work.RootID)
	state.Set("euclo.unit_of_work_transition", work.TransitionState)
	state.Set("euclo.unit_of_work_history", eucloruntime.UpdateUnitOfWorkHistory(history, work, work.UpdatedAt))
}

func (a *Agent) ensureWorkflowRun(ctx context.Context, task *core.Task, state *core.Context) {
	if a == nil || state == nil {
		return
	}
	store := a.workflowStore()
	if store == nil {
		return
	}
	_, _, _ = euclorestore.EnsureWorkflowRun(ctx, store, task, state)
}

func (a *Agent) ensureDeferralPlan(task *core.Task, state *core.Context) {
	if a == nil || a.GuidanceBroker == nil {
		return
	}
	workflowID := workflowIDFromTaskState(task, state)
	if workflowID == "" {
		workflowID = "session"
	}
	if a.DeferralPlan == nil || a.DeferralPlan.WorkflowID != workflowID {
		now := time.Now().UTC()
		a.DeferralPlan = &guidance.DeferralPlan{
			ID:         fmt.Sprintf("deferral-%d", now.UnixNano()),
			WorkflowID: workflowID,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
	a.GuidanceBroker.SetDeferralPlan(a.DeferralPlan)
}

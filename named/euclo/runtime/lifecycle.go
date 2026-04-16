package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statebus"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statekeys"
)

func ContextLifecycleFromState(state *core.Context) (ContextLifecycleState, bool) {
	if state == nil {
		return ContextLifecycleState{}, false
	}
	raw, ok := statebus.GetAny(state, statekeys.KeyContextCompaction)
	if !ok || raw == nil {
		return ContextLifecycleState{}, false
	}
	switch typed := raw.(type) {
	case ContextLifecycleState:
		return typed, true
	case *ContextLifecycleState:
		if typed != nil {
			return *typed, true
		}
	}
	return decodeContextLifecycle(raw)
}

func CompiledExecutionFromState(state *core.Context) (CompiledExecution, bool) {
	if state == nil {
		return CompiledExecution{}, false
	}
	raw, ok := statebus.GetAny(state, statekeys.KeyCompiledExecution)
	if !ok || raw == nil {
		return CompiledExecution{}, false
	}
	switch typed := raw.(type) {
	case CompiledExecution:
		return typed, true
	case *CompiledExecution:
		if typed != nil {
			return *typed, true
		}
	}
	return decodeCompiledExecution(raw)
}

func ReconstructUnitOfWorkFromCompiledExecution(state *core.Context) (UnitOfWork, bool) {
	compiled, ok := CompiledExecutionFromState(state)
	if !ok {
		return UnitOfWork{}, false
	}
	uow := UnitOfWork{
		ID:                              compiled.UnitOfWorkID,
		RootID:                          firstNonEmpty(compiled.RootUnitOfWorkID, compiled.UnitOfWorkID),
		WorkflowID:                      compiled.WorkflowID,
		RunID:                           compiled.RunID,
		ExecutionID:                     compiled.ExecutionID,
		ModeID:                          compiled.ModeID,
		ObjectiveKind:                   compiled.ObjectiveKind,
		BehaviorFamily:                  compiled.BehaviorFamily,
		ContextStrategyID:               compiled.ContextStrategyID,
		VerificationPolicyID:            compiled.VerificationPolicyID,
		DeferralPolicyID:                compiled.DeferralPolicyID,
		CheckpointPolicyID:              compiled.CheckpointPolicyID,
		PrimaryRelurpicCapabilityID:     compiled.PrimaryRelurpicCapabilityID,
		SupportingRelurpicCapabilityIDs: append([]string(nil), compiled.SupportingRelurpicCapabilityIDs...),
		SemanticInputs:                  compiled.SemanticInputs,
		ResolvedPolicy:                  compiled.ResolvedPolicy,
		ExecutorDescriptor:              compiled.ExecutorDescriptor,
		PlanBinding:                     clonePlanBinding(compiled.PlanBinding),
		ContextBundle:                   compiled.ContextBundle,
		RoutineBindings:                 append([]UnitOfWorkRoutineBinding(nil), compiled.RoutineBindings...),
		SkillBindings:                   append([]UnitOfWorkSkillBinding(nil), compiled.SkillBindings...),
		ToolBindings:                    append([]UnitOfWorkToolBinding(nil), compiled.ToolBindings...),
		CapabilityBindings:              append([]UnitOfWorkCapabilityBinding(nil), compiled.CapabilityBindings...),
		PredecessorUnitOfWorkID:         compiled.PredecessorUnitOfWorkID,
		TransitionReason:                compiled.TransitionReason,
		TransitionState:                 compiled.TransitionState,
		ResultClass:                     compiled.ResultClass,
		DeferredIssueIDs:                append([]string(nil), compiled.DeferredIssueIDs...),
		CreatedAt:                       compiled.CompiledAt,
		UpdatedAt:                       compiled.UpdatedAt,
	}
	switch compiled.Status {
	case ExecutionStatusRestoring:
		uow.Status = UnitOfWorkStatusRestoring
	case ExecutionStatusCompacted:
		uow.Status = UnitOfWorkStatusCompacted
	case ExecutionStatusCompletedWithDeferrals:
		uow.Status = UnitOfWorkStatusCompletedWithDeferrals
	case ExecutionStatusCompleted:
		uow.Status = UnitOfWorkStatusCompleted
	case ExecutionStatusBlocked:
		uow.Status = UnitOfWorkStatusBlocked
	case ExecutionStatusFailed, ExecutionStatusRestoreFailed:
		uow.Status = UnitOfWorkStatusFailed
	case ExecutionStatusCanceled:
		uow.Status = UnitOfWorkStatusCanceled
	default:
		uow.Status = UnitOfWorkStatusReady
	}
	if lifecycle, ok := ContextLifecycleFromState(state); ok {
		switch lifecycle.Stage {
		case ContextLifecycleStageRestoring:
			uow.Status = UnitOfWorkStatusRestoring
		case ContextLifecycleStageCompacted:
			uow.Status = UnitOfWorkStatusCompacted
		}
	}
	if uow.ID == "" {
		uow.ID = firstNonEmpty(compiled.ExecutionID, compiled.RunID, compiled.WorkflowID)
	}
	if uow.ID == "" {
		return UnitOfWork{}, false
	}
	return uow, true
}

func RestoreRequested(task *core.Task, state *core.Context) bool {
	if lifecycle, ok := ContextLifecycleFromState(state); ok && lifecycle.RestoreRequired {
		return true
	}
	if task == nil || task.Context == nil {
		return false
	}
	if raw, ok := task.Context["euclo.restore_continuity"]; ok && boolValue(raw) {
		return true
	}
	if raw, ok := task.Context["euclo.context_compaction"]; ok {
		if lifecycle, ok := decodeContextLifecycle(raw); ok {
			return lifecycle.RestoreRequired || lifecycle.Stage == ContextLifecycleStageCompacted
		}
	}
	return false
}

func BuildContextLifecycleState(uow UnitOfWork, prior ContextLifecycleState, status ExecutionStatus, artifactKinds []string, now time.Time) ContextLifecycleState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lifecycle := prior
	lifecycle.WorkflowID = firstNonEmpty(uow.WorkflowID, lifecycle.WorkflowID)
	lifecycle.RunID = firstNonEmpty(uow.RunID, lifecycle.RunID)
	lifecycle.ExecutionID = firstNonEmpty(uow.ExecutionID, lifecycle.ExecutionID)
	lifecycle.UnitOfWorkID = firstNonEmpty(uow.ID, lifecycle.UnitOfWorkID)
	lifecycle.CompactionEligible = uow.ContextBundle.CompactionEligible
	lifecycle.RestoreRequired = uow.ContextBundle.RestoreRequired || lifecycle.RestoreRequired
	lifecycle.DeferredIssueIDs = append([]string(nil), uow.DeferredIssueIDs...)
	lifecycle.PreservedArtifacts = append([]string(nil), artifactKinds...)
	if uow.PlanBinding != nil {
		lifecycle.ActivePlanID = uow.PlanBinding.PlanID
		lifecycle.ActivePlanVersion = uow.PlanBinding.PlanVersion
	}
	switch status {
	case ExecutionStatusCompacted:
		lifecycle.Stage = ContextLifecycleStageCompacted
		lifecycle.CompactionCount++
		lifecycle.LastCompactedAt = now
		lifecycle.Summary = fmt.Sprintf("context compacted for workflow %s run %s", lifecycle.WorkflowID, lifecycle.RunID)
	case ExecutionStatusRestoring:
		lifecycle.Stage = ContextLifecycleStageRestoring
		lifecycle.RestoreCount++
		lifecycle.RestoreSource = "workflow_artifacts"
		lifecycle.Summary = fmt.Sprintf("restoring execution continuity for workflow %s run %s", lifecycle.WorkflowID, lifecycle.RunID)
	case ExecutionStatusRestoreFailed:
		lifecycle.Stage = ContextLifecycleStageRestoreFailed
		lifecycle.LastRestoreStatus = string(ExecutionStatusRestoreFailed)
		lifecycle.Summary = fmt.Sprintf("failed to restore execution continuity for workflow %s run %s", lifecycle.WorkflowID, lifecycle.RunID)
	default:
		if lifecycle.RestoreCount > 0 {
			lifecycle.Stage = ContextLifecycleStageRestored
			lifecycle.LastRestoredAt = now
			lifecycle.LastRestoreStatus = string(status)
			lifecycle.Summary = fmt.Sprintf("restored execution continuity for workflow %s run %s", lifecycle.WorkflowID, lifecycle.RunID)
		} else {
			lifecycle.Stage = ContextLifecycleStageActive
			lifecycle.Summary = fmt.Sprintf("execution active for workflow %s run %s", lifecycle.WorkflowID, lifecycle.RunID)
		}
	}
	return lifecycle
}

func MarkContextLifecycleRestoring(state *core.Context, now time.Time) ContextLifecycleState {
	prior, _ := ContextLifecycleFromState(state)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	prior.Stage = ContextLifecycleStageRestoring
	prior.RestoreRequired = true
	prior.RestoreCount++
	prior.RestoreSource = "workflow_artifacts"
	prior.Summary = fmt.Sprintf("restoring execution continuity for workflow %s run %s", prior.WorkflowID, prior.RunID)
	statebus.SetAny(state, statekeys.KeyContextCompaction, prior)
	return prior
}

func decodeCompiledExecution(raw any) (CompiledExecution, bool) {
	blob, err := json.Marshal(raw)
	if err != nil {
		return CompiledExecution{}, false
	}
	var compiled CompiledExecution
	if err := json.Unmarshal(blob, &compiled); err != nil {
		return CompiledExecution{}, false
	}
	if compiled.UnitOfWorkID == "" && compiled.ExecutionID == "" && compiled.WorkflowID == "" {
		return CompiledExecution{}, false
	}
	return compiled, true
}

func decodeContextLifecycle(raw any) (ContextLifecycleState, bool) {
	blob, err := json.Marshal(raw)
	if err != nil {
		return ContextLifecycleState{}, false
	}
	var lifecycle ContextLifecycleState
	if err := json.Unmarshal(blob, &lifecycle); err != nil {
		return ContextLifecycleState{}, false
	}
	if lifecycle.WorkflowID == "" && lifecycle.RunID == "" && lifecycle.Stage == "" && !lifecycle.RestoreRequired {
		return ContextLifecycleState{}, false
	}
	return lifecycle, true
}

func clonePlanBinding(in *UnitOfWorkPlanBinding) *UnitOfWorkPlanBinding {
	if in == nil {
		return nil
	}
	out := *in
	out.RootChunkIDs = append([]string(nil), in.RootChunkIDs...)
	out.StepIDs = append([]string(nil), in.StepIDs...)
	if len(in.ArchaeoRefs) > 0 {
		out.ArchaeoRefs = make(map[string][]string, len(in.ArchaeoRefs))
		for key, refs := range in.ArchaeoRefs {
			out.ArchaeoRefs[key] = append([]string(nil), refs...)
		}
	}
	return &out
}

func boolValue(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "y":
			return true
		}
	}
	return false
}

package runtime

import (
	"context"
	"time"

	euclokeys "github.com/lexcodex/relurpify/named/euclo/runtime/keys"
)

func BuildCompiledExecution(uow UnitOfWork, status RuntimeExecutionStatus, compiledAt time.Time) CompiledExecution {
	if compiledAt.IsZero() {
		compiledAt = time.Now().UTC()
	}
	updatedAt := status.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = compiledAt
	}
	return CompiledExecution{
		WorkflowID:                      uow.WorkflowID,
		RunID:                           uow.RunID,
		ExecutionID:                     uow.ExecutionID,
		UnitOfWorkID:                    uow.ID,
		RootUnitOfWorkID:                uow.RootID,
		CompiledAt:                      compiledAt,
		UpdatedAt:                       updatedAt,
		ModeID:                          uow.ModeID,
		ObjectiveKind:                   uow.ObjectiveKind,
		BehaviorFamily:                  uow.BehaviorFamily,
		ContextStrategyID:               uow.ContextStrategyID,
		VerificationPolicyID:            uow.VerificationPolicyID,
		DeferralPolicyID:                uow.DeferralPolicyID,
		CheckpointPolicyID:              uow.CheckpointPolicyID,
		PrimaryRelurpicCapabilityID:     uow.PrimaryRelurpicCapabilityID,
		SupportingRelurpicCapabilityIDs: append([]string(nil), uow.SupportingRelurpicCapabilityIDs...),
		SemanticInputs:                  uow.SemanticInputs,
		ResolvedPolicy:                  uow.ResolvedPolicy,
		ExecutorDescriptor:              uow.ExecutorDescriptor,
		PlanBinding:                     uow.PlanBinding,
		ContextBundle:                   uow.ContextBundle,
		RoutineBindings:                 append([]UnitOfWorkRoutineBinding(nil), uow.RoutineBindings...),
		SkillBindings:                   append([]UnitOfWorkSkillBinding(nil), uow.SkillBindings...),
		ToolBindings:                    append([]UnitOfWorkToolBinding(nil), uow.ToolBindings...),
		CapabilityBindings:              append([]UnitOfWorkCapabilityBinding(nil), uow.CapabilityBindings...),
		PredecessorUnitOfWorkID:         uow.PredecessorUnitOfWorkID,
		TransitionReason:                uow.TransitionReason,
		TransitionState:                 uow.TransitionState,
		Status:                          status.Status,
		ResultClass:                     status.ResultClass,
		AssuranceClass:                  status.AssuranceClass,
		DeferredIssueIDs:                append([]string(nil), status.DeferredIssueIDs...),
		ArchaeoRefs:                     archaeoRefsForCompiledExecution(uow),
	}
}

func BuildRuntimeExecutionStatus(uow UnitOfWork, status ExecutionStatus, resultClass ExecutionResultClass, updatedAt time.Time) RuntimeExecutionStatus {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	record := RuntimeExecutionStatus{
		WorkflowID:       uow.WorkflowID,
		RunID:            uow.RunID,
		ExecutionID:      uow.ExecutionID,
		UnitOfWorkID:     uow.ID,
		Status:           status,
		ResultClass:      resultClass,
		AssuranceClass:   uow.AssuranceClass,
		DeferredIssueIDs: append([]string(nil), uow.DeferredIssueIDs...),
		UpdatedAt:        updatedAt,
	}
	if uow.PlanBinding != nil {
		record.ActivePlanID = uow.PlanBinding.PlanID
		record.ActivePlanVersion = uow.PlanBinding.PlanVersion
		record.ActiveStepID = uow.PlanBinding.ActiveStepID
	}
	return record
}

func SeedCompiledExecutionState(stateSetter interface{ Set(string, any) }, uow UnitOfWork, status RuntimeExecutionStatus) {
	if stateSetter == nil {
		return
	}
	compiledAt := uow.CreatedAt
	if compiledAt.IsZero() {
		compiledAt = status.UpdatedAt
	}
	if compiledAt.IsZero() {
		compiledAt = time.Now().UTC()
	}
	stateSetter.Set(euclokeys.KeyExecutionStatus, status)
	stateSetter.Set(euclokeys.KeyCompiledExecution, BuildCompiledExecution(uow, status, compiledAt))
}

func ResultClassForOutcome(status ExecutionStatus, deferredIssueIDs []string, err error) ExecutionResultClass {
	switch {
	case err != nil && err == context.Canceled:
		return ExecutionResultClassCanceled
	case status == ExecutionStatusRestoreFailed:
		return ExecutionResultClassRestoreFailed
	case err != nil || status == ExecutionStatusFailed:
		return ExecutionResultClassFailed
	case status == ExecutionStatusBlocked:
		return ExecutionResultClassBlocked
	case len(deferredIssueIDs) > 0 || status == ExecutionStatusCompletedWithDeferrals:
		return ExecutionResultClassCompletedWithDeferrals
	default:
		return ExecutionResultClassCompleted
	}
}

func StatusForResultClass(status ExecutionStatus, resultClass ExecutionResultClass) ExecutionStatus {
	switch resultClass {
	case ExecutionResultClassCompletedWithDeferrals:
		return ExecutionStatusCompletedWithDeferrals
	case ExecutionResultClassBlocked:
		return ExecutionStatusBlocked
	case ExecutionResultClassCanceled:
		return ExecutionStatusCanceled
	case ExecutionResultClassRestoreFailed:
		return ExecutionStatusRestoreFailed
	case ExecutionResultClassFailed:
		return ExecutionStatusFailed
	default:
		if status == "" {
			return ExecutionStatusCompleted
		}
		return status
	}
}

func archaeoRefsForCompiledExecution(uow UnitOfWork) map[string][]string {
	if uow.PlanBinding == nil || len(uow.PlanBinding.ArchaeoRefs) == 0 {
		return nil
	}
	out := make(map[string][]string, len(uow.PlanBinding.ArchaeoRefs))
	for key, refs := range uow.PlanBinding.ArchaeoRefs {
		out[key] = append([]string(nil), refs...)
	}
	return out
}

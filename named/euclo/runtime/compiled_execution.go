package runtime

import (
	"context"
	"time"

	euclokeys "codeburg.org/lexbit/relurpify/named/euclo/runtime/keys"
)

func BuildCompiledExecution(uow UnitOfWork, status RuntimeExecutionStatus, compiledAt time.Time) CompiledExecution {
	if compiledAt.IsZero() {
		compiledAt = time.Now().UTC()
	}
	updatedAt := status.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = compiledAt
	}
	desc := cloneExecutionDescriptor(uow.ExecutionDescriptor)
	desc.DeferredIssueIDs = append([]string(nil), status.DeferredIssueIDs...)
	desc.ResultClass = status.ResultClass
	desc.AssuranceClass = status.AssuranceClass
	desc.UpdatedAt = updatedAt
	return CompiledExecution{
		ExecutionDescriptor: desc,
		UnitOfWorkID:        uow.ID,
		RootUnitOfWorkID:    firstNonEmpty(uow.RootID, uow.ID),
		Status:              status.Status,
		CompiledAt:          compiledAt,
		ArchaeoRefs:         archaeoRefsForCompiledExecution(uow),
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

func cloneExecutionDescriptor(desc ExecutionDescriptor) ExecutionDescriptor {
	desc.SupportingRelurpicCapabilityIDs = append([]string(nil), desc.SupportingRelurpicCapabilityIDs...)
	desc.PlanBinding = clonePlanBinding(desc.PlanBinding)
	desc.RoutineBindings = append([]UnitOfWorkRoutineBinding(nil), desc.RoutineBindings...)
	desc.SkillBindings = append([]UnitOfWorkSkillBinding(nil), desc.SkillBindings...)
	desc.ToolBindings = append([]UnitOfWorkToolBinding(nil), desc.ToolBindings...)
	desc.CapabilityBindings = append([]UnitOfWorkCapabilityBinding(nil), desc.CapabilityBindings...)
	return desc
}

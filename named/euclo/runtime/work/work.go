package work

import (
	"time"

	frameworkcore "codeburg.org/lexbit/relurpify/framework/core"
	euclotypespkg "codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type TaskEnvelope = runtimepkg.TaskEnvelope
type SemanticRequestRef = runtimepkg.SemanticRequestRef
type SemanticFindingSummary = runtimepkg.SemanticFindingSummary
type PatternProposalSummary = runtimepkg.PatternProposalSummary
type TensionClusterSummary = runtimepkg.TensionClusterSummary
type CoherenceSuggestion = runtimepkg.CoherenceSuggestion
type ProspectivePairingSummary = runtimepkg.ProspectivePairingSummary
type SemanticInputBundle = runtimepkg.SemanticInputBundle
type UnitOfWorkStatus = runtimepkg.UnitOfWorkStatus
type UnitOfWorkTransitionState = runtimepkg.UnitOfWorkTransitionState
type UnitOfWorkHistoryEntry = runtimepkg.UnitOfWorkHistoryEntry
type UnitOfWork = runtimepkg.UnitOfWork
type ExecutionDescriptor = runtimepkg.ExecutionDescriptor
type UnitOfWorkPlanBinding = runtimepkg.UnitOfWorkPlanBinding
type UnitOfWorkContextSource = runtimepkg.UnitOfWorkContextSource
type UnitOfWorkContextBundle = runtimepkg.UnitOfWorkContextBundle
type UnitOfWorkRoutineBinding = runtimepkg.UnitOfWorkRoutineBinding
type UnitOfWorkSkillBinding = runtimepkg.UnitOfWorkSkillBinding
type UnitOfWorkToolBinding = runtimepkg.UnitOfWorkToolBinding
type UnitOfWorkCapabilityBinding = runtimepkg.UnitOfWorkCapabilityBinding
type CompiledExecution = runtimepkg.CompiledExecution
type RuntimeExecutionStatus = runtimepkg.RuntimeExecutionStatus
type ExecutionResultClass = runtimepkg.ExecutionResultClass
type ExecutionStatus = runtimepkg.ExecutionStatus
type EditExecutionRecord = runtimepkg.EditExecutionRecord
type ExecutionEnvelope = euclotypespkg.ExecutionEnvelope

const (
	ExecutionResultClassCompleted              = runtimepkg.ExecutionResultClassCompleted
	ExecutionResultClassCompletedWithDeferrals = runtimepkg.ExecutionResultClassCompletedWithDeferrals
	ExecutionResultClassBlocked                = runtimepkg.ExecutionResultClassBlocked
	ExecutionResultClassFailed                 = runtimepkg.ExecutionResultClassFailed
	ExecutionResultClassCanceled               = runtimepkg.ExecutionResultClassCanceled
	ExecutionResultClassRestoreFailed          = runtimepkg.ExecutionResultClassRestoreFailed
	ExecutionStatusPreparing                   = runtimepkg.ExecutionStatusPreparing
	ExecutionStatusReady                       = runtimepkg.ExecutionStatusReady
	ExecutionStatusExecuting                   = runtimepkg.ExecutionStatusExecuting
	ExecutionStatusVerifying                   = runtimepkg.ExecutionStatusVerifying
	ExecutionStatusSurfacing                   = runtimepkg.ExecutionStatusSurfacing
	ExecutionStatusCompacted                   = runtimepkg.ExecutionStatusCompacted
	ExecutionStatusRestoring                   = runtimepkg.ExecutionStatusRestoring
	ExecutionStatusCompleted                   = runtimepkg.ExecutionStatusCompleted
	ExecutionStatusCompletedWithDeferrals      = runtimepkg.ExecutionStatusCompletedWithDeferrals
	ExecutionStatusBlocked                     = runtimepkg.ExecutionStatusBlocked
	ExecutionStatusFailed                      = runtimepkg.ExecutionStatusFailed
	ExecutionStatusCanceled                    = runtimepkg.ExecutionStatusCanceled
	ExecutionStatusRestoreFailed               = runtimepkg.ExecutionStatusRestoreFailed
	UnitOfWorkStatusAssembling                 = runtimepkg.UnitOfWorkStatusAssembling
	UnitOfWorkStatusReady                      = runtimepkg.UnitOfWorkStatusReady
	UnitOfWorkStatusExecuting                  = runtimepkg.UnitOfWorkStatusExecuting
	UnitOfWorkStatusVerifying                  = runtimepkg.UnitOfWorkStatusVerifying
	UnitOfWorkStatusCompacted                  = runtimepkg.UnitOfWorkStatusCompacted
	UnitOfWorkStatusRestoring                  = runtimepkg.UnitOfWorkStatusRestoring
	UnitOfWorkStatusCompleted                  = runtimepkg.UnitOfWorkStatusCompleted
	UnitOfWorkStatusCompletedWithDeferrals     = runtimepkg.UnitOfWorkStatusCompletedWithDeferrals
	UnitOfWorkStatusBlocked                    = runtimepkg.UnitOfWorkStatusBlocked
	UnitOfWorkStatusFailed                     = runtimepkg.UnitOfWorkStatusFailed
	UnitOfWorkStatusCanceled                   = runtimepkg.UnitOfWorkStatusCanceled
)

func BuildUnitOfWork(
	task *frameworkcore.Task,
	state *frameworkcore.Context,
	envelope TaskEnvelope,
	classification runtimepkg.TaskClassification,
	mode runtimepkg.ModeResolution,
	profile runtimepkg.ExecutionProfileSelection,
	modeRegistry *runtimepkg.ModeRegistry,
	semanticInputs SemanticInputBundle,
	resolvedPolicy runtimepkg.ResolvedExecutionPolicy,
	executor runtimepkg.WorkUnitExecutorDescriptor,
) UnitOfWork {
	return runtimepkg.BuildUnitOfWork(
		task,
		state,
		envelope,
		classification,
		mode,
		profile,
		modeRegistry,
		semanticInputs,
		resolvedPolicy,
		executor,
	)
}

func BuildCompiledExecution(uow UnitOfWork, status RuntimeExecutionStatus, compiledAt time.Time) CompiledExecution {
	return runtimepkg.BuildCompiledExecution(uow, status, compiledAt)
}

func BuildRuntimeExecutionStatus(uow UnitOfWork, status ExecutionStatus, resultClass ExecutionResultClass, updatedAt time.Time) RuntimeExecutionStatus {
	return runtimepkg.BuildRuntimeExecutionStatus(uow, status, resultClass, updatedAt)
}

func SeedCompiledExecutionState(stateSetter interface{ Set(string, any) }, uow UnitOfWork, status RuntimeExecutionStatus) {
	runtimepkg.SeedCompiledExecutionState(stateSetter, uow, status)
}

func ResultClassForOutcome(status ExecutionStatus, deferredIssueIDs []string, err error) ExecutionResultClass {
	return runtimepkg.ResultClassForOutcome(status, deferredIssueIDs, err)
}

func StatusForResultClass(status ExecutionStatus, resultClass ExecutionResultClass) ExecutionStatus {
	return runtimepkg.StatusForResultClass(status, resultClass)
}

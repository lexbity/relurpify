package runtime

import (
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// Public wrapper functions for persistence package integration.

// LoadExecutionState is the exported version of loadExecutionState.
func LoadExecutionState(state *core.Context) ExecutionState {
	return loadExecutionState(state)
}

// PublishExecutionState is the exported version of publishExecutionState.
func PublishExecutionState(state *core.Context, execution ExecutionState) {
	publishExecutionState(state, execution)
}

// NormalizeHTNState is the exported version of normalizeHTNState.
func NormalizeHTNState(snapshot *HTNState) {
	normalizeHTNState(snapshot)
}

// DecodeContextValue is the exported version of decodeContextValue.
func DecodeContextValue(raw any, target any) bool {
	return decodeContextValue(raw, target)
}

// MapsClone is the exported version of mapsClone.
func MapsClone(input map[string]string) map[string]string {
	return mapsClone(input)
}

// MethodStateFromResolved is the exported version of methodStateFromResolved.
func MethodStateFromResolved(resolved ResolvedMethod) MethodState {
	return methodStateFromResolved(resolved)
}

// CompletedStepsFromContext is the exported version of completedStepsFromContext.
func CompletedStepsFromContext(state *core.Context) []string {
	return completedStepsFromContext(state)
}

// PublishTaskState is the exported version of publishTaskState.
func PublishTaskState(state *core.Context, task *core.Task) {
	publishTaskState(state, task)
}

// PublishResolvedMethodState is the exported version of publishResolvedMethodState.
func PublishResolvedMethodState(state *core.Context, method *ResolvedMethod) {
	publishResolvedMethodState(state, method)
}

// PublishPreflightState is the exported version of publishPreflightState.
func PublishPreflightState(state *core.Context, report *graph.PreflightReport, err error) {
	publishPreflightState(state, report, err)
}

// PublishResumeState is the exported version of publishResumeState.
func PublishResumeState(state *core.Context, checkpointID string) {
	publishResumeState(state, checkpointID)
}

// PublishWorkflowRetrieval is the exported version of publishWorkflowRetrieval.
func PublishWorkflowRetrieval(state *core.Context, payload any, applied bool) {
	publishWorkflowRetrieval(state, payload, applied)
}

// PublishPlanState is the exported version of publishPlanState.
func PublishPlanState(state *core.Context, plan *core.Plan) {
	publishPlanState(state, plan)
}

// PublishTerminationState is the exported version of publishTerminationState.
func PublishTerminationState(state *core.Context, termination string) {
	publishTerminationState(state, termination)
}

// PlanPreflight checks plan step required capabilities against the registry.
func PlanPreflight(plan *core.Plan, registry *capability.Registry) (*graph.PreflightReport, error) {
	return planPreflight(plan, registry)
}

// Exported constants for persistence use.
const (
	ContextKeyTask                    = contextKeyTask
	ContextKeySelectedMethod          = contextKeySelectedMethod
	ContextKeyMetrics                 = contextKeyMetrics
	ContextKeyPlan                    = contextKeyPlan
	ContextKeyCheckpoint              = contextKeyCheckpoint
	ContextKeyCheckpointRef           = contextKeyCheckpointRef
	ContextKeyCheckpointSummary       = contextKeyCheckpointSummary
	ContextKeyRunSummaryRef           = contextKeyRunSummaryRef
	ContextKeyRunSummarySummary       = contextKeyRunSummarySummary
	ContextKeyExecutionMetricsRef     = contextKeyExecutionMetricsRef
	ContextKeyExecutionMetricsSummary = contextKeyExecutionMetricsSummary
	ContextKnowledgeMethod            = contextKnowledgeMethod
	HTNSchemaVersion                  = htnSchemaVersion
	ContextKeyLastRecoveryNotes       = contextKeyLastRecoveryNotes
	ContextKeyLastRecoveryDiag        = contextKeyLastRecoveryDiag
	ContextKeyLastFailureStep         = contextKeyLastFailureStep
	ContextKeyLastFailureError        = contextKeyLastFailureError
)

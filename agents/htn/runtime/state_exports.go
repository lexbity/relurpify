package runtime

import (
	"codeburg.org/lexbit/relurpify/agents/plan"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// Public wrapper functions for persistence package integration.

// LoadExecutionState is the exported version of loadExecutionState.
func LoadExecutionState(env *contextdata.Envelope) ExecutionState {
	return loadExecutionState(env)
}

// PublishExecutionState is the exported version of publishExecutionState.
func PublishExecutionState(env *contextdata.Envelope, execution ExecutionState) {
	publishExecutionState(env, execution)
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

// CompletedStepsFromEnvelope is the exported version of completedStepsFromEnvelope.
func CompletedStepsFromEnvelope(env *contextdata.Envelope) []string {
	return completedStepsFromEnvelope(env)
}

// PublishTaskState is the exported version of publishTaskState.
func PublishTaskState(env *contextdata.Envelope, task *core.Task) {
	publishTaskState(env, task)
}

// PublishResolvedMethodState is the exported version of publishResolvedMethodState.
func PublishResolvedMethodState(env *contextdata.Envelope, method *ResolvedMethod) {
	publishResolvedMethodState(env, method)
}

// PublishPreflightState is the exported version of publishPreflightState.
func PublishPreflightState(env *contextdata.Envelope, report *graph.PreflightReport, err error) {
	publishPreflightState(env, report, err)
}

// PublishResumeState is the exported version of publishResumeState.
func PublishResumeState(env *contextdata.Envelope, checkpointID string) {
	publishResumeState(env, checkpointID)
}

// PublishWorkflowRetrieval is the exported version of publishWorkflowRetrieval.
func PublishWorkflowRetrieval(env *contextdata.Envelope, payload any, applied bool) {
	publishWorkflowRetrieval(env, payload, applied)
}

// PublishPlanState is the exported version of publishPlanState.
func PublishPlanState(env *contextdata.Envelope, plan *plan.Plan) {
	publishPlanState(env, plan)
}

// PublishTerminationState is the exported version of publishTerminationState.
func PublishTerminationState(env *contextdata.Envelope, termination string) {
	publishTerminationState(env, termination)
}

// PlanPreflight checks plan step required capabilities against the registry.
func PlanPreflight(plan *plan.Plan, registry *capability.Registry) (*graph.PreflightReport, error) {
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

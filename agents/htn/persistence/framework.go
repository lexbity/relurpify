package persistence

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// envelopeGet retrieves a value from envelope working memory.
func envelopeGet(state *contextdata.Envelope, key string) (any, bool) {
	if state == nil {
		return nil, false
	}
	return state.GetWorkingValue(key)
}

// envelopeSet stores a value in envelope working memory with task scope.
func envelopeSet(state *contextdata.Envelope, key string, value any) {
	if state == nil {
		return
	}
	state.SetWorkingValue(key, value, contextdata.MemoryClassTask)
}

// Phase 9: Framework-native persistence integration for HTN runtime artifacts.
// Persists end-of-run summaries, method metadata, execution metrics, and operator
// outcomes using framework persistence conventions and sinks.

// HTNRunSummary captures end-of-run execution metrics and outcomes.
type HTNRunSummary struct {
	SchemaVersion      int           `json:"schema_version"`
	TaskType           core.TaskType `json:"task_type"`
	SelectedMethod     string        `json:"selected_method"`
	PlannedStepCount   int           `json:"planned_step_count"`
	CompletedStepCount int           `json:"completed_step_count"`
	TerminationStatus  string        `json:"termination_status"`
	TotalDuration      int           `json:"total_duration_seconds"`
	RetrievalApplied   bool          `json:"retrieval_applied"`
	Success            bool          `json:"success"`
	ErrorMessage       string        `json:"error_message,omitempty"`
}

// OperatorOutcome captures results from executing a primitive step.
type OperatorOutcome struct {
	OperatorName string                 `json:"operator_name"`
	StepID       string                 `json:"step_id"`
	TaskType     core.TaskType          `json:"task_type"`
	Success      bool                   `json:"success"`
	Duration     int                    `json:"duration_seconds"`
	CostClass    string                 `json:"cost_class,omitempty"`
	RetryClass   string                 `json:"retry_class,omitempty"`
	Retried      bool                   `json:"retried"`
	RetryCount   int                    `json:"retry_count"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	OutputKeys   []string               `json:"output_keys,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ExecutionMetrics captures quantitative measures of HTN execution.
type ExecutionMetrics struct {
	SchemaVersion     int    `json:"schema_version"`
	TotalDuration     int    `json:"total_duration_seconds"`
	DecompositionTime int    `json:"decomposition_time_seconds"`
	ExecutionTime     int    `json:"execution_time_seconds"`
	PlanStepCount     int    `json:"plan_step_count"`
	CompletedSteps    int    `json:"completed_steps"`
	FailedSteps       int    `json:"failed_steps"`
	RetriedSteps      int    `json:"retried_steps"`
	AverageCost       string `json:"average_cost_class"`
	ParallelBranches  int    `json:"parallel_branches"`
	RetrievalApplied  bool   `json:"retrieval_applied"`
	Success           bool   `json:"success"`
}

// persistHTNRunSummary saves end-of-run summary to workflow artifacts.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func SaveRunSummary(ctx context.Context, state *contextdata.Envelope,
	repo agentlifecycle.Repository, workflowID, runID string,
	startTime time.Time, success bool, err error) error {

	if state == nil || repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	// Placeholder - run summary persistence to be reimplemented
	// using agentlifecycle.Repository
	return nil
}

// persistHTNMethodMetadata persists selected method metadata as knowledge.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func SaveMethodMetadata(ctx context.Context, state *contextdata.Envelope,
	repo agentlifecycle.Repository, workflowID, runID string) error {

	if state == nil || repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	// Placeholder - method metadata persistence to be reimplemented
	// using agentlifecycle.Repository
	return nil
}

// persistHTNExecutionMetrics persists execution metrics as workflow artifact.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func SaveExecutionMetrics(ctx context.Context, state *contextdata.Envelope,
	repo agentlifecycle.Repository, workflowID, runID string,
	decompositionTime time.Duration, executionTime time.Duration) error {

	if state == nil || repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	// Placeholder - execution metrics persistence to be reimplemented
	// using agentlifecycle.Repository
	return nil
}

// PersistOperatorOutcome persists individual operator step outcome.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func PersistOperatorOutcome(ctx context.Context,
	repo agentlifecycle.Repository,
	workflowID, runID, stepRunID string,
	operator string, stepID string,
	duration time.Duration, success bool, outputKeys []string, err error) error {

	if repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	// Placeholder - operator outcome persistence to be reimplemented
	// using agentlifecycle.Repository
	return nil
}

// AppendHTNEvent appends an HTN execution event to workflow history.
// TODO: Reimplement without WorkflowStateStore dependency
// per the agentlifecycle workflow-store removal plan
func AppendHTNEvent(ctx context.Context,
	repo agentlifecycle.Repository,
	workflowID, runID, stepID string,
	eventType, message string) error {

	if repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	// Placeholder - event persistence to be reimplemented
	// using agentlifecycle.Repository
	return nil
}

// Helper functions for marshaling and summarization.

func marshalHTNRunSummary(summary HTNRunSummary) string {
	data, _ := marshalJSON(summary)
	return string(data)
}

func marshalExecutionMetrics(metrics ExecutionMetrics) string {
	data, _ := marshalJSON(metrics)
	return string(data)
}

func marshalOperatorOutcome(outcome OperatorOutcome) string {
	data, _ := marshalJSON(outcome)
	return string(data)
}

func summarizeHTNRun(summary HTNRunSummary) string {
	termStatus := "SUCCESS"
	if !summary.Success {
		termStatus = "FAILED"
	}
	return fmt.Sprintf(
		"[%s] Task: %s | Method: %s | Progress: %d/%d | Duration: %ds",
		termStatus,
		summary.TaskType,
		summary.SelectedMethod,
		summary.CompletedStepCount,
		summary.PlannedStepCount,
		summary.TotalDuration,
	)
}

func summarizeHTNRunMetadata(summary HTNRunSummary) map[string]interface{} {
	return map[string]interface{}{
		"schema_version":     summary.SchemaVersion,
		"task_type":          string(summary.TaskType),
		"selected_method":    summary.SelectedMethod,
		"planned_steps":      summary.PlannedStepCount,
		"completed_steps":    summary.CompletedStepCount,
		"termination_status": summary.TerminationStatus,
		"total_duration":     summary.TotalDuration,
		"retrieval_applied":  summary.RetrievalApplied,
		"success":            summary.Success,
	}
}

func summarizeExecutionMetrics(metrics ExecutionMetrics) string {
	return fmt.Sprintf(
		"Duration: %ds (decomp: %ds, exec: %ds) | Steps: %d/%d | Retries: %d",
		metrics.TotalDuration,
		metrics.DecompositionTime,
		metrics.ExecutionTime,
		metrics.CompletedSteps,
		metrics.PlanStepCount,
		metrics.RetriedSteps,
	)
}

func metricsMetadata(metrics ExecutionMetrics) map[string]interface{} {
	return map[string]interface{}{
		"schema_version":     metrics.SchemaVersion,
		"total_duration":     metrics.TotalDuration,
		"decomposition_time": metrics.DecompositionTime,
		"execution_time":     metrics.ExecutionTime,
		"plan_steps":         metrics.PlanStepCount,
		"completed_steps":    metrics.CompletedSteps,
		"failed_steps":       metrics.FailedSteps,
		"retried_steps":      metrics.RetriedSteps,
		"average_cost":       metrics.AverageCost,
		"parallel_branches":  metrics.ParallelBranches,
		"retrieval_applied":  metrics.RetrievalApplied,
		"success":            metrics.Success,
	}
}

func summarizeOperatorOutcome(outcome OperatorOutcome) string {
	status := "OK"
	if !outcome.Success {
		status = "FAILED"
	}
	return fmt.Sprintf(
		"[%s] %s (%s) | Duration: %ds | Retry: %d | Outputs: %d",
		status,
		outcome.OperatorName,
		outcome.StepID,
		outcome.Duration,
		outcome.RetryCount,
		len(outcome.OutputKeys),
	)
}

func operatorOutcomeMetadata(outcome OperatorOutcome) map[string]interface{} {
	return map[string]interface{}{
		"operator_name": outcome.OperatorName,
		"step_id":       outcome.StepID,
		"task_type":     string(outcome.TaskType),
		"success":       outcome.Success,
		"duration":      outcome.Duration,
		"cost_class":    outcome.CostClass,
		"retry_class":   outcome.RetryClass,
		"retried":       outcome.Retried,
		"retry_count":   outcome.RetryCount,
		"output_count":  len(outcome.OutputKeys),
	}
}

func statusString(success bool) string {
	if success {
		return "completed"
	}
	return "failed"
}

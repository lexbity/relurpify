package persistence

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

// Phase 9: Framework-native persistence integration for HTN runtime artifacts.
// Persists end-of-run summaries, method metadata, execution metrics, and operator
// outcomes using framework persistence conventions and sinks.

// HTNRunSummary captures end-of-run execution metrics and outcomes.
type HTNRunSummary struct {
	SchemaVersion      int                `json:"schema_version"`
	TaskType           execution.TaskType `json:"task_type"`
	SelectedMethod     string             `json:"selected_method"`
	PlannedStepCount   int                `json:"planned_step_count"`
	CompletedStepCount int                `json:"completed_step_count"`
	TerminationStatus  string             `json:"termination_status"`
	TotalDuration      int                `json:"total_duration_seconds"`
	RetrievalApplied   bool               `json:"retrieval_applied"`
	Success            bool               `json:"success"`
	ErrorMessage       string             `json:"error_message,omitempty"`
}

// OperatorOutcome captures results from executing a primitive step.
type OperatorOutcome struct {
	OperatorName string             `json:"operator_name"`
	StepID       string             `json:"step_id"`
	TaskType     execution.TaskType `json:"task_type"`
	Success      bool               `json:"success"`
	Duration     int                `json:"duration_seconds"`
	CostClass    string             `json:"cost_class,omitempty"`
	RetryClass   string             `json:"retry_class,omitempty"`
	Retried      bool               `json:"retried"`
	RetryCount   int                `json:"retry_count"`
	ErrorMessage string             `json:"error_message,omitempty"`
	OutputKeys   []string           `json:"output_keys,omitempty"`
	Metadata     map[string]any     `json:"metadata,omitempty"`
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



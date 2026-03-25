package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/agents/htn/runtime"
	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

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
func SaveRunSummary(ctx context.Context, state *core.Context,
	store memory.WorkflowStateStore, workflowID, runID string,
	startTime time.Time, success bool, err error) error {

	if state == nil || store == nil || workflowID == "" || runID == "" {
		return nil
	}

	// Load HTN state snapshot for summary data.
	snapshot, loaded, stateErr := runtime.LoadStateFromContext(state)
	if stateErr != nil || !loaded || snapshot == nil {
		return nil
	}

	duration := int(time.Since(startTime).Seconds())
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	summary := HTNRunSummary{
		SchemaVersion:      runtime.HTNSchemaVersion,
		TaskType:           snapshot.Task.Type,
		SelectedMethod:     snapshot.Method.Name,
		PlannedStepCount:   snapshot.Execution.PlannedStepCount,
		CompletedStepCount: snapshot.Execution.CompletedStepCount,
		TerminationStatus:  snapshot.Termination,
		TotalDuration:      duration,
		RetrievalApplied:   snapshot.RetrievalApplied,
		Success:            success,
		ErrorMessage:       errorMsg,
	}

	// Persist as workflow artifact.
	artifact := memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("htn_run_summary_%d", time.Now().UnixNano()),
		WorkflowID:      workflowID,
		RunID:           runID,
		Kind:            "htn_run_summary",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     summarizeHTNRun(summary),
		SummaryMetadata: summarizeHTNRunMetadata(summary),
		InlineRawText:   marshalHTNRunSummary(summary),
		CreatedAt:       time.Now().UTC(),
	}

	if err := store.UpsertWorkflowArtifact(ctx, artifact); err != nil {
		return fmt.Errorf("htn: failed to persist run summary: %w", err)
	}
	if state != nil {
		state.Set(runtime.ContextKeyRunSummaryRef, workflowutil.WorkflowArtifactReference(artifact))
		state.Set(runtime.ContextKeyRunSummarySummary, artifact.SummaryText)
	}

	return nil
}

// persistHTNMethodMetadata persists selected method metadata as knowledge.
func SaveMethodMetadata(ctx context.Context, state *core.Context,
	store memory.WorkflowStateStore, workflowID, runID string) error {

	if state == nil || store == nil || workflowID == "" || runID == "" {
		return nil
	}

	snapshot, loaded, err := runtime.LoadStateFromContext(state)
	if err != nil || !loaded || snapshot == nil {
		return nil
	}

	if snapshot.Method.Name == "" {
		return nil // No method selected, nothing to persist.
	}

	// Persist method metadata as decision knowledge.
	content := fmt.Sprintf(
		"Selected method: %s\nTask type: %s\nPriority: %d\nOperators: %d\nSubtasks: %d",
		snapshot.Method.Name,
		snapshot.Method.TaskType,
		snapshot.Method.Priority,
		snapshot.Method.OperatorCount,
		snapshot.Method.SubtaskCount,
	)

	record := memory.KnowledgeRecord{
		RecordID:   fmt.Sprintf("htn_method_%d", time.Now().UnixNano()),
		WorkflowID: workflowID,
		Kind:       memory.KnowledgeKindDecision,
		Title:      fmt.Sprintf("Method: %s", snapshot.Method.Name),
		Content:    content,
		Status:     "accepted",
		Metadata: map[string]interface{}{
			"method_name":         snapshot.Method.Name,
			"task_type":           string(snapshot.Method.TaskType),
			"priority":            snapshot.Method.Priority,
			"operator_count":      snapshot.Method.OperatorCount,
			"subtask_count":       snapshot.Method.SubtaskCount,
			"required_caps_count": len(snapshot.Method.RequiredCapabilities),
		},
		CreatedAt: time.Now().UTC(),
	}

	if err := store.PutKnowledge(ctx, record); err != nil {
		return fmt.Errorf("htn: failed to persist method metadata: %w", err)
	}

	return nil
}

// persistHTNExecutionMetrics persists execution metrics as workflow artifact.
func SaveExecutionMetrics(ctx context.Context, state *core.Context,
	store memory.WorkflowStateStore, workflowID, runID string,
	decompositionTime time.Duration, executionTime time.Duration) error {

	if state == nil || store == nil || workflowID == "" || runID == "" {
		return nil
	}

	snapshot, loaded, err := runtime.LoadStateFromContext(state)
	if err != nil || !loaded || snapshot == nil {
		return nil
	}

	totalDuration := int(decompositionTime.Seconds()) + int(executionTime.Seconds())
	failedSteps := snapshot.Execution.PlannedStepCount - snapshot.Execution.CompletedStepCount
	retried := 0
	if raw, ok := state.Get(runtime.ContextKeyLastFailureStep); ok && raw != nil {
		retried = 1
	}

	metrics := ExecutionMetrics{
		SchemaVersion:     runtime.HTNSchemaVersion,
		TotalDuration:     totalDuration,
		DecompositionTime: int(decompositionTime.Seconds()),
		ExecutionTime:     int(executionTime.Seconds()),
		PlanStepCount:     snapshot.Execution.PlannedStepCount,
		CompletedSteps:    snapshot.Execution.CompletedStepCount,
		FailedSteps:       failedSteps,
		RetriedSteps:      retried,
		AverageCost:       "unknown",
		ParallelBranches:  0,
		RetrievalApplied:  snapshot.RetrievalApplied,
		Success:           snapshot.Termination == "completed",
	}

	// Persist as workflow artifact.
	artifact := memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("htn_metrics_%d", time.Now().UnixNano()),
		WorkflowID:      workflowID,
		RunID:           runID,
		Kind:            "htn_execution_metrics",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     summarizeExecutionMetrics(metrics),
		SummaryMetadata: metricsMetadata(metrics),
		InlineRawText:   marshalExecutionMetrics(metrics),
		CreatedAt:       time.Now().UTC(),
	}

	if err := store.UpsertWorkflowArtifact(ctx, artifact); err != nil {
		return fmt.Errorf("htn: failed to persist execution metrics: %w", err)
	}
	if state != nil {
		state.Set(runtime.ContextKeyExecutionMetricsRef, workflowutil.WorkflowArtifactReference(artifact))
		state.Set(runtime.ContextKeyExecutionMetricsSummary, artifact.SummaryText)
	}

	return nil
}

// PersistOperatorOutcome persists individual operator step outcome.
func PersistOperatorOutcome(ctx context.Context,
	store memory.WorkflowStateStore,
	workflowID, runID, stepRunID string,
	operator string, stepID string,
	duration time.Duration, success bool, outputKeys []string, err error) error {

	if store == nil || workflowID == "" || runID == "" {
		return nil
	}

	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	// Try to extract operator metadata from context if available.
	retryClass := "unknown"
	costClass := "unknown"

	outcome := OperatorOutcome{
		OperatorName: operator,
		StepID:       stepID,
		Success:      success,
		Duration:     int(duration.Seconds()),
		CostClass:    costClass,
		RetryClass:   retryClass,
		ErrorMessage: errorMsg,
		OutputKeys:   outputKeys,
		Metadata: map[string]interface{}{
			"step_run_id": stepRunID,
		},
	}

	// Persist as step artifact.
	artifact := memory.StepArtifactRecord{
		ArtifactID:      fmt.Sprintf("htn_operator_%d", time.Now().UnixNano()),
		WorkflowID:      workflowID,
		StepRunID:       stepRunID,
		Kind:            "htn_operator_outcome",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     summarizeOperatorOutcome(outcome),
		SummaryMetadata: operatorOutcomeMetadata(outcome),
		InlineRawText:   marshalOperatorOutcome(outcome),
		CreatedAt:       time.Now().UTC(),
	}

	if err := store.UpsertArtifact(ctx, artifact); err != nil {
		return fmt.Errorf("htn: failed to persist operator outcome: %w", err)
	}

	// Also persist as knowledge for reasoning.
	knowledgeKind := memory.KnowledgeKindFact
	status := "accepted"
	if !success {
		knowledgeKind = memory.KnowledgeKindIssue
		status = "open"
	}

	knowledge := memory.KnowledgeRecord{
		RecordID:   fmt.Sprintf("htn_op_knowledge_%d", time.Now().UnixNano()),
		WorkflowID: workflowID,
		StepRunID:  stepRunID,
		StepID:     stepID,
		Kind:       knowledgeKind,
		Title:      fmt.Sprintf("Operator %s: %s", operator, statusString(success)),
		Content:    summarizeOperatorOutcome(outcome),
		Status:     status,
		Metadata: map[string]interface{}{
			"operator":    operator,
			"duration":    int(duration.Seconds()),
			"success":     success,
			"output_keys": outputKeys,
		},
		CreatedAt: time.Now().UTC(),
	}

	if err := store.PutKnowledge(ctx, knowledge); err != nil {
		return fmt.Errorf("htn: failed to persist operator knowledge: %w", err)
	}

	return nil
}

// AppendHTNEvent appends an HTN execution event to workflow history.
func AppendHTNEvent(ctx context.Context,
	store memory.WorkflowStateStore,
	workflowID, runID, stepID string,
	eventType, message string) error {

	if store == nil || workflowID == "" || runID == "" {
		return nil
	}

	event := memory.WorkflowEventRecord{
		EventID:    fmt.Sprintf("htn_event_%d", time.Now().UnixNano()),
		WorkflowID: workflowID,
		RunID:      runID,
		StepID:     stepID,
		EventType:  eventType,
		Message:    message,
		CreatedAt:  time.Now().UTC(),
	}

	if err := store.AppendEvent(ctx, event); err != nil {
		return fmt.Errorf("htn: failed to append event: %w", err)
	}

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

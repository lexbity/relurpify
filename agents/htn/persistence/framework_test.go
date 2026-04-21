package persistence

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

// TestHTNRunSummaryStructure verifies HTNRunSummary has all required fields.
func TestHTNRunSummaryStructure(t *testing.T) {
	summary := HTNRunSummary{
		SchemaVersion:      1,
		TaskType:           core.TaskType("code"),
		SelectedMethod:     "test_method",
		PlannedStepCount:   5,
		CompletedStepCount: 3,
		TerminationStatus:  "partial",
		TotalDuration:      30,
		RetrievalApplied:   true,
		Success:            false,
		ErrorMessage:       "test error",
	}

	if summary.SchemaVersion != 1 {
		t.Errorf("Expected schema version 1, got %d", summary.SchemaVersion)
	}
	if summary.TaskType != core.TaskType("code") {
		t.Errorf("Expected task type 'code', got %s", summary.TaskType)
	}
	if summary.SelectedMethod != "test_method" {
		t.Error("Selected method not set correctly")
	}
	if summary.PlannedStepCount != 5 {
		t.Errorf("Expected 5 planned steps, got %d", summary.PlannedStepCount)
	}
	if summary.CompletedStepCount != 3 {
		t.Errorf("Expected 3 completed steps, got %d", summary.CompletedStepCount)
	}
	if summary.TotalDuration != 30 {
		t.Errorf("Expected duration 30s, got %d", summary.TotalDuration)
	}
	if !summary.RetrievalApplied {
		t.Error("RetrievalApplied should be true")
	}
	if summary.Success {
		t.Error("Success should be false")
	}
}

// TestOperatorOutcomeStructure verifies OperatorOutcome has all required fields.
func TestOperatorOutcomeStructure(t *testing.T) {
	outcome := OperatorOutcome{
		OperatorName: "analyze",
		StepID:       "step_1",
		TaskType:     core.TaskType("code"),
		Success:      true,
		Duration:     10,
		CostClass:    "fast",
		RetryClass:   "idempotent",
		Retried:      false,
		RetryCount:   0,
		ErrorMessage: "",
		OutputKeys:   []string{"result", "explanation"},
		Metadata: map[string]interface{}{
			"step_run_id": "run_123",
		},
	}

	if outcome.OperatorName != "analyze" {
		t.Errorf("Expected operator 'analyze', got %s", outcome.OperatorName)
	}
	if outcome.StepID != "step_1" {
		t.Errorf("Expected step_id 'step_1', got %s", outcome.StepID)
	}
	if !outcome.Success {
		t.Error("Success should be true")
	}
	if outcome.Duration != 10 {
		t.Errorf("Expected duration 10s, got %d", outcome.Duration)
	}
	if len(outcome.OutputKeys) != 2 {
		t.Errorf("Expected 2 output keys, got %d", len(outcome.OutputKeys))
	}
}

// TestExecutionMetricsStructure verifies ExecutionMetrics has all required fields.
func TestExecutionMetricsStructure(t *testing.T) {
	metrics := ExecutionMetrics{
		SchemaVersion:     1,
		TotalDuration:     45,
		DecompositionTime: 15,
		ExecutionTime:     30,
		PlanStepCount:     5,
		CompletedSteps:    4,
		FailedSteps:       1,
		RetriedSteps:      1,
		AverageCost:       "medium",
		ParallelBranches:  0,
		RetrievalApplied:  true,
		Success:           false,
	}

	if metrics.SchemaVersion != 1 {
		t.Errorf("Expected schema version 1, got %d", metrics.SchemaVersion)
	}
	if metrics.TotalDuration != 45 {
		t.Errorf("Expected total duration 45s, got %d", metrics.TotalDuration)
	}
	if metrics.DecompositionTime != 15 {
		t.Errorf("Expected decomposition time 15s, got %d", metrics.DecompositionTime)
	}
	if metrics.ExecutionTime != 30 {
		t.Errorf("Expected execution time 30s, got %d", metrics.ExecutionTime)
	}
	if metrics.CompletedSteps != 4 {
		t.Errorf("Expected 4 completed steps, got %d", metrics.CompletedSteps)
	}
}

// TestHTNRunSummarySerialization tests JSON marshaling of HTNRunSummary.
func TestHTNRunSummarySerialization(t *testing.T) {
	summary := HTNRunSummary{
		SchemaVersion:      1,
		TaskType:           core.TaskType("code"),
		SelectedMethod:     "test_method",
		PlannedStepCount:   5,
		CompletedStepCount: 4,
		TerminationStatus:  "completed",
		TotalDuration:      30,
		RetrievalApplied:   false,
		Success:            true,
		ErrorMessage:       "",
	}

	jsonStr := marshalHTNRunSummary(summary)
	if jsonStr == "" {
		t.Error("Serialization should not return empty string")
	}
	if len(jsonStr) < 50 {
		t.Errorf("Serialized output too short: %d bytes", len(jsonStr))
	}
}

// TestExecutionMetricsSerialization tests JSON marshaling of ExecutionMetrics.
func TestExecutionMetricsSerialization(t *testing.T) {
	metrics := ExecutionMetrics{
		SchemaVersion:     1,
		TotalDuration:     45,
		DecompositionTime: 15,
		ExecutionTime:     30,
		PlanStepCount:     5,
		CompletedSteps:    4,
		FailedSteps:       1,
		RetriedSteps:      0,
		AverageCost:       "medium",
		ParallelBranches:  0,
		RetrievalApplied:  true,
		Success:           true,
	}

	jsonStr := marshalExecutionMetrics(metrics)
	if jsonStr == "" {
		t.Error("Serialization should not return empty string")
	}
	if len(jsonStr) < 50 {
		t.Errorf("Serialized output too short: %d bytes", len(jsonStr))
	}
}

// TestOperatorOutcomeSerialization tests JSON marshaling of OperatorOutcome.
func TestOperatorOutcomeSerialization(t *testing.T) {
	outcome := OperatorOutcome{
		OperatorName: "analyze",
		StepID:       "step_1",
		TaskType:     core.TaskType("code"),
		Success:      true,
		Duration:     10,
		CostClass:    "fast",
		RetryClass:   "idempotent",
		Retried:      false,
		RetryCount:   0,
		OutputKeys:   []string{"result"},
	}

	jsonStr := marshalOperatorOutcome(outcome)
	if jsonStr == "" {
		t.Error("Serialization should not return empty string")
	}
	if len(jsonStr) < 50 {
		t.Errorf("Serialized output too short: %d bytes", len(jsonStr))
	}
}

// TestSummarizeHTNRun tests the summarization function.
func TestSummarizeHTNRun(t *testing.T) {
	summary := HTNRunSummary{
		TaskType:           core.TaskType("code"),
		SelectedMethod:     "test_method",
		PlannedStepCount:   5,
		CompletedStepCount: 4,
		TotalDuration:      30,
		Success:            true,
	}

	text := summarizeHTNRun(summary)
	if text == "" {
		t.Error("Summary text should not be empty")
	}
	if !contains(text, "SUCCESS") {
		t.Errorf("Summary should contain SUCCESS status, got: %s", text)
	}
	if !contains(text, "test_method") {
		t.Errorf("Summary should contain method name, got: %s", text)
	}
}

// TestSummarizeHTNRunFailed tests summarization for failed run.
func TestSummarizeHTNRunFailed(t *testing.T) {
	summary := HTNRunSummary{
		TaskType:           core.TaskType("code"),
		SelectedMethod:     "test_method",
		PlannedStepCount:   5,
		CompletedStepCount: 2,
		TotalDuration:      30,
		Success:            false,
	}

	text := summarizeHTNRun(summary)
	if !contains(text, "FAILED") {
		t.Errorf("Summary should contain FAILED status, got: %s", text)
	}
}

// TestSummarizeExecutionMetrics tests metrics summarization.
func TestSummarizeExecutionMetrics(t *testing.T) {
	metrics := ExecutionMetrics{
		TotalDuration:     45,
		DecompositionTime: 15,
		ExecutionTime:     30,
		PlanStepCount:     5,
		CompletedSteps:    4,
		RetriedSteps:      1,
	}

	text := summarizeExecutionMetrics(metrics)
	if text == "" {
		t.Error("Metrics text should not be empty")
	}
	if !contains(text, "45") {
		t.Errorf("Summary should contain total duration, got: %s", text)
	}
}

// TestSummarizeOperatorOutcome tests operator outcome summarization.
func TestSummarizeOperatorOutcome(t *testing.T) {
	outcome := OperatorOutcome{
		OperatorName: "analyze",
		StepID:       "step_1",
		Success:      true,
		Duration:     10,
		RetryCount:   0,
		OutputKeys:   []string{"result"},
	}

	text := summarizeOperatorOutcome(outcome)
	if text == "" {
		t.Error("Outcome text should not be empty")
	}
	if !contains(text, "OK") {
		t.Errorf("Summary should contain OK status, got: %s", text)
	}
	if !contains(text, "analyze") {
		t.Errorf("Summary should contain operator name, got: %s", text)
	}
}

// TestOperatorOutcomeMetadata tests metadata map generation.
func TestOperatorOutcomeMetadata(t *testing.T) {
	outcome := OperatorOutcome{
		OperatorName: "analyze",
		StepID:       "step_1",
		TaskType:     core.TaskType("code"),
		Success:      true,
		Duration:     10,
		RetryCount:   2,
		OutputKeys:   []string{"result", "explanation"},
	}

	metadata := operatorOutcomeMetadata(outcome)
	if len(metadata) == 0 {
		t.Error("Metadata map should not be empty")
	}
	if v, ok := metadata["operator_name"]; !ok || v != "analyze" {
		t.Error("Metadata should contain operator_name")
	}
	if v, ok := metadata["success"]; !ok || v != true {
		t.Error("Metadata should contain success flag")
	}
	if v, ok := metadata["duration"]; !ok || v != 10 {
		t.Error("Metadata should contain duration")
	}
}

// TestHTNRunSummaryMetadata tests metadata map generation for run summary.
func TestHTNRunSummaryMetadata(t *testing.T) {
	summary := HTNRunSummary{
		SchemaVersion:      1,
		TaskType:           core.TaskType("code"),
		SelectedMethod:     "test_method",
		PlannedStepCount:   5,
		CompletedStepCount: 4,
		TerminationStatus:  "completed",
		TotalDuration:      30,
		Success:            true,
	}

	metadata := summarizeHTNRunMetadata(summary)
	if len(metadata) == 0 {
		t.Error("Metadata map should not be empty")
	}
	if v, ok := metadata["selected_method"]; !ok || v != "test_method" {
		t.Error("Metadata should contain selected_method")
	}
	if v, ok := metadata["success"]; !ok || v != true {
		t.Error("Metadata should contain success")
	}
}

// TestExecutionMetricsMetadata tests metadata map generation for metrics.
func TestExecutionMetricsMetadata(t *testing.T) {
	metrics := ExecutionMetrics{
		SchemaVersion:     1,
		TotalDuration:     45,
		DecompositionTime: 15,
		ExecutionTime:     30,
		PlanStepCount:     5,
		CompletedSteps:    4,
		FailedSteps:       1,
	}

	metadata := metricsMetadata(metrics)
	if len(metadata) == 0 {
		t.Error("Metadata map should not be empty")
	}
	if v, ok := metadata["total_duration"]; !ok || v != 45 {
		t.Error("Metadata should contain total_duration")
	}
	if v, ok := metadata["completed_steps"]; !ok || v != 4 {
		t.Error("Metadata should contain completed_steps")
	}
}

// TestStatusString tests status string conversion.
func TestStatusString(t *testing.T) {
	if statusString(true) != "completed" {
		t.Error("statusString(true) should return 'completed'")
	}
	if statusString(false) != "failed" {
		t.Error("statusString(false) should return 'failed'")
	}
}

// Mock implementation of WorkflowStateStore for testing
type mockWorkflowStateStore struct {
	artifacts map[string]memory.WorkflowArtifactRecord
	knowledge map[string]memory.KnowledgeRecord
	events    []memory.WorkflowEventRecord
}

func newMockWorkflowStateStore() *mockWorkflowStateStore {
	return &mockWorkflowStateStore{
		artifacts: make(map[string]memory.WorkflowArtifactRecord),
		knowledge: make(map[string]memory.KnowledgeRecord),
		events:    []memory.WorkflowEventRecord{},
	}
}

func (m *mockWorkflowStateStore) UpsertWorkflowArtifact(ctx context.Context, artifact memory.WorkflowArtifactRecord) error {
	m.artifacts[artifact.ArtifactID] = artifact
	return nil
}

func (m *mockWorkflowStateStore) PutKnowledge(ctx context.Context, record memory.KnowledgeRecord) error {
	m.knowledge[record.RecordID] = record
	return nil
}

func (m *mockWorkflowStateStore) UpsertArtifact(ctx context.Context, artifact memory.StepArtifactRecord) error {
	return nil
}

func (m *mockWorkflowStateStore) AppendEvent(ctx context.Context, event memory.WorkflowEventRecord) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockWorkflowStateStore) Close() error {
	return nil
}

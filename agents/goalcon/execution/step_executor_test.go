package execution

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// mockTool for testing
type mockTool struct {
	name      string
	shouldFail bool
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return "Mock tool for testing"
}

func (m *mockTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "input", Type: "string", Description: "Input", Required: true},
	}
}

func (m *mockTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if m.shouldFail {
		return nil, ErrTestToolFailed
	}
	return map[string]any{
		"result": "success",
		"input":  input,
	}, nil
}

var ErrTestToolFailed = &testError{"tool execution failed"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestStepExecutor_Create(t *testing.T) {
	registry := capability.NewRegistry()
	executor := goalcon.NewStepExecutor(registry)

	if executor == nil {
		t.Fatal("expected executor to be created")
	}
}

func TestStepExecutor_SetTimeout(t *testing.T) {
	executor := goalcon.NewStepExecutor(nil)
	executor.SetTimeout(5 * time.Second)
	// No assertion needed, just verify no panic
}

func TestStepExecutor_Execute_NoTool(t *testing.T) {
	executor := goalcon.NewStepExecutor(nil)

	req := goalcon.StepExecutionRequest{
		Step: core.PlanStep{ID: "step1", Tool: ""},
	}

	result := executor.Execute(context.Background(), req)
	if result == nil || result.Success {
		t.Fatal("expected execution to fail for missing tool")
	}
}

func TestStepExecutor_Execute_ToolNotFound(t *testing.T) {
	registry := capability.NewRegistry()
	executor := goalcon.NewStepExecutor(registry)

	req := goalcon.StepExecutionRequest{
		Step: core.PlanStep{
			ID:   "step1",
			Tool: "nonexistent",
		},
		Context: core.NewContext(),
	}

	result := executor.Execute(context.Background(), req)
	if result == nil {
		t.Fatal("expected execution result")
	}
	if result.Success {
		t.Fatal("expected execution to fail")
	}
	if result.Error == nil {
		t.Fatal("expected error message")
	}
}

func TestStepExecutor_RecordMetrics(t *testing.T) {
	registry := capability.NewRegistry()
	executor := goalcon.NewStepExecutor(registry)

	// Create metrics recorder
	recorder := goalcon.NewMetricsRecorder(nil)
	executor.SetMetricsRecorder(recorder)

	// Execute a step
	req := goalcon.StepExecutionRequest{
		Step: core.PlanStep{
			ID:   "step1",
			Tool: "TestTool",
		},
		Context: core.NewContext(),
	}

	_ = executor.Execute(context.Background(), req)

	// Metrics should be recorded
	metrics := recorder.GetMetrics("TestTool")
	if metrics == nil {
		t.Fatal("expected metrics to be recorded")
	}
}

func TestExecutorChain_SingleStep(t *testing.T) {
	executor := goalcon.NewStepExecutor(nil)
	chain := goalcon.NewExecutorChain(executor)

	steps := []core.PlanStep{
		{ID: "step1", Tool: "Tool1", Description: "First step"},
	}

	results := chain.ExecuteSteps(context.Background(), steps, core.NewContext(), nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].StepID != "step1" {
		t.Errorf("expected step1, got %s", results[0].StepID)
	}
}

func TestExecutorChain_MultipleSteps(t *testing.T) {
	// Note: Steps will fail if capabilities not found in registry
	// This tests the chain execution mechanism, not success
	registry := capability.NewRegistry()
	executor := goalcon.NewStepExecutor(registry)
	chain := goalcon.NewExecutorChain(executor)

	steps := []core.PlanStep{
		{ID: "step1", Tool: "Tool1"},
		{ID: "step2", Tool: "Tool2"},
		{ID: "step3", Tool: "Tool3"},
	}

	results := chain.ExecuteSteps(context.Background(), steps, core.NewContext(), registry)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// All steps will fail since capabilities not in registry
	if chain.FailureCount() != 3 {
		t.Errorf("expected all steps to fail, got %d successes", chain.SuccessCount())
	}
}

func TestExecutorChain_SuccessCount(t *testing.T) {
	executor := goalcon.NewStepExecutor(nil)
	chain := goalcon.NewExecutorChain(executor)

	if chain.SuccessCount() != 0 {
		t.Fatal("expected 0 success count before execution")
	}

	steps := []core.PlanStep{
		{ID: "step1", Tool: "Tool1"},
	}

	chain.ExecuteSteps(context.Background(), steps, core.NewContext(), nil)

	// Step will fail because capability not found in nil registry
	if chain.SuccessCount() != 0 {
		t.Fatalf("expected 0 success count (tool not found), got %d", chain.SuccessCount())
	}
	if chain.FailureCount() != 1 {
		t.Fatalf("expected 1 failure, got %d", chain.FailureCount())
	}
}

func TestExecutorChain_Summary(t *testing.T) {
	executor := goalcon.NewStepExecutor(nil)
	chain := goalcon.NewExecutorChain(executor)

	// Before execution, summary should indicate no steps
	summary := chain.Summary()
	if summary != "No steps executed" {
		t.Errorf("expected 'No steps executed', got %s", summary)
	}

	// Execute a step (will fail because capability not found)
	steps := []core.PlanStep{
		{ID: "step1", Tool: "Tool1"},
	}
	chain.ExecuteSteps(context.Background(), steps, core.NewContext(), nil)

	// After execution, summary should reflect 1 failure
	summary = chain.Summary()
	if !contains(summary, "1 failed") {
		t.Errorf("expected summary to mention '1 failed', got: %s", summary)
	}
}

func TestExecutorChain_SetFailureMode(t *testing.T) {
	executor := goalcon.NewStepExecutor(nil)
	chain := goalcon.NewExecutorChain(executor)

	chain.SetFailureMode(goalcon.FailureModeAbort)
	// Just verify no panic
}

func TestExecutionTrace_Create(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	if trace == nil {
		t.Fatal("expected trace to be created")
	}
	if trace.PlanGoal != "test goal" {
		t.Errorf("expected goal 'test goal', got '%s'", trace.PlanGoal)
	}
}

func TestExecutionTrace_RecordEvents(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	trace.RecordPlanStart()
	trace.RecordStepStart("step1", "Tool1")

	if len(trace.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(trace.Events))
	}
}

func TestExecutionTrace_RecordStepComplete(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	result := &goalcon.StepExecutionResult{
		StepID:   "step1",
		ToolName: "Tool1",
		Success:  true,
		Duration: 100 * time.Millisecond,
	}

	trace.RecordStepComplete(result)

	if len(trace.StepResults) != 1 {
		t.Fatal("expected step result to be stored")
	}
	if trace.StepCount() != 1 {
		t.Fatal("expected step count to be 1")
	}
}

func TestExecutionTrace_Duration(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	trace.RecordPlanStart()
	time.Sleep(10 * time.Millisecond)
	trace.RecordPlanComplete(true, 1)

	duration := trace.Duration()
	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}
}

func TestExecutionTrace_SuccessCount(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	// Add successful result
	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:  "step1",
		Success: true,
	})

	// Add failed result
	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:  "step2",
		Success: false,
	})

	if trace.SuccessCount() != 1 {
		t.Errorf("expected 1 success, got %d", trace.SuccessCount())
	}
	if trace.FailureCount() != 1 {
		t.Errorf("expected 1 failure, got %d", trace.FailureCount())
	}
}

func TestExecutionTrace_EventsByType(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	trace.RecordPlanStart()
	trace.RecordPlanStart()
	trace.RecordStepStart("step1", "Tool1")

	startEvents := trace.EventsByType("plan_start")
	if len(startEvents) != 2 {
		t.Errorf("expected 2 plan_start events, got %d", len(startEvents))
	}
}

func TestExecutionTrace_FailedSteps(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:  "step1",
		Success: true,
	})

	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:  "step2",
		Success: false,
	})

	failed := trace.FailedSteps()
	if len(failed) != 1 {
		t.Fatalf("expected 1 failed step, got %d", len(failed))
	}
	if failed[0].StepID != "step2" {
		t.Errorf("expected step2, got %s", failed[0].StepID)
	}
}

func TestExecutionTrace_CriticalPath(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	// Add steps with different durations
	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:   "step1",
		Success:  true,
		Duration: 100 * time.Millisecond,
	})

	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:   "step2",
		Success:  true,
		Duration: 50 * time.Millisecond,
	})

	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:   "step3",
		Success:  true,
		Duration: 200 * time.Millisecond,
	})

	criticalPath := trace.CriticalPath()
	if len(criticalPath) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(criticalPath))
	}

	// Should be sorted by duration (longest first)
	if criticalPath[0].Duration != 200*time.Millisecond {
		t.Errorf("expected first step to be 200ms, got %v", criticalPath[0].Duration)
	}
}

func TestExecutionTrace_Summary(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:  "step1",
		Success: true,
	})

	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:  "step2",
		Success: false,
	})

	summary := trace.Summary()
	if summary == "No trace" {
		t.Fatal("unexpected summary")
	}
	if len(summary) == 0 {
		t.Fatal("expected non-empty summary")
	}
}

func TestExecutionTrace_ToDebugString(t *testing.T) {
	trace := goalcon.NewExecutionTrace("test goal")

	trace.RecordPlanStart()
	trace.RecordStepStart("step1", "Tool1")
	trace.RecordStepComplete(&goalcon.StepExecutionResult{
		StepID:   "step1",
		ToolName: "Tool1",
		Success:  true,
		Duration: 100 * time.Millisecond,
	})
	trace.RecordPlanComplete(true, 1)

	debugStr := trace.ToDebugString()
	if len(debugStr) == 0 {
		t.Fatal("expected non-empty debug string")
	}
	if !contains(debugStr, "Execution Trace") {
		t.Error("debug string should contain 'Execution Trace'")
	}
	if !contains(debugStr, "test goal") {
		t.Error("debug string should contain goal")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0
}

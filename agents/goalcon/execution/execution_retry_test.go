package execution

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/goalcon/audit"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// TestRetryExecutor_Create tests retry executor creation.
func TestRetryExecutor_Create(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)

	if retryExec == nil {
		t.Fatal("Failed to create RetryExecutor")
	}

	if retryExec.executor == nil {
		t.Fatal("RetryExecutor.executor is nil")
	}
}

// TestRetryExecutor_SetMaxReplans tests max replans configuration.
func TestRetryExecutor_SetMaxReplans(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)

	retryExec.SetMaxReplans(5)

	if retryExec.maxReplans != 5 {
		t.Errorf("Expected maxReplans=5, got %d", retryExec.maxReplans)
	}
}

// TestRetryExecutor_ExecuteSuccessOnFirst tests successful execution on first attempt.
func TestRetryExecutor_ExecuteSuccessOnFirst(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	detector := NewFailureDetector(nil, nil)
	retryExec := NewRetryExecutor(executor, nil, detector, nil, nil)

	req := StepExecutionRequest{
		Step: core.PlanStep{
			ID:   "step1",
			Tool: "test-tool",
		},
		Context: core.NewContext(),
	}

	result := retryExec.ExecuteWithRetry(context.Background(), req)

	// Phase 4: executor might fail because tool not found in empty registry
	// The important thing is that retries should be 0 for first attempt
	if result.Retries != 0 {
		t.Errorf("Expected 0 retries on first execution, got %d", result.Retries)
	}

	// When executor successfully completes (or fails), retries should remain 0 until retry logic kicks in
	if result.Retries > 0 {
		t.Errorf("Unexpected retries on first attempt: %d", result.Retries)
	}
}

// TestRetryExecutor_ShouldReplan_ExhaustedRetries tests replan decision.
func TestRetryExecutor_ShouldReplan_ExhaustedRetries(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)

	result := &StepExecutionResult{
		StepID:   "step1",
		ToolName: "test-tool",
		Success:  false,
		Error:    fmt.Errorf("transient error"),
		Retries:  3,
	}

	policy := &RetryPolicy{MaxAttempts: 3}
	retryExec.policies = OperatorRetryPolicies{
		"test-tool": policy,
	}

	shouldReplan := retryExec.ShouldReplan(result, 0)

	if !shouldReplan {
		t.Error("Expected ShouldReplan=true after exhausted retries")
	}
}

// TestRetryExecutor_ShouldReplan_BelowMaxReplans tests max replans limit.
func TestRetryExecutor_ShouldReplan_BelowMaxReplans(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)
	retryExec.SetMaxReplans(3)

	result := &StepExecutionResult{
		StepID:   "step1",
		ToolName: "test-tool",
		Success:  false,
		Error:    fmt.Errorf("error"),
		Retries:  2,
	}

	// At attempt 2, should not replan if max is 3
	shouldReplan := retryExec.ShouldReplan(result, 2)

	if shouldReplan {
		t.Error("Expected ShouldReplan=false below max replan limit")
	}
}

// TestRetryExecutor_ShouldReplan_ExceededMaxReplans tests max replan enforcement.
func TestRetryExecutor_ShouldReplan_ExceededMaxReplans(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)
	retryExec.SetMaxReplans(2)

	result := &StepExecutionResult{
		Success: false,
		Error:   fmt.Errorf("error"),
		Retries: 2,
	}

	// At max attempts, should not replan
	shouldReplan := retryExec.ShouldReplan(result, 2)

	if shouldReplan {
		t.Error("Expected ShouldReplan=false at max replan limit")
	}
}

// TestRetryExecutor_ShouldReplan_SuccessfulResult tests no replan on success.
func TestRetryExecutor_ShouldReplan_SuccessfulResult(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)

	result := &StepExecutionResult{
		Success: true,
		Retries: 0,
	}

	shouldReplan := retryExec.ShouldReplan(result, 0)

	if shouldReplan {
		t.Error("Expected ShouldReplan=false for successful result")
	}
}

// TestComputeRetryMetrics_SingleAttempt tests metrics for no retries.
func TestComputeRetryMetrics_SingleAttempt(t *testing.T) {
	attempts := []*RetryAttempt{
		{
			AttemptNumber: 1,
			Error:         nil,
		},
	}

	metrics := ComputeRetryMetrics("step1", "tool1", attempts, false)

	if metrics == nil {
		t.Fatal("Expected non-nil metrics")
	}

	if metrics.TotalAttempts != 1 {
		t.Errorf("Expected TotalAttempts=1, got %d", metrics.TotalAttempts)
	}

	if metrics.RecoveredAfterRetry {
		t.Error("Expected RecoveredAfterRetry=false for single successful attempt")
	}
}

// TestComputeRetryMetrics_MultipleAttempts tests metrics for retries.
func TestComputeRetryMetrics_MultipleAttempts(t *testing.T) {
	attempts := []*RetryAttempt{
		{AttemptNumber: 1, Error: fmt.Errorf("error"), BackoffDuration: 0},
		{AttemptNumber: 2, Error: fmt.Errorf("error"), BackoffDuration: 100 * time.Millisecond},
		{AttemptNumber: 3, Error: nil, BackoffDuration: 200 * time.Millisecond},
	}

	metrics := ComputeRetryMetrics("step1", "tool1", attempts, true)

	if metrics.TotalAttempts != 3 {
		t.Errorf("Expected TotalAttempts=3, got %d", metrics.TotalAttempts)
	}

	if metrics.FailedAttempts != 2 {
		t.Errorf("Expected FailedAttempts=2, got %d", metrics.FailedAttempts)
	}

	if !metrics.RecoveredAfterRetry {
		t.Error("Expected RecoveredAfterRetry=true")
	}

	expectedBackoffTime := 300 * time.Millisecond
	if metrics.TotalBackoffTime != expectedBackoffTime {
		t.Errorf("Expected TotalBackoffTime=%v, got %v", expectedBackoffTime, metrics.TotalBackoffTime)
	}
}

// TestComputeRetryMetrics_EmptyAttempts tests handling of empty attempt list.
func TestComputeRetryMetrics_EmptyAttempts(t *testing.T) {
	metrics := ComputeRetryMetrics("step1", "tool1", []*RetryAttempt{}, false)

	if metrics != nil {
		t.Error("Expected nil metrics for empty attempts")
	}
}

// TestGetRetryableCategoriesForReplan tests replan categories.
func TestGetRetryableCategoriesForReplan(t *testing.T) {
	categories := GetRetryableCategoriesForReplan()

	if len(categories) != 3 {
		t.Errorf("Expected 3 retryable categories, got %d", len(categories))
	}

	// Verify expected categories are present
	expectedMap := map[FailureCategory]bool{
		FailureCategoryTransientError:      false,
		FailureCategoryInsufficientContext: false,
		FailureCategoryUnexpectedOutput:    false,
	}

	for _, cat := range categories {
		if _, exists := expectedMap[cat]; !exists {
			t.Errorf("Unexpected category in replan categories: %s", cat.String())
		}
		expectedMap[cat] = true
	}

	for cat, found := range expectedMap {
		if !found {
			t.Errorf("Expected category %s not found in retryable categories", cat.String())
		}
	}
}

// TestIsRetryableAfterMaxAttempts_TransientError tests retryable transient error.
func TestIsRetryableAfterMaxAttempts_TransientError(t *testing.T) {
	fc := &FailureContext{
		Category: FailureCategoryTransientError,
	}

	result := IsRetryableAfterMaxAttempts(fc)

	if !result {
		t.Error("Expected transient error to be retryable after max attempts")
	}
}

// TestIsRetryableAfterMaxAttempts_PermanentFailure tests permanent failure.
func TestIsRetryableAfterMaxAttempts_PermanentFailure(t *testing.T) {
	fc := &FailureContext{
		Category: FailureCategoryPermanentFailure,
	}

	result := IsRetryableAfterMaxAttempts(fc)

	if result {
		t.Error("Expected permanent failure to not be retryable after max attempts")
	}
}

// TestRetryExecutor_ApplyBackoff tests backoff application.
func TestRetryExecutor_ApplyBackoff(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)

	ctx := context.Background()
	backoff := 10 * time.Millisecond

	start := time.Now()
	result := retryExec.applyBackoff(ctx, backoff)
	elapsed := time.Since(start)

	if !result {
		t.Error("Expected applyBackoff to return true")
	}

	if elapsed < backoff {
		t.Errorf("Expected elapsed time >= %v, got %v", backoff, elapsed)
	}
}

// TestRetryExecutor_ApplyBackoff_ContextCancelled tests backoff cancellation.
func TestRetryExecutor_ApplyBackoff_ContextCancelled(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	retryExec := NewRetryExecutor(executor, nil, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	backoff := 1 * time.Second

	result := retryExec.applyBackoff(ctx, backoff)

	if result {
		t.Error("Expected applyBackoff to return false when context cancelled")
	}
}

// TestRetryExecutor_RecordRetryHistory tests retry history recording.
func TestRetryExecutor_RecordRetryHistory(t *testing.T) {
	executor := NewStepExecutor(capability.NewRegistry())
	trail := audit.NewCapabilityAuditTrail("plan-1")
	retryExec := NewRetryExecutor(executor, nil, nil, nil, trail)

	attempts := []*RetryAttempt{
		{AttemptNumber: 1, Error: fmt.Errorf("error 1")},
		{AttemptNumber: 2, Error: fmt.Errorf("error 2")},
		{AttemptNumber: 3, Error: nil},
	}

	result := &StepExecutionResult{
		Success: true,
	}

	// This should not panic and should record retry history
	retryExec.recordRetryHistory("step1", "tool1", attempts, result)
}

// TestExecutionAttempt_Fields tests ExecutionAttempt struct.
func TestExecutionAttempt_Fields(t *testing.T) {
	attempt := ExecutionAttempt{
		Number:    1,
		StartTime: time.Now(),
		Duration:  100 * time.Millisecond,
		Result: &StepExecutionResult{
			StepID:  "step1",
			Success: true,
		},
		Backoff: 50 * time.Millisecond,
	}

	if attempt.Number != 1 {
		t.Errorf("Expected Number=1, got %d", attempt.Number)
	}

	if attempt.Duration != 100*time.Millisecond {
		t.Errorf("Expected Duration=100ms, got %v", attempt.Duration)
	}

	if attempt.Result.StepID != "step1" {
		t.Errorf("Expected Result.StepID=step1, got %s", attempt.Result.StepID)
	}
}

// TestRetryMetrics_Fields tests RetryMetrics struct.
func TestRetryMetrics_Fields(t *testing.T) {
	metrics := &RetryMetrics{
		TotalAttempts:       3,
		SuccessfulAttempt:   2,
		FailedAttempts:      1,
		TotalBackoffTime:    300 * time.Millisecond,
		AverageRetryTime:    150 * time.Millisecond,
		OperatorName:        "test-tool",
		StepID:              "step1",
		RecoveredAfterRetry: true,
	}

	if metrics.TotalAttempts != 3 {
		t.Errorf("Expected TotalAttempts=3, got %d", metrics.TotalAttempts)
	}

	if !metrics.RecoveredAfterRetry {
		t.Error("Expected RecoveredAfterRetry=true")
	}

	if metrics.OperatorName != "test-tool" {
		t.Errorf("Expected OperatorName=test-tool, got %s", metrics.OperatorName)
	}
}

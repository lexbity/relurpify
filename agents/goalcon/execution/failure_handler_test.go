package execution

import (
	"fmt"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents/goalcon/audit"
)

// TestFailureDetector_CategorizesTransientError tests detection of transient errors.
func TestFailureDetector_CategorizesTransientError(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	result := &StepExecutionResult{
		StepID:   "step1",
		ToolName: "fetch-data",
		Success:  false,
		Error:    fmt.Errorf("connection timeout"),
	}

	fc := detector.AnalyzeStepFailure(result)
	if fc == nil {
		t.Fatal("Expected failure context")
	}

	if fc.Category != FailureCategoryTransientError {
		t.Errorf("Expected TransientError, got %s", fc.Category)
	}

	if fc.Suggestion == "" {
		t.Error("Expected suggestion for transient error")
	}
}

// TestFailureDetector_CategorizesPermanentFailure tests detection of permanent failures.
func TestFailureDetector_CategorizesPermanentFailure(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	result := &StepExecutionResult{
		StepID:   "step2",
		ToolName: "invalid-tool",
		Success:  false,
		Error:    fmt.Errorf("capability not found"),
	}

	fc := detector.AnalyzeStepFailure(result)
	if fc == nil {
		t.Fatal("Expected failure context")
	}

	if fc.Category != FailureCategoryPermanentFailure {
		t.Errorf("Expected PermanentFailure, got %s", fc.Category)
	}
}

// TestFailureDetector_RateLimitError tests transient rate limit detection.
func TestFailureDetector_RateLimitError(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	result := &StepExecutionResult{
		StepID:   "step3",
		ToolName: "api-call",
		Success:  false,
		Error:    fmt.Errorf("HTTP 429 Too Many Requests: rate limit exceeded"),
	}

	fc := detector.AnalyzeStepFailure(result)
	if fc == nil {
		t.Fatal("Expected failure context")
	}

	if fc.Category != FailureCategoryTransientError {
		t.Errorf("Expected TransientError for rate limit, got %s", fc.Category)
	}
}

// TestFailureDetector_DeadlineError tests transient deadline detection.
func TestFailureDetector_DeadlineError(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	result := &StepExecutionResult{
		StepID:   "step4",
		ToolName: "long-running",
		Success:  false,
		Error:    fmt.Errorf("context deadline exceeded"),
	}

	fc := detector.AnalyzeStepFailure(result)
	if fc == nil {
		t.Fatal("Expected failure context")
	}

	if fc.Category != FailureCategoryTransientError {
		t.Errorf("Expected TransientError for deadline, got %s", fc.Category)
	}
}

// TestFailureDetector_GetRecoveryStrategy_TransientError tests recovery for transient error.
func TestFailureDetector_GetRecoveryStrategy_TransientError(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	fc := &FailureContext{
		Category:   FailureCategoryTransientError,
		RetryCount: 0,
	}

	policy := DefaultRetryPolicy()
	strategy := detector.GetRecoveryStrategy(fc, policy)

	if strategy != RecoveryStrategyRetry {
		t.Errorf("Expected Retry for transient error, got %s", strategy)
	}
}

// TestFailureDetector_GetRecoveryStrategy_ExhaustedRetries tests recovery when retries exhausted.
func TestFailureDetector_GetRecoveryStrategy_ExhaustedRetries(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	fc := &FailureContext{
		Category:   FailureCategoryTransientError,
		RetryCount: 3,
	}

	policy := &RetryPolicy{MaxAttempts: 3}
	strategy := detector.GetRecoveryStrategy(fc, policy)

	if strategy != RecoveryStrategyReplan {
		t.Errorf("Expected Replan after exhausted retries, got %s", strategy)
	}
}

// TestFailureDetector_GetRecoveryStrategy_InsufficientContext tests recovery strategy for insufficient context.
func TestFailureDetector_GetRecoveryStrategy_InsufficientContext(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	fc := &FailureContext{
		Category: FailureCategoryInsufficientContext,
	}

	policy := DefaultRetryPolicy()
	strategy := detector.GetRecoveryStrategy(fc, policy)

	if strategy != RecoveryStrategyReplan {
		t.Errorf("Expected Replan for insufficient context, got %s", strategy)
	}
}

// TestFailureDetector_GetRecoveryStrategy_PermanentFailure tests abort strategy.
func TestFailureDetector_GetRecoveryStrategy_PermanentFailure(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	fc := &FailureContext{
		Category: FailureCategoryPermanentFailure,
	}

	policy := DefaultRetryPolicy()
	strategy := detector.GetRecoveryStrategy(fc, policy)

	if strategy != RecoveryStrategyAbort {
		t.Errorf("Expected Abort for permanent failure, got %s", strategy)
	}
}

// TestFailureDetector_ShouldRetry tests the ShouldRetry logic.
func TestFailureDetector_ShouldRetry(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	tests := []struct {
		name        string
		fc          *FailureContext
		policy      *RetryPolicy
		shouldRetry bool
	}{
		{
			name:        "Transient error within retry limit",
			fc:          &FailureContext{Category: FailureCategoryTransientError, RetryCount: 0},
			policy:      DefaultRetryPolicy(),
			shouldRetry: true,
		},
		{
			name:        "Transient error beyond retry limit",
			fc:          &FailureContext{Category: FailureCategoryTransientError, RetryCount: 3},
			policy:      &RetryPolicy{MaxAttempts: 3},
			shouldRetry: false,
		},
		{
			name:        "Permanent failure",
			fc:          &FailureContext{Category: FailureCategoryPermanentFailure, RetryCount: 0},
			policy:      DefaultRetryPolicy(),
			shouldRetry: false,
		},
		{
			name:        "Nil failure context",
			fc:          nil,
			policy:      DefaultRetryPolicy(),
			shouldRetry: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := detector.ShouldRetry(test.fc, test.policy)
			if result != test.shouldRetry {
				t.Errorf("Expected %v, got %v", test.shouldRetry, result)
			}
		})
	}
}

// TestFailureCategory_String tests string representation.
func TestFailureCategory_String(t *testing.T) {
	tests := map[FailureCategory]string{
		FailureCategoryTransientError:      "transient_error",
		FailureCategoryPermanentFailure:    "permanent_failure",
		FailureCategoryInsufficientContext: "insufficient_context",
		FailureCategoryUnexpectedOutput:    "unexpected_output",
	}

	for category, expectedStr := range tests {
		result := category.String()
		if result != expectedStr {
			t.Errorf("Category %d: expected %q, got %q", category, expectedStr, result)
		}
	}
}

// TestRecoveryStrategy_String tests string representation.
func TestRecoveryStrategy_String(t *testing.T) {
	tests := map[RecoveryStrategy]string{
		RecoveryStrategyRetry:  "retry",
		RecoveryStrategyReplan: "replan",
		RecoveryStrategyAbort:  "abort",
		RecoveryStrategyHITL:   "hitl",
	}

	for strategy, expectedStr := range tests {
		result := strategy.String()
		if result != expectedStr {
			t.Errorf("Strategy %d: expected %q, got %q", strategy, expectedStr, result)
		}
	}
}

// TestFailureDetector_WithMetrics tests failure detection with metrics context.
func TestFailureDetector_WithMetrics(t *testing.T) {
	// Create a metrics recorder (without memory store for testing)
	recorder := audit.NewMetricsRecorder(nil)

	// Record some execution metrics
	recorder.RecordExecution(audit.ExecutionMetrics{
		OperatorName: "reliable-tool",
		Success:      true,
		Duration:     100 * time.Millisecond,
	})
	for i := 0; i < 8; i++ {
		recorder.RecordExecution(audit.ExecutionMetrics{
			OperatorName: "reliable-tool",
			Success:      true,
			Duration:     100 * time.Millisecond,
		})
	}
	recorder.RecordExecution(audit.ExecutionMetrics{
		OperatorName: "reliable-tool",
		Success:      false,
		Duration:     100 * time.Millisecond,
	})

	// Flaky tool with low success rate
	for i := 0; i < 2; i++ {
		recorder.RecordExecution(audit.ExecutionMetrics{
			OperatorName: "flaky-tool",
			Success:      true,
			Duration:     100 * time.Millisecond,
		})
	}
	for i := 0; i < 8; i++ {
		recorder.RecordExecution(audit.ExecutionMetrics{
			OperatorName: "flaky-tool",
			Success:      false,
			Duration:     100 * time.Millisecond,
		})
	}

	detector := NewFailureDetector(recorder, nil)

	// Test reliable tool failure
	result1 := &StepExecutionResult{
		StepID:   "step1",
		ToolName: "reliable-tool",
		Success:  false,
		Error:    fmt.Errorf("unknown error"),
	}

	fc1 := detector.AnalyzeStepFailure(result1)
	if fc1.SuccessRate < 0.8 {
		t.Errorf("Expected success rate ~0.9 for reliable tool, got %.2f", fc1.SuccessRate)
	}

	// Test flaky tool failure
	result2 := &StepExecutionResult{
		StepID:   "step2",
		ToolName: "flaky-tool",
		Success:  false,
		Error:    fmt.Errorf("unknown error"),
	}

	fc2 := detector.AnalyzeStepFailure(result2)
	if fc2.SuccessRate > 0.3 {
		t.Errorf("Expected success rate ~0.2 for flaky tool, got %.2f", fc2.SuccessRate)
	}

	// Flaky tool should trigger replan suggestion
	if fc2.Category != FailureCategoryInsufficientContext {
		t.Errorf("Expected InsufficientContext for flaky tool, got %s", fc2.Category)
	}
}

// TestFailureDetector_FormatFailureReport tests report generation.
func TestFailureDetector_FormatFailureReport(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	fc := &FailureContext{
		StepID:       "step1",
		ToolName:     "read-file",
		CapabilityID: "read-file",
		Error:        fmt.Errorf("file not found"),
		Category:     FailureCategoryPermanentFailure,
		SuccessRate:  0.95,
		RetryCount:   0,
		Suggestion:   "File path is incorrect",
	}

	report := detector.FormatFailureReport(fc)

	// Verify report contains key information
	if report == "" {
		t.Fatal("Expected non-empty report")
	}

	requiredFields := []string{"step1", "read-file", "permanent_failure", "file not found"}
	for _, field := range requiredFields {
		if !contains(report, field) {
			t.Errorf("Report missing field: %s", field)
		}
	}
}

// TestFailureContext_CreatedWithCorrectTimestamp tests failure context timestamp.
func TestFailureContext_CreatedWithCorrectTimestamp(t *testing.T) {
	detector := NewFailureDetector(nil, nil)

	executedAt := time.Now().UTC()
	result := &StepExecutionResult{
		StepID:     "step1",
		ToolName:   "test",
		Success:    false,
		Error:      fmt.Errorf("test error"),
		ExecutedAt: executedAt,
	}

	fc := detector.AnalyzeStepFailure(result)

	// Failure context should use the executed time from the result
	if fc.Timestamp != executedAt {
		t.Errorf("Expected timestamp=%v, got %v", executedAt, fc.Timestamp)
	}
}

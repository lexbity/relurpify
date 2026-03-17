package features

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// TestIdempotentRecoveryStrategy tests direct retry strategy.
func TestIdempotentRecoveryStrategy(t *testing.T) {
	strategy := &IdempotentRecoveryStrategy{}

	if strategy.Name() != "idempotent" {
		t.Errorf("Expected name 'idempotent', got %s", strategy.Name())
	}

	// Should apply for idempotent retry class
	if !strategy.CanApply("step_1", RetryClassIdempotent, 0, 3) {
		t.Error("Should apply for idempotent retry class")
	}

	// Should not apply for other retry classes
	if strategy.CanApply("step_1", RetryClassNone, 0, 3) {
		t.Error("Should not apply for none retry class")
	}

	// Should not apply if max attempts exceeded
	if strategy.CanApply("step_1", RetryClassIdempotent, 3, 3) {
		t.Error("Should not apply if max attempts exceeded")
	}

	// Execute should succeed
	err := strategy.Execute(context.Background(), core.NewContext(), "step_1", nil)
	if err != nil {
		t.Errorf("Execute should not error, got %v", err)
	}
}

// TestStatelessRecoveryStrategy tests state reset strategy.
func TestStatelessRecoveryStrategy(t *testing.T) {
	strategy := &StatelessRecoveryStrategy{}

	if strategy.Name() != "stateless" {
		t.Errorf("Expected name 'stateless', got %s", strategy.Name())
	}

	// Should apply for stateless retry class
	if !strategy.CanApply("step_1", RetryClassStateless, 0, 3) {
		t.Error("Should apply for stateless retry class")
	}

	// Should not apply for other retry classes
	if strategy.CanApply("step_1", RetryClassIdempotent, 0, 3) {
		t.Error("Should not apply for idempotent retry class")
	}

	// Execute should clear state
	state := core.NewContext()
	state.Set("operator_state.step_1", "some_state")
	state.Set("step_history.step_1", "some_history")

	err := strategy.Execute(context.Background(), state, "step_1", nil)
	if err != nil {
		t.Errorf("Execute should not error, got %v", err)
	}

	// State should be cleared (set to nil)
	if val, ok := state.Get("operator_state.step_1"); ok {
		if val != nil {
			t.Error("Operator state should be cleared to nil")
		}
	}
}

// TestProbedRecoveryStrategy tests precondition checking strategy.
func TestProbedRecoveryStrategy(t *testing.T) {
	strategy := &ProbedRecoveryStrategy{
		Prober: func(ctx context.Context, state *core.Context, stepID string) (bool, error) {
			return true, nil
		},
	}

	if strategy.Name() != "probed" {
		t.Errorf("Expected name 'probed', got %s", strategy.Name())
	}

	// Should apply for probed retry class
	if !strategy.CanApply("step_1", RetryClassProbed, 0, 3) {
		t.Error("Should apply for probed retry class")
	}

	// Should not apply for other retry classes
	if strategy.CanApply("step_1", RetryClassStateless, 0, 3) {
		t.Error("Should not apply for stateless retry class")
	}

	// Execute should succeed when preconditions met
	err := strategy.Execute(context.Background(), core.NewContext(), "step_1", nil)
	if err != nil {
		t.Errorf("Execute should succeed when preconditions met, got %v", err)
	}
}

// TestProbedRecoveryStrategyFailedPreconditions tests failed precondition check.
func TestProbedRecoveryStrategyFailedPreconditions(t *testing.T) {
	strategy := &ProbedRecoveryStrategy{
		Prober: func(ctx context.Context, state *core.Context, stepID string) (bool, error) {
			return false, nil
		},
	}

	err := strategy.Execute(context.Background(), core.NewContext(), "step_1", nil)
	if err == nil {
		t.Error("Execute should error when preconditions not met")
	}
	if err.Error() != "preconditions not met for retry" {
		t.Errorf("Expected preconditions error, got %v", err)
	}
}

// TestAdaptiveTimeoutCalculator calculates timeouts.
func TestAdaptiveTimeoutCalculator(t *testing.T) {
	calculator := &AdaptiveTimeoutCalculator{
		BaseTimeout:      5 * time.Second,
		SafetyMultiplier: 2.0,
		MaxTimeout:       5 * time.Minute,
	}

	// With metrics
	metrics := &OperatorMetricsSnapshot{
		AverageDuration: 10, // 10 seconds
	}

	timeout := calculator.CalculateTimeout(metrics)
	expected := 20 * time.Second // 10 * 2.0
	if timeout != expected {
		t.Errorf("Expected timeout %v, got %v", expected, timeout)
	}

	// Respects minimum
	minMetrics := &OperatorMetricsSnapshot{
		AverageDuration: 1, // 1 second
	}
	minTimeout := calculator.CalculateTimeout(minMetrics)
	if minTimeout < calculator.BaseTimeout {
		t.Errorf("Timeout should not go below base timeout")
	}

	// Respects maximum
	maxMetrics := &OperatorMetricsSnapshot{
		AverageDuration: 300, // 300 seconds
	}
	maxTimeout := calculator.CalculateTimeout(maxMetrics)
	if maxTimeout > calculator.MaxTimeout {
		t.Errorf("Timeout should not exceed max timeout")
	}

	// Without metrics, uses base timeout
	baseTimeout := calculator.CalculateTimeout(nil)
	if baseTimeout != calculator.BaseTimeout {
		t.Errorf("Expected base timeout without metrics")
	}
}

// TestFileRecoveryScope extracts scope from metadata.
func TestFileRecoveryScope(t *testing.T) {
	focus := &FileFocus{
		Primary:   []string{"main.go"},
		Secondary: []string{"config.go"},
		Patterns:  []string{"*.go"},
		Exclude:   []string{"vendor/*"},
	}

	scope := ExtractRecoveryScopeFromMetadata(focus, 100)

	if len(scope.PrimaryFiles) != 1 {
		t.Errorf("Expected 1 primary file, got %d", len(scope.PrimaryFiles))
	}
	if scope.PrimaryFiles[0] != "main.go" {
		t.Errorf("Expected main.go, got %s", scope.PrimaryFiles[0])
	}
	if scope.MaxFiles != 100 {
		t.Errorf("Expected max files 100, got %d", scope.MaxFiles)
	}
}

// TestFileRecoveryScopeNil handles nil focus.
func TestFileRecoveryScopeNil(t *testing.T) {
	scope := ExtractRecoveryScopeFromMetadata(nil, 50)

	if scope == nil {
		t.Error("Expected non-nil scope")
	}
	if scope.MaxFiles != 50 {
		t.Errorf("Expected max files 50, got %d", scope.MaxFiles)
	}
}

// TestVerificationContext verifies context structure.
func TestVerificationContext(t *testing.T) {
	hint := &VerificationHint{
		Description: "Check output",
		Criteria:    []string{"file_exists"},
	}

	vctx := &VerificationContext{
		StepID:       "step_1",
		OperatorName: "analyze",
		Hint:         hint,
		Timeout:      30 * time.Second,
	}

	if vctx.StepID != "step_1" {
		t.Errorf("Expected step_1, got %s", vctx.StepID)
	}
	if vctx.Timeout != 30*time.Second {
		t.Errorf("Expected 30s timeout, got %v", vctx.Timeout)
	}
}

// TestHintBasedVerificationStrategy verifies using hints.
func TestHintBasedVerificationStrategy(t *testing.T) {
	strategy := &HintBasedVerificationStrategy{}

	if strategy.Name() != "hint_based" {
		t.Errorf("Expected name 'hint_based', got %s", strategy.Name())
	}

	// Without hint, should succeed
	vctx := &VerificationContext{
		StepID: "step_1",
	}
	success, issues := strategy.Verify(context.Background(), core.NewContext(), vctx)
	if !success {
		t.Error("Should succeed without hint")
	}
	if len(issues) > 0 {
		t.Errorf("Expected no issues, got %v", issues)
	}

	// With hint, should check criteria
	vctx.Hint = &VerificationHint{
		Criteria: []string{"output_exists"},
	}
	success, issues = strategy.Verify(context.Background(), core.NewContext(), vctx)
	if !success {
		t.Error("Should succeed with valid criteria")
	}
}

// TestResultBasedVerificationStrategy verifies using results.
func TestResultBasedVerificationStrategy(t *testing.T) {
	strategy := &ResultBasedVerificationStrategy{}

	if strategy.Name() != "result_based" {
		t.Errorf("Expected name 'result_based', got %s", strategy.Name())
	}

	// Without result, should fail
	vctx := &VerificationContext{
		StepID: "step_1",
	}
	success, issues := strategy.Verify(context.Background(), core.NewContext(), vctx)
	if success {
		t.Error("Should fail without result")
	}

	// With successful result, should succeed
	vctx.ExecutionResult = &core.Result{
		Success: true,
		Data:    map[string]any{"output": "value"},
	}
	success, issues = strategy.Verify(context.Background(), core.NewContext(), vctx)
	if !success {
		t.Errorf("Should succeed with successful result, issues: %v", issues)
	}

	// With failed result, should fail
	vctx.ExecutionResult.Success = false
	success, issues = strategy.Verify(context.Background(), core.NewContext(), vctx)
	if success {
		t.Error("Should fail with failed result")
	}
}

// TestRecoveryPolicyEngineDefault creates default engine.
func TestRecoveryPolicyEngineDefault(t *testing.T) {
	engine := NewRecoveryPolicyEngine()

	if engine == nil {
		t.Error("Expected non-nil engine")
	}
	if len(engine.Strategies) == 0 {
		t.Error("Expected strategies in default engine")
	}
	if len(engine.VerificationStrategies) == 0 {
		t.Error("Expected verification strategies in default engine")
	}
	if engine.MaxRecoveryAttempts != 3 {
		t.Errorf("Expected 3 max attempts, got %d", engine.MaxRecoveryAttempts)
	}
}

// TestDetermineRecoveryAction selects appropriate strategy.
func TestDetermineRecoveryAction(t *testing.T) {
	engine := NewRecoveryPolicyEngine()

	// Idempotent should select idempotent strategy
	strategy, err := engine.DetermineRecoveryAction("step_1", RetryClassIdempotent, 0, nil)
	if err != nil {
		t.Errorf("Should find strategy, got %v", err)
	}
	if strategy == nil {
		t.Error("Expected strategy, got nil")
	}
	if strategy.Name() != "idempotent" {
		t.Errorf("Expected idempotent strategy, got %s", strategy.Name())
	}

	// None should return error
	strategy, err = engine.DetermineRecoveryAction("step_1", RetryClassNone, 0, nil)
	if err == nil {
		t.Error("Should error for retry class none")
	}

	// Max attempts exceeded should return error
	strategy, err = engine.DetermineRecoveryAction("step_1", RetryClassIdempotent, 3, nil)
	if err == nil {
		t.Error("Should error for max attempts exceeded")
	}
}

// TestApplyRecoveryStrategy executes strategy.
func TestApplyRecoveryStrategy(t *testing.T) {
	engine := NewRecoveryPolicyEngine()
	strategy := &IdempotentRecoveryStrategy{}

	err := engine.ApplyRecoveryStrategy(
		context.Background(),
		core.NewContext(),
		strategy,
		"step_1",
		fmt.Errorf("test error"),
	)

	if err != nil {
		t.Errorf("Should apply strategy without error, got %v", err)
	}
}

// TestApplyRecoveryStrategyNil handles nil strategy.
func TestApplyRecoveryStrategyNil(t *testing.T) {
	engine := NewRecoveryPolicyEngine()

	err := engine.ApplyRecoveryStrategy(
		context.Background(),
		core.NewContext(),
		nil,
		"step_1",
		nil,
	)

	if err == nil {
		t.Error("Should error for nil strategy")
	}
}

// TestVerifyStepCompletion verifies step completion.
func TestVerifyStepCompletion(t *testing.T) {
	engine := NewRecoveryPolicyEngine()

	vctx := &VerificationContext{
		StepID:          "step_1",
		ExecutionResult: &core.Result{Success: true, Data: map[string]any{"output": "value"}},
	}

	success, issues := engine.VerifyStepCompletion(context.Background(), core.NewContext(), vctx)
	if !success {
		t.Errorf("Should verify successful execution, issues: %v", issues)
	}

	// Failed result
	vctx.ExecutionResult.Success = false
	success, issues = engine.VerifyStepCompletion(context.Background(), core.NewContext(), vctx)
	if success {
		t.Error("Should fail for unsuccessful execution")
	}
}

// TestVerifyStepCompletionNil handles nil context.
func TestVerifyStepCompletionNil(t *testing.T) {
	engine := NewRecoveryPolicyEngine()

	success, issues := engine.VerifyStepCompletion(context.Background(), core.NewContext(), nil)
	if success {
		t.Error("Should fail for nil verification context")
	}
	if len(issues) == 0 {
		t.Error("Expected issues for nil context")
	}
}

// TestRecoveryContextBuilder builds recovery context.
func TestRecoveryContextBuilder(t *testing.T) {
	builder := &RecoveryContextBuilder{
		WorkflowID: "workflow_1",
	}

	metadata := OperatorMetadata{
		RetryClass: RetryClassIdempotent,
		CostClass:  CostClassMedium,
		BranchSafe: true,
		VerificationHint: &VerificationHint{
			Description: "Check output",
		},
	}

	ctx := builder.BuildRecoveryContext(
		context.Background(),
		"step_1",
		"analyze",
		metadata,
		fmt.Errorf("test error"),
		1,
	)

	if ctx == nil {
		t.Error("Expected recovery context")
	}
	if ctx["step_id"] != "step_1" {
		t.Errorf("Expected step_id step_1, got %v", ctx["step_id"])
	}
	if ctx["retry_class"] != "idempotent" {
		t.Errorf("Expected idempotent retry class, got %v", ctx["retry_class"])
	}
}

// TestRecoveryMetricsRecording records metrics.
func TestRecoveryMetricsRecording(t *testing.T) {
	metrics := &RecoveryMetrics{
		StepID: "step_1",
	}

	// First attempt success
	RecordRecoveryAttempt(metrics, 2*time.Second, true, nil)

	if metrics.TotalAttempts != 1 {
		t.Errorf("Expected 1 total attempt, got %d", metrics.TotalAttempts)
	}
	if metrics.SuccessfulAttempts != 1 {
		t.Errorf("Expected 1 successful attempt, got %d", metrics.SuccessfulAttempts)
	}
	if metrics.FailedAttempts != 0 {
		t.Errorf("Expected 0 failed attempts, got %d", metrics.FailedAttempts)
	}

	// Second attempt failure
	RecordRecoveryAttempt(metrics, 3*time.Second, false, fmt.Errorf("test error"))

	if metrics.TotalAttempts != 2 {
		t.Errorf("Expected 2 total attempts, got %d", metrics.TotalAttempts)
	}
	if metrics.FailedAttempts != 1 {
		t.Errorf("Expected 1 failed attempt, got %d", metrics.FailedAttempts)
	}
	if metrics.LastRecoveryError != "test error" {
		t.Errorf("Expected test error, got %s", metrics.LastRecoveryError)
	}

	// Average should be calculated
	expectedAverage := (2*time.Second + 3*time.Second) / 2
	if metrics.AverageRecoveryTime != expectedAverage {
		t.Errorf("Expected average %v, got %v", expectedAverage, metrics.AverageRecoveryTime)
	}
}

// TestRecoveryMetricsRecordingNil handles nil metrics.
func TestRecoveryMetricsRecordingNil(t *testing.T) {
	// Should not panic with nil metrics
	RecordRecoveryAttempt(nil, 2*time.Second, true, nil)
}

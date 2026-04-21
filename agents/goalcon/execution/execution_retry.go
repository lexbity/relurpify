package execution

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/agents/goalcon/audit"
	"codeburg.org/lexbit/relurpify/framework/capability"
)

// RetryExecutor wraps StepExecutor with retry logic and backoff.
type RetryExecutor struct {
	executor        *StepExecutor
	policies        OperatorRetryPolicies
	failureDetector *FailureDetector
	metrics         *audit.MetricsRecorder
	auditTrail      *audit.CapabilityAuditTrail
	maxReplans      int
}

// NewRetryExecutor creates a new retry executor.
func NewRetryExecutor(
	executor *StepExecutor,
	policies OperatorRetryPolicies,
	failureDetector *FailureDetector,
	metrics *audit.MetricsRecorder,
	auditTrail *audit.CapabilityAuditTrail,
) *RetryExecutor {
	if executor == nil {
		executor = NewStepExecutor(capability.NewRegistry())
	}
	if policies == nil {
		policies = make(OperatorRetryPolicies)
	}
	if failureDetector == nil {
		failureDetector = NewFailureDetector(metrics, auditTrail)
	}

	return &RetryExecutor{
		executor:        executor,
		policies:        policies,
		failureDetector: failureDetector,
		metrics:         metrics,
		auditTrail:      auditTrail,
		maxReplans:      3,
	}
}

// SetMaxReplans sets the maximum number of re-planning attempts.
func (re *RetryExecutor) SetMaxReplans(max int) {
	if re != nil && max > 0 {
		re.maxReplans = max
	}
}

// ExecuteWithRetry executes a step with retry logic and backoff.
func (re *RetryExecutor) ExecuteWithRetry(
	ctx context.Context,
	req StepExecutionRequest,
) *StepExecutionResult {
	if re == nil || re.executor == nil {
		return &StepExecutionResult{
			Success: false,
			Error:   fmt.Errorf("retry executor not initialized"),
		}
	}

	// Get retry policy for this operator
	policy := PolicyForOperator(req.Step.Tool, re.policies)

	// Track retry attempts
	var attempts []*RetryAttempt
	var finalResult *StepExecutionResult

	// Initial attempt
	finalResult = re.executor.Execute(ctx, req)
	attempts = append(attempts, &RetryAttempt{
		AttemptNumber: 1,
		Timestamp:     time.Now().UTC(),
		Error:         finalResult.Error,
	})

	// Retry loop
	calculator := NewBackoffCalculator(policy)
	retryCount := 0

	for !finalResult.Success && retryCount < policy.MaxAttempts {
		// Analyze failure to determine if we should retry
		failureCtx := re.failureDetector.AnalyzeStepFailure(finalResult)
		if !re.failureDetector.ShouldRetry(failureCtx, policy) {
			break
		}

		// Compute backoff
		backoff := calculator.NextBackoff()
		retryCount++

		// Apply backoff with context cancellation support
		if !re.applyBackoff(ctx, backoff) {
			// Context cancelled during backoff
			finalResult.Error = ctx.Err()
			break
		}

		// Retry execution
		finalResult = re.executor.Execute(ctx, req)
		finalResult.Retries = retryCount

		attempts = append(attempts, &RetryAttempt{
			AttemptNumber:   retryCount + 1,
			BackoffDuration: backoff,
			Timestamp:       time.Now().UTC(),
			Error:           finalResult.Error,
		})
	}

	// Record retry history
	if retryCount > 0 {
		re.recordRetryHistory(req.Step.ID, req.Step.Tool, attempts, finalResult)
	}

	return finalResult
}

// applyBackoff sleeps for the specified duration, respecting context cancellation.
func (re *RetryExecutor) applyBackoff(ctx context.Context, backoff time.Duration) bool {
	select {
	case <-time.After(backoff):
		return true // Sleep completed normally
	case <-ctx.Done():
		return false // Context cancelled
	}
}

// recordRetryHistory records retry attempts to audit trail (if available).
func (re *RetryExecutor) recordRetryHistory(
	stepID, toolName string,
	attempts []*RetryAttempt,
	finalResult *StepExecutionResult,
) {
	if re == nil || len(attempts) <= 1 {
		return // No retries
	}

	// Build retry summary for audit trail metadata
	summary := fmt.Sprintf(
		"Step %s (%s) required %d attempts: ",
		stepID,
		toolName,
		len(attempts),
	)

	for i, attempt := range attempts {
		if i > 0 {
			summary += fmt.Sprintf(
				" → Attempt %d (after %v backoff)",
				attempt.AttemptNumber,
				attempt.BackoffDuration,
			)
		} else {
			summary += fmt.Sprintf("Initial attempt")
		}
		if attempt.Error != nil {
			summary += fmt.Sprintf(" (error: %v)", attempt.Error)
		}
	}

	if finalResult.Success {
		summary += " [RECOVERED]"
	} else {
		summary += " [FAILED]"
	}

	// Optionally log summary (could also attach to audit trail metadata)
	_ = summary
}

// ShouldReplan determines if a failed step should trigger re-planning.
func (re *RetryExecutor) ShouldReplan(
	result *StepExecutionResult,
	replanAttempts int,
) bool {
	if re == nil || result == nil {
		return false
	}

	// Don't exceed max replans
	if replanAttempts >= re.maxReplans {
		return false
	}

	// Don't replan if step succeeded
	if result.Success {
		return false
	}

	// Replan if we exhausted retries
	policy := PolicyForOperator(result.ToolName, re.policies)
	if result.Retries >= policy.MaxAttempts {
		return true
	}

	return false
}

// ExecutionAttempt represents a single execution attempt.
type ExecutionAttempt struct {
	Number    int
	StartTime time.Time
	Duration  time.Duration
	Result    *StepExecutionResult
	Backoff   time.Duration
}

// RetryMetrics captures aggregated retry statistics.
type RetryMetrics struct {
	TotalAttempts       int
	SuccessfulAttempt   int
	FailedAttempts      int
	TotalBackoffTime    time.Duration
	AverageRetryTime    time.Duration
	OperatorName        string
	StepID              string
	RecoveredAfterRetry bool
}

// ComputeRetryMetrics calculates metrics from a list of attempts.
func ComputeRetryMetrics(
	stepID, operatorName string,
	attempts []*RetryAttempt,
	recovered bool,
) *RetryMetrics {
	if len(attempts) == 0 {
		return nil
	}

	metrics := &RetryMetrics{
		TotalAttempts:       len(attempts),
		StepID:              stepID,
		OperatorName:        operatorName,
		RecoveredAfterRetry: recovered,
	}

	var totalTime time.Duration
	for _, attempt := range attempts {
		totalTime += attempt.BackoffDuration
		if attempt.Error != nil {
			metrics.FailedAttempts++
		}
	}

	if metrics.TotalAttempts > 1 {
		metrics.SuccessfulAttempt = metrics.TotalAttempts - metrics.FailedAttempts
		metrics.AverageRetryTime = totalTime / time.Duration(metrics.TotalAttempts-1)
	}
	metrics.TotalBackoffTime = totalTime

	return metrics
}

// GetRetryableCategoriesForReplan returns failure categories that should trigger re-planning.
func GetRetryableCategoriesForReplan() []FailureCategory {
	return []FailureCategory{
		FailureCategoryTransientError,
		FailureCategoryInsufficientContext,
		FailureCategoryUnexpectedOutput,
	}
}

// IsRetryableAfterMaxAttempts checks if a failure is recoverable via re-planning.
func IsRetryableAfterMaxAttempts(fc *FailureContext) bool {
	if fc == nil {
		return false
	}

	// These categories benefit from re-planning (trying different operator)
	replanCategories := GetRetryableCategoriesForReplan()
	for _, cat := range replanCategories {
		if fc.Category == cat {
			return true
		}
	}

	return false
}

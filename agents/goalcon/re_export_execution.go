package goalcon

import (
	"codeburg.org/lexbit/relurpify/agents/goalcon/execution"
)

// Re-exports from execution package for backward compatibility
type StepExecutor = execution.StepExecutor
type StepExecutionRequest = execution.StepExecutionRequest
type StepExecutionResult = execution.StepExecutionResult
type FailureDetector = execution.FailureDetector
type FailureContext = execution.FailureContext
type FailureCategory = execution.FailureCategory
type RecoveryStrategy = execution.RecoveryStrategy
type RetryPolicy = execution.RetryPolicy
type BackoffCalculator = execution.BackoffCalculator
type OperatorRetryPolicies = execution.OperatorRetryPolicies
type RetryExecutor = execution.RetryExecutor
type ExecutionAttempt = execution.ExecutionAttempt
type RetryMetrics = execution.RetryMetrics
type RetryAttempt = execution.RetryAttempt
type ExecutionTrace = execution.ExecutionTrace
type ExecutionEvent = execution.ExecutionEvent

// Re-exported constructors and constants
var (
	NewStepExecutor                 = execution.NewStepExecutor
	NewFailureDetector              = execution.NewFailureDetector
	DefaultRetryPolicy              = execution.DefaultRetryPolicy
	PolicyForOperator               = execution.PolicyForOperator
	ComputeRetryMetrics             = execution.ComputeRetryMetrics
	GetRetryableCategoriesForReplan = execution.GetRetryableCategoriesForReplan
	IsRetryableAfterMaxAttempts     = execution.IsRetryableAfterMaxAttempts
	NewBackoffCalculator            = execution.NewBackoffCalculator
	NewRetryExecutor                = execution.NewRetryExecutor
	NewExecutionTrace               = execution.NewExecutionTrace
)

const (
	FailureCategoryTransientError      = execution.FailureCategoryTransientError
	FailureCategoryPermanentFailure    = execution.FailureCategoryPermanentFailure
	FailureCategoryInsufficientContext = execution.FailureCategoryInsufficientContext
	FailureCategoryUnexpectedOutput    = execution.FailureCategoryUnexpectedOutput
	RecoveryStrategyRetry              = execution.RecoveryStrategyRetry
	RecoveryStrategyReplan             = execution.RecoveryStrategyReplan
	RecoveryStrategyAbort              = execution.RecoveryStrategyAbort
	RecoveryStrategyHITL               = execution.RecoveryStrategyHITL
)

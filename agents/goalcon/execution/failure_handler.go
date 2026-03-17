package execution

import (
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/goalcon/audit"
)

// FailureCategory classifies the nature of a failure.
type FailureCategory int

const (
	FailureCategoryTransientError    FailureCategory = iota // Retry candidate (timeout, rate-limit, network)
	FailureCategoryPermanentFailure                         // No retry (capability missing, invalid params)
	FailureCategoryInsufficientContext                       // Need world state update
	FailureCategoryUnexpectedOutput                          // Result contradicts expectations
)

// String returns the string representation of a FailureCategory.
func (fc FailureCategory) String() string {
	switch fc {
	case FailureCategoryTransientError:
		return "transient_error"
	case FailureCategoryPermanentFailure:
		return "permanent_failure"
	case FailureCategoryInsufficientContext:
		return "insufficient_context"
	case FailureCategoryUnexpectedOutput:
		return "unexpected_output"
	default:
		return "unknown"
	}
}

// FailureContext provides detailed information about a step failure.
type FailureContext struct {
	StepID        string
	ToolName      string
	CapabilityID  string
	Error         error
	Category      FailureCategory
	SuccessRate   float64   // Historical success rate of this operator (0.0-1.0)
	RetryCount    int       // Attempts so far
	LastError     string    // Previous error message
	Suggestion    string    // Recovery suggestion
	Timestamp     time.Time
}

// RecoveryStrategy determines how to recover from a failure.
type RecoveryStrategy int

const (
	RecoveryStrategyRetry   RecoveryStrategy = iota // Retry the step
	RecoveryStrategyReplan                          // Re-plan without this operator
	RecoveryStrategyAbort                           // Give up, operator failed permanently
	RecoveryStrategyHITL                            // Escalate to human
)

// String returns the string representation of a RecoveryStrategy.
func (rs RecoveryStrategy) String() string {
	switch rs {
	case RecoveryStrategyRetry:
		return "retry"
	case RecoveryStrategyReplan:
		return "replan"
	case RecoveryStrategyAbort:
		return "abort"
	case RecoveryStrategyHITL:
		return "hitl"
	default:
		return "unknown"
	}
}

// FailureDetector analyzes step results and categorizes failures.
type FailureDetector struct {
	metrics    *audit.MetricsRecorder
	auditTrail *audit.CapabilityAuditTrail
}

// NewFailureDetector creates a new failure detector.
func NewFailureDetector(metrics *audit.MetricsRecorder, auditTrail *audit.CapabilityAuditTrail) *FailureDetector {
	return &FailureDetector{
		metrics:    metrics,
		auditTrail: auditTrail,
	}
}

// AnalyzeStepFailure categorizes a step execution failure.
func (fd *FailureDetector) AnalyzeStepFailure(result *StepExecutionResult) *FailureContext {
	if result == nil || result.Success {
		return nil
	}

	fc := &FailureContext{
		StepID:      result.StepID,
		ToolName:    result.ToolName,
		CapabilityID: result.CapabilityID,
		Error:       result.Error,
		RetryCount:  result.Retries,
		Timestamp:   result.ExecutedAt,
	}

	// Get historical success rate from metrics
	if fd.metrics != nil {
		metrics := fd.metrics.GetMetrics(result.ToolName)
		if metrics != nil {
			successCount := float64(metrics.SuccessfulCount)
			totalCount := float64(metrics.SuccessfulCount + metrics.FailedCount)
			if totalCount > 0 {
				fc.SuccessRate = successCount / totalCount
			}
		}
	}

	// Categorize failure based on error message
	if result.Error != nil {
		errMsg := strings.ToLower(result.Error.Error())
		fc.Category, fc.Suggestion = fd.categorizeError(errMsg, result.ToolName, fc.SuccessRate)
	} else {
		fc.Category = FailureCategoryPermanentFailure
		fc.Suggestion = "Step failed without error message"
	}

	return fc
}

// categorizeError analyzes error messages to categorize failures.
func (fd *FailureDetector) categorizeError(errMsg, toolName string, successRate float64) (FailureCategory, string) {
	// Transient error patterns
	transientPatterns := []string{
		"timeout",
		"deadline",
		"connection reset",
		"connection refused",
		"network unreachable",
		"rate limit",
		"too many requests",
		"temporarily unavailable",
		"try again",
		"resource busy",
		"timed out",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errMsg, pattern) {
			return FailureCategoryTransientError, "Transient error detected; retry with backoff"
		}
	}

	// Permanent failure patterns
	permanentPatterns := []string{
		"not found",
		"no such",
		"invalid parameter",
		"invalid argument",
		"permission denied",
		"access denied",
		"unauthorized",
		"forbidden",
		"syntax error",
		"malformed",
		"unsupported",
	}

	for _, pattern := range permanentPatterns {
		if strings.Contains(errMsg, pattern) {
			return FailureCategoryPermanentFailure, "Permanent failure; operator cannot succeed with current inputs"
		}
	}

	// Check if operator generally fails (low success rate)
	if successRate < 0.3 {
		return FailureCategoryInsufficientContext,
			"types.Operator has low success rate (<30%); skip and replan without this operator"
	}

	// Default to permanent failure for unknown errors
	return FailureCategoryPermanentFailure, "Unknown error; treating as permanent failure"
}

// GetRecoveryStrategy determines how to recover based on failure context.
func (fd *FailureDetector) GetRecoveryStrategy(fc *FailureContext, retryPolicy *RetryPolicy) RecoveryStrategy {
	if fc == nil {
		return RecoveryStrategyAbort
	}

	switch fc.Category {
	case FailureCategoryTransientError:
		// Retry transient errors up to policy limit
		if fc.RetryCount < retryPolicy.MaxAttempts {
			return RecoveryStrategyRetry
		}
		// If retries exhausted, try replan
		return RecoveryStrategyReplan

	case FailureCategoryInsufficientContext:
		// Always replan for insufficient context
		return RecoveryStrategyReplan

	case FailureCategoryUnexpectedOutput:
		// Replan for unexpected output
		return RecoveryStrategyReplan

	case FailureCategoryPermanentFailure:
		// Abort permanent failures
		return RecoveryStrategyAbort

	default:
		return RecoveryStrategyAbort
	}
}

// ShouldRetry determines if a step should be retried.
func (fd *FailureDetector) ShouldRetry(fc *FailureContext, policy *RetryPolicy) bool {
	if fc == nil || policy == nil {
		return false
	}

	// Don't retry permanent failures
	if fc.Category == FailureCategoryPermanentFailure {
		return false
	}

	// Don't exceed max attempts
	if fc.RetryCount >= policy.MaxAttempts {
		return false
	}

	// Transient errors should be retried
	if fc.Category == FailureCategoryTransientError {
		return true
	}

	// Low-success operators might be retried once
	if fc.Category == FailureCategoryInsufficientContext && fc.RetryCount == 0 {
		return true
	}

	return false
}

// SuggestAlternativeOperators recommends alternatives to a failed operator.
// This method will be used in Phase 6+ to guide re-planning.
func (fd *FailureDetector) SuggestAlternativeOperators(fc *FailureContext, registry interface{}) []interface{} {
	if fc == nil || registry == nil {
		return nil
	}

	// Get all operators that could satisfy the same predicates
	// This is a placeholder; actual implementation would query registry
	// for operators that provide the same effects
	return nil
}

// FormatFailureReport generates a human-readable failure report.
func (fd *FailureDetector) FormatFailureReport(fc *FailureContext) string {
	if fc == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Step: " + fc.StepID + "\n")
	sb.WriteString("Tool: " + fc.ToolName + " (" + fc.CapabilityID + ")\n")
	sb.WriteString("Category: " + fc.Category.String() + "\n")
	sb.WriteString("Error: " + fc.Error.Error() + "\n")
	sb.WriteString("Retry Count: " + string(rune(fc.RetryCount)) + "\n")
	sb.WriteString("Success Rate: " + formatPercent(fc.SuccessRate) + "%\n")
	sb.WriteString("Suggestion: " + fc.Suggestion + "\n")

	return sb.String()
}

// formatPercent converts a float (0.0-1.0) to a percentage string.
func formatPercent(value float64) string {
	if value < 0 {
		return "0"
	}
	if value > 1 {
		return "100"
	}
	// Format as whole number percentage
	percent := int(value * 100)
	switch {
	case percent <= 0:
		return "0"
	case percent >= 100:
		return "100"
	default:
		return string(rune('0' + percent/10)) + string(rune('0'+(percent%10)))
	}
}

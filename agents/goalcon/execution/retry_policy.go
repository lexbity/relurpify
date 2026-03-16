package execution

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryPolicy defines retry behavior for an operator or step.
type RetryPolicy struct {
	MaxAttempts       int           // Maximum number of retry attempts (not including initial attempt)
	InitialBackoff    time.Duration // Initial backoff duration
	MaxBackoff        time.Duration // Maximum backoff duration
	BackoffMultiplier float64       // Exponential backoff growth factor (e.g., 1.5x)
	JitterFraction    float64       // Random jitter as fraction of backoff (0.0-1.0)
}

// BackoffCalculator computes retry delays with exponential backoff and jitter.
type BackoffCalculator struct {
	policy  *RetryPolicy
	attempt int
}

// NewBackoffCalculator creates a new backoff calculator.
func NewBackoffCalculator(policy *RetryPolicy) *BackoffCalculator {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}
	return &BackoffCalculator{
		policy:  policy,
		attempt: 0,
	}
}

// NextBackoff computes the backoff duration for the next retry.
func (bc *BackoffCalculator) NextBackoff() time.Duration {
	if bc == nil || bc.policy == nil {
		return 100 * time.Millisecond
	}

	bc.attempt++

	// Compute base backoff with exponential growth
	baseDuration := float64(bc.policy.InitialBackoff)
	multiplier := math.Pow(bc.policy.BackoffMultiplier, float64(bc.attempt-1))
	computedBackoff := time.Duration(baseDuration * multiplier)

	// Cap at max backoff
	maxBackoff := bc.policy.MaxBackoff
	if computedBackoff > maxBackoff {
		computedBackoff = maxBackoff
	}

	// Add jitter
	if bc.policy.JitterFraction > 0 {
		jitter := bc.ComputeJitter(computedBackoff)
		computedBackoff = computedBackoff + jitter
	}

	return computedBackoff
}

// ComputeJitter adds random jitter to a duration.
func (bc *BackoffCalculator) ComputeJitter(baseDuration time.Duration) time.Duration {
	if baseDuration <= 0 || bc.policy.JitterFraction <= 0 {
		return 0
	}

	jitterMs := int64(baseDuration.Milliseconds()) * int64(bc.policy.JitterFraction*100) / 100
	if jitterMs <= 0 {
		return 0
	}

	// Random jitter: +/- jitterMs
	randomJitter := rand.Int63n(2*jitterMs) - jitterMs
	return time.Duration(randomJitter) * time.Millisecond
}

// Reset resets the attempt counter for a new retry cycle.
func (bc *BackoffCalculator) Reset() {
	if bc != nil {
		bc.attempt = 0
	}
}

// RetryAttempt tracks a single retry attempt.
type RetryAttempt struct {
	AttemptNumber    int
	BackoffDuration  time.Duration
	Timestamp        time.Time
	Error            error
	CategoryDetected FailureCategory
}

// OperatorRetryPolicies maps operator names to their retry policies.
type OperatorRetryPolicies map[string]*RetryPolicy

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 1.5,
		JitterFraction:    0.1, // 10% jitter
	}
}

// PolicyForOperator retrieves or creates a retry policy for an operator.
func PolicyForOperator(operatorName string, customPolicies OperatorRetryPolicies) *RetryPolicy {
	// Check custom policies first
	if customPolicies != nil {
		if policy, exists := customPolicies[operatorName]; exists && policy != nil {
			return policy
		}
	}

	// Use operator-category-specific defaults
	switch {
	case isFileIOOperator(operatorName):
		// File I/O is slow, use longer backoff
		return &RetryPolicy{
			MaxAttempts:       5,
			InitialBackoff:    200 * time.Millisecond,
			MaxBackoff:        60 * time.Second,
			BackoffMultiplier: 1.8,
			JitterFraction:    0.15,
		}

	case isNetworkOperator(operatorName):
		// Network calls: aggressive backoff with jitter to prevent thundering herd
		return &RetryPolicy{
			MaxAttempts:       4,
			InitialBackoff:    150 * time.Millisecond,
			MaxBackoff:        45 * time.Second,
			BackoffMultiplier: 1.6,
			JitterFraction:    0.2, // Higher jitter for network
		}

	case isLLMOperator(operatorName):
		// LLM calls are typically quick, use shorter backoff
		return &RetryPolicy{
			MaxAttempts:       2,
			InitialBackoff:    50 * time.Millisecond,
			MaxBackoff:        10 * time.Second,
			BackoffMultiplier: 1.5,
			JitterFraction:    0.1,
		}

	default:
		// Default policy for unknown operators
		return DefaultRetryPolicy()
	}
}

// isFileIOOperator checks if operator name suggests file I/O.
func isFileIOOperator(name string) bool {
	patterns := []string{"file", "read", "write", "mkdir", "rm", "copy", "move", "ls", "cat"}
	for _, pattern := range patterns {
		if contains(name, pattern) {
			return true
		}
	}
	return false
}

// isNetworkOperator checks if operator name suggests network operations.
func isNetworkOperator(name string) bool {
	patterns := []string{"http", "fetch", "download", "upload", "request", "api", "curl", "wget"}
	for _, pattern := range patterns {
		if contains(name, pattern) {
			return true
		}
	}
	return false
}

// isLLMOperator checks if operator name suggests LLM interactions.
func isLLMOperator(name string) bool {
	patterns := []string{"llm", "model", "prompt", "completion", "classify", "generate"}
	for _, pattern := range patterns {
		if contains(name, pattern) {
			return true
		}
	}
	return false
}

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	sLower := toLower(s)
	subLower := toLower(substr)
	for i := 0; i+len(subLower) <= len(sLower); i++ {
		if sLower[i:i+len(subLower)] == subLower {
			return true
		}
	}
	return false
}

// toLower converts a string to lowercase without using strings package.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		b[i] = c
	}
	return string(b)
}

// FormatRetryPolicy returns a human-readable description of a retry policy.
func FormatRetryPolicy(policy *RetryPolicy) string {
	if policy == nil {
		return "nil policy"
	}

	return fmt.Sprintf(
		"RetryPolicy{max_attempts=%d, initial_backoff=%v, max_backoff=%v, multiplier=%.1f, jitter=%.0f%%}",
		policy.MaxAttempts,
		policy.InitialBackoff,
		policy.MaxBackoff,
		policy.BackoffMultiplier,
		policy.JitterFraction*100,
	)
}

package execution

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/retry"
)

type RetryPolicy = retry.Policy

type RetryAttempt struct {
	AttemptNumber    int
	BackoffDuration  time.Duration
	Timestamp        time.Time
	Error            error
	CategoryDetected FailureCategory
}

type OperatorRetryPolicies map[string]*retry.Policy

func PolicyForOperator(operatorName string, customPolicies OperatorRetryPolicies) *retry.Policy {
	if customPolicies != nil {
		if policy, exists := customPolicies[operatorName]; exists && policy != nil {
			return policy
		}
	}

	switch {
	case isFileIOOperator(operatorName):
		return &retry.Policy{
			MaxAttempts:       5,
			InitialBackoff:    200 * time.Millisecond,
			MaxBackoff:        60 * time.Second,
			BackoffMultiplier: 1.8,
			JitterFraction:    0.15,
		}

	case isNetworkOperator(operatorName):
		return &retry.Policy{
			MaxAttempts:       4,
			InitialBackoff:    150 * time.Millisecond,
			MaxBackoff:        45 * time.Second,
			BackoffMultiplier: 1.6,
			JitterFraction:    0.2,
		}

	case isLLMOperator(operatorName):
		return &retry.Policy{
			MaxAttempts:       2,
			InitialBackoff:    50 * time.Millisecond,
			MaxBackoff:        10 * time.Second,
			BackoffMultiplier: 1.5,
			JitterFraction:    0.1,
		}

	default:
		p := retry.DefaultPolicy()
		return &p
	}
}

func isFileIOOperator(name string) bool {
	patterns := []string{"file", "read", "write", "mkdir", "rm", "copy", "move", "ls", "cat"}
	for _, pattern := range patterns {
		if strings.Contains(strings.ToLower(name), pattern) {
			return true
		}
	}
	return false
}

func isNetworkOperator(name string) bool {
	patterns := []string{"http", "fetch", "download", "upload", "request", "api", "curl", "wget"}
	for _, pattern := range patterns {
		if strings.Contains(strings.ToLower(name), pattern) {
			return true
		}
	}
	return false
}

func isLLMOperator(name string) bool {
	patterns := []string{"llm", "model", "prompt", "completion", "classify", "generate"}
	for _, pattern := range patterns {
		if strings.Contains(strings.ToLower(name), pattern) {
			return true
		}
	}
	return false
}

func FormatRetryPolicy(policy *retry.Policy) string {
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

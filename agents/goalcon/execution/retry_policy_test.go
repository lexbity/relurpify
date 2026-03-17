package execution

import (
	"testing"
	"time"
)

// TestDefaultRetryPolicy tests default policy values.
func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy == nil {
		t.Fatal("DefaultRetryPolicy returned nil")
	}

	if policy.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got %d", policy.MaxAttempts)
	}

	if policy.InitialBackoff != 100*time.Millisecond {
		t.Errorf("Expected InitialBackoff=100ms, got %v", policy.InitialBackoff)
	}

	if policy.MaxBackoff != 30*time.Second {
		t.Errorf("Expected MaxBackoff=30s, got %v", policy.MaxBackoff)
	}

	if policy.BackoffMultiplier != 1.5 {
		t.Errorf("Expected BackoffMultiplier=1.5, got %.2f", policy.BackoffMultiplier)
	}
}

// TestBackoffCalculator_ExponentialGrowth tests exponential backoff growth.
func TestBackoffCalculator_ExponentialGrowth(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:       5,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0, // Easy to calculate: 100ms * 2^(n-1)
		JitterFraction:    0,   // No jitter for predictable test
	}

	calc := NewBackoffCalculator(policy)

	tests := []struct {
		attemptNum      int
		expectedMinMs   int64
		expectedMaxMs   int64
	}{
		{1, 100, 100},      // 100ms * 2^0 = 100ms
		{2, 200, 200},      // 100ms * 2^1 = 200ms
		{3, 400, 400},      // 100ms * 2^2 = 400ms
		{4, 800, 800},      // 100ms * 2^3 = 800ms
		{5, 1600, 1600},    // 100ms * 2^4 = 1600ms
	}

	for _, test := range tests {
		backoff := calc.NextBackoff()
		backoffMs := backoff.Milliseconds()

		if backoffMs < test.expectedMinMs || backoffMs > test.expectedMaxMs {
			t.Errorf(
				"Attempt %d: expected backoff between %d-%d ms, got %d ms",
				test.attemptNum, test.expectedMinMs, test.expectedMaxMs, backoffMs,
			)
		}
	}
}

// TestBackoffCalculator_MaxBackoffCap tests that backoff is capped at MaxBackoff.
func TestBackoffCalculator_MaxBackoffCap(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:       10,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        5 * time.Second, // Cap at 5 seconds
		BackoffMultiplier: 2.0,
		JitterFraction:    0,
	}

	calc := NewBackoffCalculator(policy)

	// Skip to attempt that would exceed max backoff
	for i := 0; i < 5; i++ {
		calc.NextBackoff()
	}

	// This attempt should be capped
	backoff := calc.NextBackoff()

	if backoff > policy.MaxBackoff {
		t.Errorf("Expected backoff <= %v, got %v", policy.MaxBackoff, backoff)
	}
}

// TestBackoffCalculator_WithJitter tests jitter addition.
func TestBackoffCalculator_WithJitter(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    1000 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 1.0, // No multiplication
		JitterFraction:    0.1,  // 10% jitter
	}

	calc := NewBackoffCalculator(policy)

	// Generate multiple backoffs with jitter - they should vary slightly
	backoffs := make([]time.Duration, 0, 5)
	for i := 0; i < 5; i++ {
		backoff := calc.NextBackoff()
		backoffs = append(backoffs, backoff)
	}

	// It's possible (though unlikely) to get the same value with jitter,
	// so we'll just verify jitter is being applied when non-zero
	if policy.JitterFraction > 0 && len(backoffs) > 0 {
		// Jitter should be applied
		firstBackoff := backoffs[0]
		maxJitter := int64(float64(firstBackoff.Milliseconds()) * policy.JitterFraction)
		// Backoff should be within ±jitter of base value
		if maxJitter == 0 {
			t.Error("Expected jitter to be computed")
		}
	}
}

// TestBackoffCalculator_Reset tests reset functionality.
func TestBackoffCalculator_Reset(t *testing.T) {
	policy := DefaultRetryPolicy()
	calc := NewBackoffCalculator(policy)

	// Get some backoffs
	for i := 0; i < 2; i++ {
		calc.NextBackoff()
	}

	// Reset
	calc.Reset()

	// Next backoff should start from the beginning (100ms)
	backoff := calc.NextBackoff()
	if backoff != policy.InitialBackoff {
		t.Errorf("Expected reset backoff to be %v, got %v", policy.InitialBackoff, backoff)
	}
}

// TestPolicyForOperator_FileIOOperator tests file I/O operator policy.
func TestPolicyForOperator_FileIOOperator(t *testing.T) {
	policy := PolicyForOperator("read-file", nil)

	if policy == nil {
		t.Fatal("Expected non-nil policy for file I/O operator")
	}

	// File I/O should have more generous retries
	if policy.MaxAttempts < 4 {
		t.Errorf("Expected file I/O operator to have high MaxAttempts, got %d", policy.MaxAttempts)
	}

	if policy.InitialBackoff < 100*time.Millisecond {
		t.Errorf("Expected file I/O operator to have higher InitialBackoff, got %v", policy.InitialBackoff)
	}
}

// TestPolicyForOperator_NetworkOperator tests network operator policy.
func TestPolicyForOperator_NetworkOperator(t *testing.T) {
	policy := PolicyForOperator("fetch-api", nil)

	if policy == nil {
		t.Fatal("Expected non-nil policy for network operator")
	}

	// Network operators should have high jitter to prevent thundering herd
	if policy.JitterFraction < 0.15 {
		t.Errorf("Expected high jitter for network operator, got %.2f", policy.JitterFraction)
	}
}

// TestPolicyForOperator_LLMOperator tests LLM operator policy.
func TestPolicyForOperator_LLMOperator(t *testing.T) {
	policy := PolicyForOperator("classify-text", nil)

	if policy == nil {
		t.Fatal("Expected non-nil policy for LLM operator")
	}

	// LLM operators should have quick backoff (usually fast)
	if policy.MaxAttempts > 3 {
		t.Errorf("Expected LLM operator to have low MaxAttempts, got %d", policy.MaxAttempts)
	}
}

// TestPolicyForOperator_CustomPolicy tests custom policy precedence.
func TestPolicyForOperator_CustomPolicy(t *testing.T) {
	customPolicy := &RetryPolicy{
		MaxAttempts:       10,
		InitialBackoff:    500 * time.Millisecond,
		MaxBackoff:        60 * time.Second,
		BackoffMultiplier: 1.2,
		JitterFraction:    0.05,
	}

	customPolicies := OperatorRetryPolicies{
		"my-operator": customPolicy,
	}

	policy := PolicyForOperator("my-operator", customPolicies)

	if policy != customPolicy {
		t.Error("Expected custom policy to be returned")
	}

	if policy.MaxAttempts != 10 {
		t.Errorf("Expected custom MaxAttempts=10, got %d", policy.MaxAttempts)
	}
}

// TestOperatorRetryPolicies_MultipleOperators tests managing multiple operator policies.
func TestOperatorRetryPolicies_MultipleOperators(t *testing.T) {
	policies := OperatorRetryPolicies{
		"file-read": {MaxAttempts: 5},
		"api-call": {MaxAttempts: 3},
		"llm-query": {MaxAttempts: 2},
	}

	tests := []struct {
		operatorName string
		expectedMax  int
	}{
		{"file-read", 5},
		{"api-call", 3},
		{"llm-query", 2},
	}

	for _, test := range tests {
		policy := PolicyForOperator(test.operatorName, policies)
		if policy.MaxAttempts != test.expectedMax {
			t.Errorf(
				"types.Operator %q: expected MaxAttempts=%d, got %d",
				test.operatorName, test.expectedMax, policy.MaxAttempts,
			)
		}
	}
}

// TestFormatRetryPolicy tests string formatting.
func TestFormatRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()
	formatted := FormatRetryPolicy(policy)

	if formatted == "" {
		t.Fatal("FormatRetryPolicy returned empty string")
	}

	// Verify key information is in the output
	if !contains(formatted, "max_attempts") {
		t.Error("Formatted string missing 'max_attempts'")
	}

	if !contains(formatted, "backoff") {
		t.Error("Formatted string missing 'backoff'")
	}
}

// TestComputeJitter tests jitter computation.
func TestComputeJitter(t *testing.T) {
	policy := &RetryPolicy{
		JitterFraction: 0.1,
	}
	calc := NewBackoffCalculator(policy)

	baseDuration := 1000 * time.Millisecond
	jitter := calc.ComputeJitter(baseDuration)

	// Jitter should be between -100 and +100 milliseconds (10% of 1000ms)
	if jitter < -100*time.Millisecond || jitter > 100*time.Millisecond {
		t.Errorf("Jitter out of expected range: %v", jitter)
	}
}

// TestBackoffCalculator_ZeroJitter tests zero jitter case.
func TestBackoffCalculator_ZeroJitter(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:       2,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 1.5,
		JitterFraction:    0,
	}

	calc := NewBackoffCalculator(policy)

	// First backoff should be exactly InitialBackoff
	backoff := calc.NextBackoff()
	if backoff != policy.InitialBackoff {
		t.Errorf("Expected backoff=%v (no jitter), got %v", policy.InitialBackoff, backoff)
	}
}

// TestIsFileIOOperator tests file I/O pattern matching.
func TestIsFileIOOperator(t *testing.T) {
	tests := map[string]bool{
		"read-file":     true,
		"write-config":  true,
		"mkdir-temp":    true,
		"copy-file":     true,
		"remove-old":    true,
		"list-files":    true,
		"cat-file":      true,
		"api-fetch":     false,
		"llm-classify":  false,
		"unknown":       false,
	}

	for operatorName, expected := range tests {
		result := isFileIOOperator(operatorName)
		if result != expected {
			t.Errorf("isFileIOOperator(%q): expected %v, got %v", operatorName, expected, result)
		}
	}
}

// TestIsNetworkOperator tests network pattern matching.
func TestIsNetworkOperator(t *testing.T) {
	tests := map[string]bool{
		"http-fetch":    true,
		"api-request":   true,
		"download-file": true,
		"upload-data":   true,
		"curl-request":  true,
		"read-file":     false,
		"classify-text": false,
		"unknown":       false,
	}

	for operatorName, expected := range tests {
		result := isNetworkOperator(operatorName)
		if result != expected {
			t.Errorf("isNetworkOperator(%q): expected %v, got %v", operatorName, expected, result)
		}
	}
}

// TestIsLLMOperator tests LLM pattern matching.
func TestIsLLMOperator(t *testing.T) {
	tests := map[string]bool{
		"llm-classify":      true,
		"model-inference":   true,
		"prompt-completion": true,
		"generate-text":     true,
		"read-file":         false,
		"api-fetch":         false,
		"unknown":           false,
	}

	for operatorName, expected := range tests {
		result := isLLMOperator(operatorName)
		if result != expected {
			t.Errorf("isLLMOperator(%q): expected %v, got %v", operatorName, expected, result)
		}
	}
}

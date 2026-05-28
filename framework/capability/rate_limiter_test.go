package capability

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestRateLimiterAllowsWithinBurst(t *testing.T) {
	inner := &mockInvoker2{
		results: map[string]*contracts.ToolResult{
			"test": {Success: true},
		},
	}
	limits := map[string]contracts.ToolRateLimit{
		"test": {PerSecond: 100, Burst: 5},
	}
	rl := NewRateLimitedInvoker(inner, limits)

	for i := 0; i < 5; i++ {
		_, err := rl.InvokeCapability(context.Background(), nil, "test", nil)
		if err != nil {
			t.Fatalf("call %d within burst should not error: %v", i, err)
		}
	}
}

func TestRateLimiterPerToolIsolation(t *testing.T) {
	inner := &mockInvoker2{
		results: map[string]*contracts.ToolResult{
			"a": {Success: true},
			"b": {Success: true},
		},
	}
	limits := map[string]contracts.ToolRateLimit{
		"a": {PerSecond: 1, Burst: 1},
		"b": {PerSecond: 1000, Burst: 100},
	}
	rl := NewRateLimitedInvoker(inner, limits)

	// Exhaust a's burst
	_, err := rl.InvokeCapability(context.Background(), nil, "a", nil)
	if err != nil {
		t.Fatalf("first call to a: %v", err)
	}

	// b should work independently
	_, err = rl.InvokeCapability(context.Background(), nil, "b", nil)
	if err != nil {
		t.Fatalf("b should not be affected by a: %v", err)
	}
}

func TestRateLimiterAllowsUnlimitedTools(t *testing.T) {
	inner := &mockInvoker2{
		results: map[string]*contracts.ToolResult{
			"unlimited": {Success: true},
		},
	}
	rl := NewRateLimitedInvoker(inner, nil) // no limits

	for i := 0; i < 100; i++ {
		_, err := rl.InvokeCapability(context.Background(), nil, "unlimited", nil)
		if err != nil {
			t.Fatalf("unlimited tool call %d: %v", i, err)
		}
	}
}

func TestRateLimiterContextCancelledWhileWaiting(t *testing.T) {
	inner := &mockInvoker2{
		results: map[string]*contracts.ToolResult{
			"slow": {Success: true},
		},
	}
	limits := map[string]contracts.ToolRateLimit{
		"slow": {PerSecond: 0.001, Burst: 1}, // very slow refill
	}
	rl := NewRateLimitedInvoker(inner, limits)

	// Exhaust burst
	_, _ = rl.InvokeCapability(context.Background(), nil, "slow", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := rl.InvokeCapability(ctx, nil, "slow", nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestRateLimiterBucketAllow(t *testing.T) {
	b := newToolRateBucket(100, 10)
	for i := 0; i < 10; i++ {
		if !b.allow() {
			t.Fatalf("call %d within burst should be allowed", i)
		}
	}
	// Burst exhausted
	if b.allow() {
		t.Fatal("expected rate limited after burst exhaustion")
	}
}

func TestRateLimiterBucketDefaultValues(t *testing.T) {
	b := newToolRateBucket(0, 0)
	if b.perSecond != 1 {
		t.Fatalf("expected default perSecond=1, got %f", b.perSecond)
	}
	if b.burst != 1 {
		t.Fatalf("expected default burst=1, got %d", b.burst)
	}
}

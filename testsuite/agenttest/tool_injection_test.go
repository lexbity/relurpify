package agenttest

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// Test that ToolResponseOverride struct fields are properly set
type testOverride struct {
	Tool        string
	Error       string
	FailureRate float64
	LatencyMs   int
	CallCount   int
}

func TestInjectionInterceptorMatchesOverride(t *testing.T) {
	// Create a mock tool
	baseTool := &mockTool{name: "test_tool"}

	// Create override with specific args matching
	override := ToolResponseOverride{
		Tool:      "test_tool",
		MatchArgs: map[string]interface{}{"arg1": "value1"},
		Error:     "injected error",
	}

	interceptor := NewInjectionInterceptor(baseTool, []ToolResponseOverride{override})

	// Test that args matching works
	matches := interceptor.matchesOverride(override, map[string]interface{}{"arg1": "value1"}, 1)
	if !matches {
		t.Error("Expected override to match when args match")
	}

	// Test that non-matching args don't match
	matches = interceptor.matchesOverride(override, map[string]interface{}{"arg1": "different"}, 1)
	if matches {
		t.Error("Expected override not to match when args differ")
	}
}

func TestInjectionInterceptorCallCount(t *testing.T) {
	baseTool := &mockTool{name: "test_tool"}

	// Override applies only to 2nd call
	override := ToolResponseOverride{
		Tool:      "test_tool",
		CallCount: 2,
		Error:     "second call fails",
	}

	interceptor := NewInjectionInterceptor(baseTool, []ToolResponseOverride{override})

	// First call should not match (call count = 1)
	matches := interceptor.matchesOverride(override, map[string]interface{}{}, 1)
	if matches {
		t.Error("Expected override not to match on first call when CallCount=2")
	}

	// Second call should match (call count = 2)
	matches = interceptor.matchesOverride(override, map[string]interface{}{}, 2)
	if !matches {
		t.Error("Expected override to match on second call when CallCount=2")
	}
}

func TestFilterOverridesForTool(t *testing.T) {
	overrides := []ToolResponseOverride{
		{Tool: "tool1"},
		{Tool: "tool2"},
		{Tool: "TOOL1"}, // case insensitive
		{Tool: "tool3"},
	}

	filtered := filterOverridesForTool(overrides, "tool1")
	if len(filtered) != 2 {
		t.Errorf("Expected 2 overrides for tool1 (case insensitive), got %d", len(filtered))
	}
}

func TestToolSuccessRate(t *testing.T) {
	events := []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": false}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "file_read", "success": true}},
	}

	successes, failures, rate := ToolSuccessRate(events, "go_test")
	if successes != 2 {
		t.Errorf("Expected 2 successes, got %d", successes)
	}
	if failures != 1 {
		t.Errorf("Expected 1 failure, got %d", failures)
	}
	if rate != 2.0/3.0 {
		t.Errorf("Expected rate 0.667, got %f", rate)
	}

	// Check file_read
	successes, failures, rate = ToolSuccessRate(events, "file_read")
	if successes != 1 || failures != 0 || rate != 1.0 {
		t.Errorf("Expected 1 success, 0 failures, rate 1.0 for file_read, got %d/%d/%f", successes, failures, rate)
	}

	// Check non-existent tool
	successes, failures, rate = ToolSuccessRate(events, "nonexistent")
	if successes != 0 || failures != 0 || rate != 0.0 {
		t.Errorf("Expected 0/0/0 for nonexistent tool, got %d/%d/%f", successes, failures, rate)
	}
}

func TestHasRecoveryFromToolFailure(t *testing.T) {
	// No failures - no recovery
	events := []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
	}
	if HasRecoveryFromToolFailure(events) {
		t.Error("Expected no recovery when no failures")
	}

	// Has failure but no success after
	events = []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": false}},
	}
	if HasRecoveryFromToolFailure(events) {
		t.Error("Expected no recovery when failure is last")
	}

	// Has failure followed by success
	events = []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": false}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
	}
	if !HasRecoveryFromToolFailure(events) {
		t.Error("Expected recovery detected when success follows failure")
	}
}

func TestEvaluateSuccessRateConstraint(t *testing.T) {
	tests := []struct {
		rate       float64
		constraint string
		want       bool
	}{
		{0.9, ">0.8", true},
		{0.9, ">0.9", false},
		{0.9, ">=0.9", true},
		{0.9, ">=0.8", true},
		{0.5, "<0.6", true},
		{0.5, "<=0.5", true},
		{0.5, "0.5", true},  // bare number = >=
		{0.9, "0.9", true},  // bare number = >=
		{0.8, "0.9", false}, // bare number = >=
		{0.9, "", true},     // empty constraint
		{0.9, ">=1.0", false},
		{0.0, "<0.1", true},
	}

	for _, tt := range tests {
		got := evaluateSuccessRateConstraint(tt.rate, tt.constraint)
		if got != tt.want {
			t.Errorf("evaluateSuccessRateConstraint(%f, %q) = %v, want %v", tt.rate, tt.constraint, got, tt.want)
		}
	}
}

func TestInjectionInterceptorInterface(t *testing.T) {
	baseTool := &mockTool{name: "test_tool"}
	interceptor := NewInjectionInterceptor(baseTool, nil)

	// Test that the interceptor implements the Tool interface
	if interceptor.Name() != "test_tool" {
		t.Errorf("Expected name test_tool, got %s", interceptor.Name())
	}
	if interceptor.Description() != "Mock tool test_tool" {
		t.Errorf("Unexpected description: %s", interceptor.Description())
	}
	if interceptor.Category() != "test" {
		t.Errorf("Expected category test, got %s", interceptor.Category())
	}
	if !interceptor.IsAvailable(context.Background(), nil) {
		t.Error("Expected IsAvailable to return true")
	}
}

func TestInjectionInterceptorCallHistory(t *testing.T) {
	baseTool := &mockTool{name: "test_tool"}
	interceptor := NewInjectionInterceptor(baseTool, nil)

	if interceptor.GetCallCount() != 0 {
		t.Errorf("Expected initial call count 0, got %d", interceptor.GetCallCount())
	}

	// Execute multiple times
	ctx := context.Background()
	state := core.NewContext()
	args := map[string]interface{}{}

	for i := 0; i < 3; i++ {
		interceptor.Execute(ctx, state, args)
	}

	if interceptor.GetCallCount() != 3 {
		t.Errorf("Expected call count 3, got %d", interceptor.GetCallCount())
	}
}

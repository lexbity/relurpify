package reporting

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// A route that completed with zero errors but carries the react loop's
// unverified marker (edit-shaped task closed without an executed write or
// test observation) must downgrade to unverified, never success: the model
// asserting success is not evidence.
func TestTelemetryNodeUnverifiedDowngrade(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetExecutionCompleted(env, true)
	env.SetWorkingValueWithClass("euclo.outcome.unverified", true, "task")
	env.SetWorkingValueWithClass("euclo.outcome.unverified_reason",
		"task edits code but the run executed no file write or edit observation", "task")

	result, err := node.Execute(t.Context(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result["outcome_category"] != string(OutcomeCategoryUnverified) {
		t.Errorf("Expected outcome_category unverified, got %v", result["outcome_category"])
	}
	if got, ok := result["outcome_reason"].(string); !ok || got != "task edits code but the run executed no file write or edit observation" {
		t.Errorf("Expected the unverified reason to surface, got %v", result["outcome_reason"])
	}
	if got, _ := state.GetOutcomeCategory(env); got != string(OutcomeCategoryUnverified) {
		t.Errorf("Expected envelope outcome category unverified, got %v", got)
	}
}

// A genuinely evidenced success must not be downgraded by a stale flag.
func TestTelemetryNodeVerifiedSuccessStaysSuccess(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetExecutionCompleted(env, true)

	result, err := node.Execute(t.Context(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result["outcome_category"] != string(OutcomeCategorySuccess) {
		t.Errorf("Expected outcome_category success, got %v", result["outcome_category"])
	}
}

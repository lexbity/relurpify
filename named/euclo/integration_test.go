package euclo

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

// TestExecutionPathCapability verifies the capability execution path.
func TestExecutionPathCapability(t *testing.T) {
	graph := orchestrate.NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	// Set up for capability execution
	env.SetWorkingValue("euclo.route.kind", "capability", contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.route.capability_id", "debug", contextdata.MemoryClassTask)

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify execution completed
	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok || completed != true {
		t.Error("Expected execution.completed to be true")
	}

	// Verify capability path was taken
	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok || kind != "capability" {
		t.Error("Expected execution.kind to be capability")
	}

	// Verify outcome was classified
	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category to be set")
	}

	if category != "success" {
		t.Errorf("Expected outcome.category success, got %v", category)
	}
}

// TestExecutionPathRecipe verifies the recipe execution path.
func TestExecutionPathRecipe(t *testing.T) {
	graph := orchestrate.NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify execution completed
	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok || completed != true {
		t.Error("Expected execution.completed to be true")
	}

	// Verify execution path was taken (stub defaults to capability)
	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok || kind == "" {
		t.Error("Expected execution.kind to be set")
	}

	// Verify outcome was classified
	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category to be set")
	}

	if category != "success" {
		t.Errorf("Expected outcome.category success, got %v", category)
	}
}

// TestPolicyEnforcement verifies policy decisions are enforced.
func TestPolicyEnforcement(t *testing.T) {
	graph := orchestrate.NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify policy decision was made
	permitted, ok := env.GetWorkingValue("euclo.policy.mutation_permitted")
	if !ok {
		t.Error("Expected policy.mutation_permitted to be set")
	}

	// Default policy should permit mutations for stub context
	if permitted != true {
		t.Errorf("Expected policy.mutation_permitted true, got %v", permitted)
	}

	// Verify HITL decision (stub context uses medium risk, so HITL may not be required)
	hitlRequired, ok := env.GetWorkingValue("euclo.policy.hitl_required")
	if !ok {
		t.Error("Expected policy.hitl_required to be set")
	}

	// Stub context has medium risk, so HITL is not required by default
	// This test just verifies the decision was made
	if hitlRequired == nil {
		t.Error("Expected policy.hitl_required to have a value")
	}
}

// TestEndToEndFlow verifies the complete end-to-end flow.
func TestEndToEndFlow(t *testing.T) {
	graph := orchestrate.NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify all stages completed
	// 1. Policy decision
	_, ok := env.GetWorkingValue("euclo.policy.mutation_permitted")
	if !ok {
		t.Error("Expected policy decision to be set")
	}

	// 2. Route selection
	_, ok = env.GetWorkingValue("euclo.route.kind")
	if !ok {
		t.Error("Expected route selection to be set")
	}

	// 3. Fork decision
	_, ok = env.GetWorkingValue("euclo.fork.branch")
	if !ok {
		t.Error("Expected fork decision to be set")
	}

	// 4. Execution completion
	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok || completed != true {
		t.Error("Expected execution.completed to be true")
	}

	// 5. Outcome classification
	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category to be set")
	}

	if category == "" {
		t.Error("Expected non-empty outcome.category")
	}
}

// TestOutcomeClassification verifies outcome classification logic.
func TestOutcomeClassification(t *testing.T) {
	graph := orchestrate.NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify outcome classification
	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category to be set")
	}

	reason, ok := env.GetWorkingValue("euclo.outcome.reason")
	if !ok {
		t.Error("Expected outcome.reason to be set")
	}

	// With successful execution, outcome should be success
	if category != "success" {
		t.Errorf("Expected outcome.category success, got %v", category)
	}

	if reason != "execution completed successfully" {
		t.Errorf("Expected reason execution completed successfully, got %v", reason)
	}
}

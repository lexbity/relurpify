package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestRootGraphExecute(t *testing.T) {
	graph := NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify that execution completed
	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok {
		t.Error("Expected execution.completed in envelope")
	}

	if completed != true {
		t.Errorf("Expected execution.completed true, got %v", completed)
	}

	// Verify outcome was classified
	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category in envelope")
	}

	if category == "" {
		t.Error("Expected non-empty outcome.category")
	}
}

func TestRootGraphRecipeRoute(t *testing.T) {
	graph := NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify execution path was taken (stub defaults to capability)
	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind == "" {
		t.Error("Expected non-empty execution.kind")
	}
}

func TestRootGraphCapabilityRoute(t *testing.T) {
	graph := NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route.kind", "capability", contextdata.MemoryClassTask)

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify capability execution path was taken
	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "capability" {
		t.Errorf("Expected execution.kind capability, got %v", kind)
	}
}

func TestRootGraphPolicyDecision(t *testing.T) {
	graph := NewRootGraph()

	env := contextdata.NewEnvelope("task-123", "session-456")

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify policy decision was made
	permitted, ok := env.GetWorkingValue("euclo.policy.mutation_permitted")
	if !ok {
		t.Error("Expected policy.mutation_permitted in envelope")
	}

	if permitted == nil {
		t.Error("Expected non-nil policy.mutation_permitted")
	}
}

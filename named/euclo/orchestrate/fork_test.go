package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestForkNodeRecipeBranch(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route.kind", "recipe", contextdata.MemoryClassTask)

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["branch"] != "recipe_execution" {
		t.Errorf("Expected branch recipe_execution, got %v", result["branch"])
	}

	branch, ok := env.GetWorkingValue("euclo.fork.branch")
	if !ok {
		t.Error("Expected fork.branch in envelope")
	}

	if branch != "recipe_execution" {
		t.Errorf("Expected fork.branch recipe_execution, got %v", branch)
	}
}

func TestForkNodeCapabilityBranch(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route.kind", "capability", contextdata.MemoryClassTask)

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["branch"] != "capability_execution" {
		t.Errorf("Expected branch capability_execution, got %v", result["branch"])
	}

	branch, ok := env.GetWorkingValue("euclo.fork.branch")
	if !ok {
		t.Error("Expected fork.branch in envelope")
	}

	if branch != "capability_execution" {
		t.Errorf("Expected fork.branch capability_execution, got %v", branch)
	}
}

func TestForkNodeDefaultBranch(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["branch"] != "capability_execution" {
		t.Errorf("Expected branch capability_execution (default), got %v", result["branch"])
	}
}

func TestForkNodeID(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	if fork.ID() != "fork1" {
		t.Errorf("Expected ID fork1, got %s", fork.ID())
	}
}

func TestForkNodeType(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	if fork.Type() != "route_fork" {
		t.Errorf("Expected Type route_fork, got %s", fork.Type())
	}
}

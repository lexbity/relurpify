package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestForkNodeThoughtRecipeBranch(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.dispatch.route_kind", "thoughtrecipe", contextdata.MemoryClassTask)

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Data["branch"] != "thoughtrecipe_execution" {
		t.Errorf("Expected branch thoughtrecipe_execution, got %v", result.Data["branch"])
	}

	branch, ok := env.GetWorkingValue("euclo.fork.branch")
	if !ok {
		t.Error("Expected fork.branch in envelope")
	}

	if branch != "thoughtrecipe_execution" {
		t.Errorf("Expected fork.branch thoughtrecipe_execution, got %v", branch)
	}
	if got, ok := result.Data["next"].(string); !ok || got != "euclo.execute_thoughtrecipe" {
		t.Fatalf("expected next euclo.execute_thoughtrecipe, got %v (ok=%v)", got, ok)
	}
}

func TestForkNodeCapabilityBranch(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.dispatch.route_kind", "capability", contextdata.MemoryClassTask)

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Data["branch"] != "capability_execution" {
		t.Errorf("Expected branch capability_execution, got %v", result.Data["branch"])
	}

	branch, ok := env.GetWorkingValue("euclo.fork.branch")
	if !ok {
		t.Error("Expected fork.branch in envelope")
	}

	if branch != "capability_execution" {
		t.Errorf("Expected fork.branch capability_execution, got %v", branch)
	}
	if got, ok := result.Data["next"].(string); !ok || got != "euclo.execute_capability" {
		t.Fatalf("expected next euclo.execute_capability, got %v (ok=%v)", got, ok)
	}
}

func TestForkNodeIntentBranchUsesThoughtRecipeExecution(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.dispatch.route_kind", RouteKindIntent, contextdata.MemoryClassTask)

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Data["branch"] != "thoughtrecipe_execution" {
		t.Errorf("Expected branch thoughtrecipe_execution for intent route, got %v", result.Data["branch"])
	}
	if got, ok := result.Data["next"].(string); !ok || got != "euclo.execute_thoughtrecipe" {
		t.Fatalf("expected next euclo.execute_thoughtrecipe, got %v (ok=%v)", got, ok)
	}
}

func TestForkNodeMissingRouteKind(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")

	if _, err := fork.Execute(context.Background(), env); err == nil {
		t.Fatal("expected error when route kind is missing")
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

	if fork.Type() != agentgraph.NodeTypeConditional {
		t.Errorf("Expected Type conditional, got %s", fork.Type())
	}
}

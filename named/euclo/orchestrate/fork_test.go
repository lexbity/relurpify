package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func TestForkNodeThoughtRecipeBranch(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       euclotypes.RouteKindThoughtRecipe,
		ThoughtRecipeID: "thoughtrecipe.intent.review",
	})

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got, ok := execution.ResultField(result.Data, "branch"); !ok || got != "thoughtrecipe_execution" {
		t.Errorf("Expected branch thoughtrecipe_execution, got %v", got)
	}

	if branch := state.GetForkBranch(env); branch != "thoughtrecipe_execution" {
		t.Errorf("Expected fork.branch thoughtrecipe_execution, got %v", branch)
	}
	if got, ok := execution.ResultField(result.Data, "next"); !ok || got != "euclo.execute_thoughtrecipe" {
		t.Fatalf("expected next euclo.execute_thoughtrecipe, got %v (ok=%v)", got, ok)
	}
}

func TestForkNodeCapabilityBranch(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    euclotypes.RouteKindCapability,
		CapabilityID: "euclo:cap.review",
	})

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got, ok := execution.ResultField(result.Data, "branch"); !ok || got != "capability_execution" {
		t.Errorf("Expected branch capability_execution, got %v", got)
	}

	if branch := state.GetForkBranch(env); branch != "capability_execution" {
		t.Errorf("Expected fork.branch capability_execution, got %v", branch)
	}
	if got, ok := execution.ResultField(result.Data, "next"); !ok || got != "euclo.execute_capability" {
		t.Fatalf("expected next euclo.execute_capability, got %v (ok=%v)", got, ok)
	}
}

func TestForkNodeIntentBranchUsesThoughtRecipeExecution(t *testing.T) {
	fork := NewRouteForkNode("fork1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:       euclotypes.RouteKindIntent,
		ThoughtRecipeID: clarificationThoughtRecipeID,
	})

	result, err := fork.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got, ok := execution.ResultField(result.Data, "branch"); !ok || got != "thoughtrecipe_execution" {
		t.Errorf("Expected branch thoughtrecipe_execution for intent route, got %v", got)
	}
	if got, ok := execution.ResultField(result.Data, "next"); !ok || got != "euclo.execute_thoughtrecipe" {
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

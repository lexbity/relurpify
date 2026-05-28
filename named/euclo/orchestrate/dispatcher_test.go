package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestDispatcherExecute(t *testing.T) {
	dispatcher := NewDispatcher("dispatcher1")

	env := contextdata.NewEnvelope("task-123", "session-456")

	result, err := dispatcher.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	routeSelection, ok := state.GetRouteSelection(env)
	if !ok || routeSelection == nil {
		t.Fatalf("Expected *euclotypes.RouteSelection, got %#v", routeSelection)
	}
	if routeSelection.RouteKind != euclotypes.RouteKindIntent {
		t.Errorf("Expected route selection intent, got %v", routeSelection.RouteKind)
	}
	if routeSelection.ThoughtRecipeID != clarificationThoughtRecipeID {
		t.Errorf("Expected clarification thoughtrecipe, got %v", routeSelection.ThoughtRecipeID)
	}
}

func TestDispatcherID(t *testing.T) {
	dispatcher := NewDispatcher("dispatcher1")

	if dispatcher.ID() != "dispatcher1" {
		t.Errorf("Expected ID dispatcher1, got %s", dispatcher.ID())
	}
}

func TestDispatcherType(t *testing.T) {
	dispatcher := NewDispatcher("dispatcher1")

	if dispatcher.Type() != agentgraph.NodeTypeSystem {
		t.Errorf("Expected Type system, got %s", dispatcher.Type())
	}
}

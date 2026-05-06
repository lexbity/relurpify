package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
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

	selection, ok := env.GetWorkingValue("euclo.route_selection")
	if !ok {
		t.Fatal("Expected route_selection in envelope")
	}
	routeSelection, ok := selection.(*RouteSelection)
	if !ok || routeSelection == nil {
		t.Fatalf("Expected *RouteSelection, got %T", selection)
	}
	if routeSelection.RouteKind != "capability" {
		t.Errorf("Expected route selection capability, got %v", routeSelection.RouteKind)
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

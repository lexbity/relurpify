package orchestrate

import (
	"context"
	"testing"

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

	// Check that route selection was written to envelope
	routeKind, ok := env.GetWorkingValue("euclo.route.kind")
	if !ok {
		t.Error("Expected route.kind in envelope")
	}

	if routeKind != "capability" {
		t.Errorf("Expected route.kind capability, got %v", routeKind)
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

	if dispatcher.Type() != "dispatcher" {
		t.Errorf("Expected Type dispatcher, got %s", dispatcher.Type())
	}
}

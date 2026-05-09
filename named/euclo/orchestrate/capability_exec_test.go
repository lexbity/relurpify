package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestCapabilityExecutionNodeExecute(t *testing.T) {
	node := NewCapabilityExecutionNode("capability-exec1")
	node.registry = nil

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:    "capability",
		CapabilityID: "debug",
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}
	if err == nil {
		t.Fatal("Expected error")
	}
	if result.Success {
		t.Fatalf("Expected failure, got success result: %+v", result)
	}
	if got := result.Data["error"]; got != "capability registry unavailable" {
		t.Fatalf("Expected registry error, got %v", got)
	}
}

func TestCapabilityExecutionNodeID(t *testing.T) {
	node := NewCapabilityExecutionNode("capability-exec1")

	if node.ID() != "capability-exec1" {
		t.Errorf("Expected ID capability-exec1, got %s", node.ID())
	}
}

func TestCapabilityExecutionNodeType(t *testing.T) {
	node := NewCapabilityExecutionNode("capability-exec1")

	if node.Type() != agentgraph.NodeTypeSystem {
		t.Errorf("Expected Type system, got %s", node.Type())
	}
}

func TestCapabilityExecutionNodeWritesToEnvelope(t *testing.T) {
	node := NewCapabilityExecutionNode("capability-exec1")
	node.registry = nil

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:    "capability",
		CapabilityID: "debug",
	}, contextdata.MemoryClassTask)

	_, err := node.Execute(context.Background(), env)
	if err == nil {
		t.Fatal("Expected error")
	}

	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "capability" {
		t.Errorf("Expected execution.kind capability, got %v", kind)
	}

	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok {
		t.Error("Expected execution.completed in envelope")
	}

	if completed != false {
		t.Errorf("Expected execution.completed false, got %v", completed)
	}

}

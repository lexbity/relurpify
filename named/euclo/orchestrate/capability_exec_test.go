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

func TestCapabilityExecutionNodeExecute(t *testing.T) {
	node := NewCapabilityExecutionNode("capability-exec1")
	node.registry = nil

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    "capability",
		CapabilityID: "debug",
	})

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
	if got, ok := execution.ResultField(result.Data, "error"); !ok || got != "capability registry unavailable" {
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
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    "capability",
		CapabilityID: "debug",
	})

	_, err := node.Execute(context.Background(), env)
	if err == nil {
		t.Fatal("Expected error")
	}

	kind := state.GetExecutionKind(env)
	if kind == "" {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "capability" {
		t.Errorf("Expected execution.kind capability, got %v", kind)
	}

	completed, ok := contextdata.GetTyped[bool](env, state.KeyExecutionCompleted)
	if !ok {
		t.Error("Expected execution.completed in envelope")
	}
	if completed != false {
		t.Errorf("Expected execution.completed false, got %v", completed)
	}

}

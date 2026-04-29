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
	env.SetWorkingValue("euclo.route.capability_id", "debug", contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result.Data["capability_id"] != "debug" {
		t.Errorf("Expected capability_id debug, got %v", result.Data["capability_id"])
	}

	if result.Data["stub"] != true {
		t.Errorf("Expected stub execution result, got %v", result.Data["stub"])
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
	env.SetWorkingValue("euclo.route.capability_id", "debug", contextdata.MemoryClassTask)

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
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

	if completed != true {
		t.Errorf("Expected execution.completed true, got %v", completed)
	}

}

package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestCapabilityExecutionNodeExecute(t *testing.T) {
	node := NewCapabilityExecutionNode("capability-exec1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route.capability_id", "debug", contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result["execution_kind"] != "capability" {
		t.Errorf("Expected execution_kind capability, got %v", result["execution_kind"])
	}

	if result["capability_id"] != "debug" {
		t.Errorf("Expected capability_id debug, got %v", result["capability_id"])
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

	if node.Type() != "capability_execution" {
		t.Errorf("Expected Type capability_execution, got %s", node.Type())
	}
}

func TestCapabilityExecutionNodeWritesToEnvelope(t *testing.T) {
	node := NewCapabilityExecutionNode("capability-exec1")

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

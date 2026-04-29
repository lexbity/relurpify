package reporting

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestTelemetryNodeExecute(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result["outcome_category"] != "success" {
		t.Errorf("Expected outcome_category success, got %v", result["outcome_category"])
	}
}

func TestTelemetryNodeID(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	if node.ID() != "telemetry1" {
		t.Errorf("Expected ID telemetry1, got %s", node.ID())
	}
}

func TestTelemetryNodeType(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	if node.Type() != "telemetry" {
		t.Errorf("Expected Type telemetry, got %s", node.Type())
	}
}

func TestTelemetryNodeWritesToEnvelope(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category in envelope")
	}

	if category != "success" {
		t.Errorf("Expected outcome.category success, got %v", category)
	}

	reason, ok := env.GetWorkingValue("euclo.outcome.reason")
	if !ok {
		t.Error("Expected outcome.reason in envelope")
	}

	if reason != "execution completed successfully" {
		t.Errorf("Expected outcome.reason execution completed successfully, got %v", reason)
	}
}

func TestTelemetryNodeIncompleteExecution(t *testing.T) {
	node := NewTelemetryNode("telemetry1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.execution.completed", false, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["outcome_category"] != "cancelled" {
		t.Errorf("Expected outcome_category cancelled, got %v", result["outcome_category"])
	}
}

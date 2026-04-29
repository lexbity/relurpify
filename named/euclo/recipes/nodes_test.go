package recipe

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestLLMNodeExecute(t *testing.T) {
	node := NewLLMNode("llm1", "LLM step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["llm_output"] == nil {
		t.Error("Expected llm_output in result")
	}
}

func TestRetrieveNodeExecute(t *testing.T) {
	node := NewRetrieveNode("retrieve1", "Retrieve step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["retrieved_docs"] == nil {
		t.Error("Expected retrieved_docs in result")
	}
}

func TestIngestNodeExecute(t *testing.T) {
	node := NewIngestNode("ingest1", "Ingest step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["ingested_files"] == nil {
		t.Error("Expected ingested_files in result")
	}
}

func TestTransformNodeExecute(t *testing.T) {
	node := NewTransformNode("transform1", "Transform step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["transformed_data"] == nil {
		t.Error("Expected transformed_data in result")
	}
}

func TestEmitNodeExecute(t *testing.T) {
	node := NewEmitNode("emit1", "Emit step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["emitted"] == nil {
		t.Error("Expected emitted in result")
	}
}

func TestGateNodeExecute(t *testing.T) {
	node := NewGateNode("gate1", "Gate step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["gate_passed"] == nil {
		t.Error("Expected gate_passed in result")
	}
}

func TestBranchNodeExecute(t *testing.T) {
	node := NewBranchNode("branch1", "Branch step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["branch_taken"] == nil {
		t.Error("Expected branch_taken in result")
	}
}

func TestParallelNodeExecute(t *testing.T) {
	node := NewParallelNode("parallel1", "Parallel step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["parallel_results"] == nil {
		t.Error("Expected parallel_results in result")
	}
}

func TestCaptureNodeExecute(t *testing.T) {
	captures := map[string]string{
		"output": "euclo.recipe.test-recipe.output",
	}
	node := NewCaptureNode("capture1", "Capture step", nil, captures, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["captured"] == nil {
		t.Error("Expected captured in result")
	}

	// Check that capture was written to envelope
	val, ok := env.GetWorkingValue("euclo.recipe.test-recipe.output")
	if !ok {
		t.Error("Expected capture value in envelope")
	}

	if val != "captured_value_output" {
		t.Errorf("Expected captured_value_output, got %v", val)
	}
}

func TestVerifyNodeExecute(t *testing.T) {
	node := NewVerifyNode("verify1", "Verify step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["verified"] == nil {
		t.Error("Expected verified in result")
	}
}

func TestPolicyCheckNodeExecute(t *testing.T) {
	node := NewPolicyCheckNode("policy1", "Policy check step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["policy_passed"] == nil {
		t.Error("Expected policy_passed in result")
	}
}

func TestTelemetryNodeExecute(t *testing.T) {
	node := NewTelemetryNode("telemetry1", "Telemetry step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["telemetry_emitted"] == nil {
		t.Error("Expected telemetry_emitted in result")
	}
}

func TestCustomNodeExecute(t *testing.T) {
	node := NewCustomNode("custom1", "Custom step", nil, nil, nil)

	env := contextdata.NewEnvelope("task-123", "session-456")
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["custom_executed"] == nil {
		t.Error("Expected custom_executed in result")
	}
}

func TestBaseNodeID(t *testing.T) {
	node := NewLLMNode("llm1", "LLM step", nil, nil, nil)

	if node.ID() != "llm1" {
		t.Errorf("Expected ID llm1, got %s", node.ID())
	}
}

func TestBaseNodeType(t *testing.T) {
	node := NewLLMNode("llm1", "LLM step", nil, nil, nil)

	if node.Type() != "llm" {
		t.Errorf("Expected Type llm, got %s", node.Type())
	}
}

func TestNodeConfig(t *testing.T) {
	config := map[string]interface{}{
		"model": "gpt-4",
	}
	node := NewLLMNode("llm1", "LLM step", config, nil, nil)

	if node.BaseNode.config["model"] != "gpt-4" {
		t.Errorf("Expected config model gpt-4, got %v", node.BaseNode.config["model"])
	}
}

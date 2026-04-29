package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestRecipeExecutionNodeExecute(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route.recipe_id", "fix-bug", contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result["execution_kind"] != "recipe" {
		t.Errorf("Expected execution_kind recipe, got %v", result["execution_kind"])
	}

	if result["recipe_id"] != "fix-bug" {
		t.Errorf("Expected recipe_id fix-bug, got %v", result["recipe_id"])
	}
}

func TestRecipeExecutionNodeID(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	if node.ID() != "recipe-exec1" {
		t.Errorf("Expected ID recipe-exec1, got %s", node.ID())
	}
}

func TestRecipeExecutionNodeType(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	if node.Type() != "recipe_executor" {
		t.Errorf("Expected Type recipe_executor, got %s", node.Type())
	}
}

func TestRecipeExecutionNodeWritesToEnvelope(t *testing.T) {
	node := NewRecipeExecutorNode("recipe-exec1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route.recipe_id", "fix-bug", contextdata.MemoryClassTask)

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "recipe" {
		t.Errorf("Expected execution.kind recipe, got %v", kind)
	}

	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok {
		t.Error("Expected execution.completed in envelope")
	}

	if completed != true {
		t.Errorf("Expected execution.completed true, got %v", completed)
	}
}

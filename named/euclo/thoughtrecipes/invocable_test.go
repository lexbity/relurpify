package thoughtrecipes

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestRecipeInvocableInvokeIncludesRecipeMetadata(t *testing.T) {
	plan := &ExecutionPlan{
		Name:     "demo-recipe",
		FilePath: "/tmp/demo-recipe.yaml",
		Steps: []ExecutionStep{
			{
				ID: "step-1",
				Parent: ExecutionStepAgent{
					Paradigm: "react",
					Prompt:   "Provide a concise summary of the task.",
				},
				Child: &ExecutionStepAgent{
					Paradigm: "planner",
					Prompt:   "This child should be ignored and still emit a warning.",
				},
			},
		},
	}

	invocable := &RecipeInvocable{
		Plan:     plan,
		Executor: NewExecutor(),
	}

	result, err := invocable.Invoke(context.Background(), execution.InvokeInput{
		Task: &core.Task{
			ID:          "task-1",
			Instruction: "Explain the current implementation.",
			Type:        core.TaskTypeAnalysis,
		},
		Environment: testutil.Env(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}
	if got := result.Data["recipe_id"]; got != plan.Name {
		t.Fatalf("expected recipe_id %q, got %#v", plan.Name, got)
	}
	if warnings, ok := result.Data["warnings"].([]string); !ok || len(warnings) == 0 {
		t.Fatalf("expected warnings in result, got %#v", result.Data["warnings"])
	}
}

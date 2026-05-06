package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

func TestDryRunEndToEndRecipePath(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.code_review", "euclo:cap.capture", "euclo:cap.consume")
	recipes := newRecipeRegistry(t, &recipepkg.ThoughtRecipe{
		ID:         "euclo.recipe.review",
		APIVersion: "euclo/v1",
		Metadata:   recipepkg.RecipeMetadata{Name: "review"},
		Sequence: recipepkg.RecipeSequence{
			Steps: []recipepkg.RecipeStep{
				{
					ID:           "step-1",
					CapabilityID: "euclo:cap.capture",
					Captures:     map[string]string{"output": "first_output"},
				},
				{
					ID:           "step-2",
					CapabilityID: "euclo:cap.consume",
					Captures:     map[string]string{"result": "second_output"},
				},
			},
		},
	})
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"review": {Sequence: []string{"euclo:cap.code_review"}, Operator: "OR"},
		},
	}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithRecipeRegistry(recipes),
		orchestrate.WithCapabilityClassifier(classifier),
	)

	env := contextdata.NewEnvelope("task-recipe", "session-recipe")
	seedTask(env, "review the auth package", "review.go")
	env.SetWorkingValue("euclo.recipe_id", "euclo.recipe.review", contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"review.go"})
	telemetry := &recordingTelemetry{}

	if err := graph.Execute(core.WithTelemetry(context.Background(), telemetry), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "recipe" {
		t.Fatalf("execution kind = %q, want recipe", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.recipe_id"); got != "euclo.recipe.review" {
		t.Fatalf("recipe id = %q, want euclo.recipe.review", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected recipe execution to complete")
	}
	if classifier.callCount() == 0 {
		t.Fatal("expected tier-2 classifier to be invoked")
	}
}

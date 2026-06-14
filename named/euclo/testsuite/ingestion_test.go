package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestEndToEndFileSelectionGrounding(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n\nfunc Review() {}\n")

	caps := newCapabilityRegistry(t)
	thoughtrecipes := newThoughtRecipeRegistry(t, &surface.ThoughtRecipe{
		ID:   "euclo.thoughtrecipe.review",
		Name: "review",
		Metadata: surface.ThoughtRecipeMetadata{
			Name: "review",
		},
	})
	deps := rootGraphDepsWithModel(caps, stubLanguageModel{})
	deps.ThoughtRecipes = thoughtrecipes
	graph, err := orchestrate.NewRootGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-file-grounding", "session-file-grounding")
	seedTask(env, "review the auth package", "review.go")
	contextdata.SetTyped(env, euclostate.KeyTaskInput, &execution.Task{
		ID:          "task-file-grounding",
		Type:        "euclo",
		Instruction: "review the auth package",
		Context: map[string]any{
			"euclo.user_files": []string{"review.go"},
		},
		Metadata: map[string]any{},
	})
	contextdata.SetTyped(env, euclostate.KeyTaskInput, &execution.Task{
		ID:          "task-file-grounding",
		Type:        "euclo",
		Instruction: "review the auth package",
		Context: map[string]any{
			"euclo.user_files": []string{"review.go"},
		},
		Metadata: map[string]any{},
	})
	runPreIngestion(t, env, dir, []string{"review.go"})

	if err := graph.Execute(context.Background(), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	evidenceValue, ok := euclostate.GetIntentEvidence(env)
	if !ok {
		t.Fatal("expected intent evidence in envelope")
	}
	if len(evidenceValue.UserFiles) == 0 || evidenceValue.UserFiles[0] != "review.go" {
		t.Fatalf("user files = %#v, want review.go", evidenceValue.UserFiles)
	}
	interpretationValue, ok := euclostate.GetIntentInterpretation(env)
	if !ok {
		t.Fatal("expected intent interpretation in envelope")
	}
	if interpretationValue.Target == "" {
		t.Fatal("expected grounded interpretation target")
	}
	selectionValue, ok := euclostate.GetRouteSelection(env)
	if !ok {
		t.Fatal("expected route_selection in envelope")
	}
	if selectionValue.RouteKind != euclotypes.RouteKindThoughtRecipe || selectionValue.ThoughtRecipeID != "euclo.thoughtrecipe.review" {
		t.Fatalf("unexpected route selection: %+v", selectionValue)
	}
	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %q, want thoughtrecipe", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.thoughtrecipe_id"); got != "euclo.thoughtrecipe.review" {
		t.Fatalf("execution thoughtrecipe id = %q, want euclo.thoughtrecipe.review", got)
	}
	if frameValue, ok := contextdata.GetTyped[any](env, euclostate.KeyClarificationFrame); ok && frameValue != nil {
		t.Fatalf("expected grounding to avoid clarification frame, got %#v", frameValue)
	}
}

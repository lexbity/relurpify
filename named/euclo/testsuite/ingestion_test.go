package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func TestEndToEndFileSelectionGrounding(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n\nfunc Review() {}\n")

	caps := newCapabilityRegistry(t)
	thoughtrecipes := newThoughtRecipeRegistry(t, &thoughtrecipepkg.ThoughtRecipe{
		ID:   "euclo.thoughtrecipe.review",
		Name: "review",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{
			Name: "review",
		},
	})
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
	)

	env := contextdata.NewEnvelope("task-file-grounding", "session-file-grounding")
	seedTask(env, "review the auth package", "review.go")
	env.SetWorkingValue("task.input", &core.Task{
		ID:          "task-file-grounding",
		Type:        "euclo",
		Instruction: "review the auth package",
		Context: map[string]any{
			"euclo.user_files": []string{"review.go"},
		},
		Metadata: map[string]any{},
	}, contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"review.go"})

	if err := graph.Execute(context.Background(), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	evidenceValue, ok := env.GetWorkingValue("euclo.intent_evidence")
	if !ok {
		t.Fatal("expected intent evidence in envelope")
	}
	evidence, ok := evidenceValue.(*intentcontext.IntentEvidence)
	if !ok || evidence == nil {
		t.Fatalf("expected *IntentEvidence, got %T", evidenceValue)
	}
	if len(evidence.UserFiles) == 0 || evidence.UserFiles[0] != "review.go" {
		t.Fatalf("user files = %#v, want review.go", evidence.UserFiles)
	}
	interpretationValue, ok := env.GetWorkingValue(intentcontext.IntentInterpretationKey)
	if !ok {
		t.Fatal("expected intent interpretation in envelope")
	}
	interpretation, ok := interpretationValue.(*intentcontext.IntentInterpretation)
	if !ok || interpretation == nil {
		t.Fatalf("expected *IntentInterpretation, got %T", interpretationValue)
	}
	if interpretation.Target == "" {
		t.Fatal("expected grounded interpretation target")
	}
	selectionValue, ok := env.GetWorkingValue("euclo.route_selection")
	if !ok {
		t.Fatal("expected route_selection in envelope")
	}
	routeSelection, ok := selectionValue.(*orchestrate.RouteSelection)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %T", selectionValue)
	}
	if routeSelection.RouteKind != orchestrate.RouteKindThoughtRecipe || routeSelection.ThoughtRecipeID != "euclo.thoughtrecipe.review" {
		t.Fatalf("unexpected route selection: %+v", routeSelection)
	}
	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %q, want thoughtrecipe", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.thoughtrecipe_id"); got != "euclo.thoughtrecipe.review" {
		t.Fatalf("execution thoughtrecipe id = %q, want euclo.thoughtrecipe.review", got)
	}
	if frameValue, ok := env.GetWorkingValue("euclo.interaction.clarification_frame"); ok && frameValue != nil {
		t.Fatalf("expected grounding to avoid clarification frame, got %#v", frameValue)
	}
}

package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func TestEndToEndRootRouteOnlyThoughtRecipeExecution(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.code_review", "euclo:cap.capture", "euclo:cap.consume")
	thoughtrecipes := newThoughtRecipeRegistry(t, &thoughtrecipepkg.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.review",
		Name:     "review",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{Name: "review"},
	})
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
	)

	env := contextdata.NewEnvelope("task-thoughtrecipe", "session-thoughtrecipe")
	seedTask(env, "review the auth package", "review.go")
	env.SetWorkingValue("euclo.route_selection", &orchestrate.RouteSelection{
		RouteKind:       "thoughtrecipe",
		ThoughtRecipeID: "euclo.thoughtrecipe.review",
	}, contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"review.go"})

	if err := graph.Execute(ctxWithTrigger(context.Background()), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %q, want thoughtrecipe", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.thoughtrecipe_id"); got != "euclo.thoughtrecipe.review" {
		t.Fatalf("thoughtrecipe id = %q, want euclo.thoughtrecipe.review", got)
	}
	if got := mustStringValue(t, env, "euclo.fork.branch"); got != "thoughtrecipe_execution" {
		t.Fatalf("fork branch = %q, want thoughtrecipe_execution", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected thoughtrecipe execution to complete")
	}
	if got, ok := env.GetWorkingValue("euclo.execution.capability_id"); ok && got != "" {
		t.Fatalf("expected thoughtrecipe-only execution, got capability id %v", got)
	}
}

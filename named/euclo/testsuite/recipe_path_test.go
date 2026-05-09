package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func TestDryRunEndToEndThoughtRecipePath(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "review.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.code_review", "euclo:cap.capture", "euclo:cap.consume")
	thoughtrecipes := newThoughtRecipeRegistry(t, &thoughtrecipepkg.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.review",
		Name:     "review",
		Metadata: thoughtrecipepkg.ThoughtRecipeMetadata{Name: "review"},
	})
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"review": {Sequence: []string{"euclo:cap.code_review"}, Operator: "OR"},
		},
	}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithThoughtRecipeRegistry(thoughtrecipes),
		orchestrate.WithCapabilityClassifier(classifier),
	)

	env := contextdata.NewEnvelope("task-thoughtrecipe", "session-thoughtrecipe")
	seedTask(env, "review the auth package", "review.go")
	env.SetWorkingValue("euclo.thoughtrecipe_id", "euclo.thoughtrecipe.review", contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"review.go"})
	telemetry := &recordingTelemetry{}

	if err := graph.Execute(core.WithTelemetry(ctxWithTrigger(context.Background()), telemetry), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %q, want thoughtrecipe", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.thoughtrecipe_id"); got != "euclo.thoughtrecipe.review" {
		t.Fatalf("thoughtrecipe id = %q, want euclo.thoughtrecipe.review", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected thoughtrecipe execution to complete")
	}
	if classifier.callCount() == 0 {
		t.Fatal("expected tier-2 classifier to be invoked")
	}
}

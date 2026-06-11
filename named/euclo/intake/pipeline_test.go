package intake

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

func TestIntakePipelineNodeCoordinatorOnly(t *testing.T) {
	registry := families.NewRegistry()
	_ = families.RegisterBuiltins(registry)

	node := NewIntakePipelineNode("intake", registry, 1024, contextstream.ModeBlocking, &MockStreamTrigger{})
	env := contextdata.NewEnvelope("task-1", "session-1")
	contextdata.SetTyped(env, "task.input", &execution.Task{
		ID:          "task-1",
		Instruction: "review named/euclo/intake/pipeline.go",
	})

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if _, ok := contextdata.GetTyped[any](env, "euclo.capability_sequence"); ok {
		t.Fatal("did not expect capability sequence in intake envelope")
	}
	if _, ok := contextdata.GetTyped[any](env, "euclo.capability_operator"); ok {
		t.Fatal("did not expect capability operator in intake envelope")
	}
	if got, ok := contextdata.GetTyped[*IntentClassification](env, "euclo.intent_classification"); !ok || got == nil {
		t.Fatal("expected intent classification in envelope")
	}
	if got, ok := execution.ResultField(result.Data, "stream_result"); !ok || got == nil {
		t.Fatal("expected structured stream result in result data")
	}
	if got, ok := execution.ResultField(result.Data, "family_selection"); !ok || got == nil {
		t.Fatal("expected family selection data")
	} else if familySelection, ok := got.(map[string]any); !ok || familySelection == nil {
		t.Fatal("expected family selection map")
	} else if source, _ := familySelection["source"].(string); source != "deterministic" {
		t.Fatalf("family selection source = %q, want deterministic", source)
	}
}

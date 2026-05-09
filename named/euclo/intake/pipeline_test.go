package intake

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

func TestIntakePipelineNodeCoordinatorOnly(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	node := NewIntakePipelineNode("intake", registry, 1024, contextstream.ModeBlocking, &MockStreamTrigger{})
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("task.input", &core.Task{
		ID:          "task-1",
		Instruction: "review named/euclo/intake/pipeline.go",
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if _, ok := env.GetWorkingValue("euclo.capability_sequence"); ok {
		t.Fatal("did not expect capability sequence in intake envelope")
	}
	if _, ok := env.GetWorkingValue("euclo.capability_operator"); ok {
		t.Fatal("did not expect capability operator in intake envelope")
	}
	if got, ok := env.GetWorkingValue("euclo.intent_classification"); !ok || got == nil {
		t.Fatal("expected intent classification in envelope")
	}
	if got, ok := result.Data["stream_result"]; !ok || got == nil {
		t.Fatal("expected structured stream result in result data")
	}
	if got, ok := result.Data["family_selection"].(map[string]any); !ok || got == nil {
		t.Fatal("expected family selection data")
	} else if source, _ := got["source"].(string); source != "deterministic" {
		t.Fatalf("family selection source = %q, want deterministic", source)
	}
}

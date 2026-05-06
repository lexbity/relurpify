package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

func TestDryRunEndToEndPreIngestionAndFinalResponse(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "preingest.go", "package demo\n\nfunc PreIngest() {}\n")

	caps := newCapabilityRegistry(t, "euclo:cap.ast_query")
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"investigation": {Sequence: []string{"euclo:cap.ast_query"}, Operator: "OR"},
		},
	}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityClassifier(classifier),
		orchestrate.WithCapabilityRegistry(caps),
	)

	env := contextdata.NewEnvelope("task-preingest", "session-preingest")
	seedTask(env, "investigate the handler", "preingest.go")
	runPreIngestion(t, env, dir, []string{"preingest.go"})
	if got, ok := env.GetWorkingValue("euclo.ingestion_result"); !ok || got == nil {
		t.Fatal("expected ingestion result to be recorded")
	}

	if err := graph.Execute(context.Background(), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "capability" {
		t.Fatalf("execution kind = %q, want capability", got)
	}
	if got := mustStringValue(t, env, "euclo.outcome.category"); got != "success" {
		t.Fatalf("outcome category = %q, want success", got)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete")
	}
	if classifier.callCount() == 0 {
		t.Fatal("expected tier-2 classifier to be invoked")
	}
}

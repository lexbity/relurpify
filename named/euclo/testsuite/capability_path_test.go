package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

func TestDryRunEndToEndCapabilityPath(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "handler.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"implementation": {Sequence: []string{"euclo:cap.targeted_refactor"}, Operator: "OR"},
		},
	}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithCapabilityClassifier(classifier),
	)

	env := contextdata.NewEnvelope("task-capability", "session-capability")
	seedTask(env, "add a cache to the handler", "handler.go")
	runPreIngestion(t, env, dir, []string{"handler.go"})
	telemetry := &recordingTelemetry{}

	if err := graph.Execute(core.WithTelemetry(context.Background(), telemetry), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "capability" {
		t.Fatalf("execution kind = %q, want capability", got)
	}
	selection, ok := env.GetWorkingValue("euclo.route_selection")
	if !ok {
		t.Fatal("expected route_selection in envelope")
	}
	routeSelection, ok := selection.(*orchestrate.RouteSelection)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %T", selection)
	}
	if routeSelection.CapabilityID != "euclo:cap.targeted_refactor" {
		t.Fatalf("capability route = %q, want euclo:cap.targeted_refactor", routeSelection.CapabilityID)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete")
	}
	if classifier.callCount() == 0 {
		t.Fatal("expected tier-2 classifier to be invoked")
	}
	assertEventOrder(t, telemetry.types(), []core.EventType{
		core.EventType("euclo.route.selected"),
		core.EventType("euclo.route.completed"),
		core.EventType("euclo.execution.complete"),
	})
}

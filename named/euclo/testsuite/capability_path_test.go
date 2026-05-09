package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

func TestEndToEndRootRouteOnlyCapabilityExecution(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "handler.go", "package demo\n")

	handler := &countingCapabilityHandler{id: "euclo:cap.targeted_refactor"}
	caps := capabilityRegistryWithHandler(t, handler)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
	)

	env := contextdata.NewEnvelope("task-capability", "session-capability")
	seedTask(env, "add a cache to the handler", "handler.go")
	env.SetWorkingValue("euclo.route_selection", &orchestrate.RouteSelection{
		RouteKind:    orchestrate.RouteKindCapability,
		CapabilityID: handler.id,
	}, contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"handler.go"})

	if err := graph.Execute(context.Background(), env); err != nil {
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
	if routeSelection.RouteKind != orchestrate.RouteKindCapability || routeSelection.CapabilityID != handler.id {
		t.Fatalf("unexpected capability route selection: %+v", routeSelection)
	}
	if got := mustStringValue(t, env, "euclo.execution.capability_id"); got != handler.id {
		t.Fatalf("execution capability id = %q, want %q", got, handler.id)
	}
	if got := mustStringValue(t, env, "euclo.fork.branch"); got != "capability_execution" {
		t.Fatalf("fork branch = %q, want capability_execution", got)
	}
	if handler.Count() != 1 {
		t.Fatalf("expected capability to execute once, got %d", handler.Count())
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete")
	}
}

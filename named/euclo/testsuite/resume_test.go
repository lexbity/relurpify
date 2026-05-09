package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

func TestEndToEndUnresolvedRouteWarningAndResume(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "resume.go", "package demo\n")

	const capabilityID = "euclo:cap.resume_route"

	missingCaps := capability.NewCapabilityRegistry()
	missingGraph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(missingCaps)),
		orchestrate.WithCapabilityRegistry(missingCaps),
	)

	env := contextdata.NewEnvelope("task-resume", "session-resume")
	seedTask(env, "add a cache to the handler", "resume.go")
	env.SetWorkingValue("euclo.route_selection", &orchestrate.RouteSelection{
		RouteKind:    orchestrate.RouteKindCapability,
		CapabilityID: capabilityID,
	}, contextdata.MemoryClassTask)

	firstTelemetry := &recordingTelemetry{}
	if err := missingGraph.Execute(core.WithTelemetry(context.Background(), firstTelemetry), env); err == nil {
		t.Fatal("expected unresolved route failure on first pass")
	}
	if !hasEventType(firstTelemetry.types(), core.EventType("euclo.route.unavailable")) {
		t.Fatalf("expected route unavailable warning, got %v", firstTelemetry.types())
	}

	resolutionValue, ok := env.GetWorkingValue("euclo.route_resolution")
	if !ok {
		t.Fatal("expected route_resolution in envelope")
	}
	resolution, ok := resolutionValue.(*orchestrate.RouteResolution)
	if !ok || resolution == nil {
		t.Fatalf("expected *RouteResolution, got %T", resolutionValue)
	}
	if resolution.ResolutionSource != "unresolved" {
		t.Fatalf("resolution source = %q, want unresolved", resolution.ResolutionSource)
	}
	if resolution.CapabilityID != capabilityID {
		t.Fatalf("resolution capability = %q, want %q", resolution.CapabilityID, capabilityID)
	}

	handler := &countingCapabilityHandler{id: capabilityID}
	resolvedCaps := capabilityRegistryWithHandler(t, handler)
	resolvedGraph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(resolvedCaps)),
		orchestrate.WithCapabilityRegistry(resolvedCaps),
	)

	secondTelemetry := &recordingTelemetry{}
	if err := resolvedGraph.Execute(core.WithTelemetry(context.Background(), secondTelemetry), env); err != nil {
		t.Fatalf("resume execute failed: %v", err)
	}
	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "capability" {
		t.Fatalf("execution kind = %q, want capability", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.capability_id"); got != capabilityID {
		t.Fatalf("execution capability id = %q, want %q", got, capabilityID)
	}
	if got := mustStringValue(t, env, "euclo.fork.branch"); got != "capability_execution" {
		t.Fatalf("fork branch = %q, want capability_execution", got)
	}
	if handler.Count() != 1 {
		t.Fatalf("expected capability to execute once after resume, got %d", handler.Count())
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete after resume")
	}
	if resolutionValue, ok := env.GetWorkingValue("euclo.route_resolution"); !ok {
		t.Fatal("expected route_resolution to remain available after resume")
	} else if resumedResolution, ok := resolutionValue.(*orchestrate.RouteResolution); !ok || resumedResolution == nil || resumedResolution.ResolutionSource != "registry" {
		t.Fatalf("unexpected resumed route resolution: %#v", resolutionValue)
	}
}

func hasEventType(got []core.EventType, want core.EventType) bool {
	for _, eventType := range got {
		if eventType == want {
			return true
		}
	}
	return false
}

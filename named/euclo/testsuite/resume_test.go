package testsuite

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestEndToEndUnresolvedRouteWarningAndResume(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "resume.go", "package demo\n")

	const capabilityID = "euclo:cap.resume_route"

	missingCaps := capability.NewRegistry()
	missingGraph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(missingCaps)),
		orchestrate.WithCapabilityRegistry(missingCaps),
	)

	env := contextdata.NewEnvelope("task-resume", "session-resume")
	seedTask(env, "add a cache to the handler", "resume.go")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    euclotypes.RouteKindCapability,
		CapabilityID: capabilityID,
	})

	firstTelemetry := &recordingTelemetry{}
	if err := missingGraph.Execute(telemetry.WithTelemetry(context.Background(), firstTelemetry), env); err == nil {
		t.Fatal("expected unresolved route failure on first pass")
	}
	if !hasEventType(firstTelemetry.types(), telemetry.EventType("euclo.route.unavailable")) {
		t.Fatalf("expected route unavailable warning, got %v", firstTelemetry.types())
	}

	resolutionValue, ok := state.GetRouteResolution(env)
	if !ok {
		t.Fatal("expected route_resolution in envelope")
	}
	resolution := resolutionValue
	if resolution == nil {
		t.Fatalf("expected *RouteResolution, got %#v", resolutionValue)
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
	if err := resolvedGraph.Execute(telemetry.WithTelemetry(context.Background(), secondTelemetry), env); err != nil {
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
	if resolutionValue, ok := state.GetRouteResolution(env); !ok {
		t.Fatal("expected route_resolution to remain available after resume")
	} else if resolutionValue == nil || resolutionValue.ResolutionSource != "registry" {
		t.Fatalf("unexpected resumed route resolution: %#v", resolutionValue)
	}
}

func hasEventType(got []telemetry.EventType, want telemetry.EventType) bool {
	for _, eventType := range got {
		if eventType == want {
			return true
		}
	}
	return false
}

package testsuite

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
)

func TestEndToEndClarificationFirstRouteSelection(t *testing.T) {
	caps := newCapabilityRegistry(t)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
	)

	env := contextdata.NewEnvelope("task-clarification", "session-clarification")
	seedTask(env, "help me with this")
	telemetry := &recordingTelemetry{}

	if err := graph.Execute(core.WithTelemetry(context.Background(), telemetry), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	selection, ok := env.GetWorkingValue("euclo.route_selection")
	if !ok {
		t.Fatal("expected route_selection in envelope")
	}
	routeSelection, ok := selection.(*orchestrate.RouteSelection)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %T", selection)
	}
	if routeSelection.RouteKind != orchestrate.RouteKindIntent || routeSelection.ThoughtRecipeID != "euclo.thoughtrecipe.intent.clarify" {
		t.Fatalf("unexpected clarification route selection: %+v", routeSelection)
	}
	resolution, ok := env.GetWorkingValue("euclo.route_resolution")
	if !ok {
		t.Fatal("expected route_resolution in envelope")
	}
	routeResolution, ok := resolution.(*orchestrate.RouteResolution)
	if !ok || routeResolution == nil {
		t.Fatalf("expected *RouteResolution, got %T", resolution)
	}
	if routeResolution.ResolutionSource != "clarification" {
		t.Fatalf("route resolution source = %q, want clarification", routeResolution.ResolutionSource)
	}
	if got := mustStringValue(t, env, "euclo.execution.kind"); got != "thoughtrecipe" {
		t.Fatalf("execution kind = %q, want thoughtrecipe", got)
	}
	if got := mustStringValue(t, env, "euclo.execution.thoughtrecipe_id"); got != "euclo.thoughtrecipe.intent.clarify" {
		t.Fatalf("execution thoughtrecipe id = %q, want clarification thoughtrecipe", got)
	}
	if frameValue, ok := env.GetWorkingValue("euclo.interaction.clarification_frame"); !ok {
		t.Fatal("expected clarification frame in envelope")
	} else if frame, ok := frameValue.(*orchestrate.ClarificationFrame); !ok || frame == nil || !frame.Pending() {
		t.Fatalf("unexpected clarification frame: %#v", frameValue)
	}
	if gateValue, ok := env.GetWorkingValue("euclo.clarification.gate_result"); !ok {
		t.Fatal("expected clarification gate result in envelope")
	} else if gate, ok := gateValue.(map[string]any); !ok || gate["decision"] != "clarify" {
		t.Fatalf("unexpected gate result: %#v", gateValue)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete")
	}
	for _, eventType := range []core.EventType{
		core.EventType("euclo.clarification.started"),
		core.EventType("euclo.route.selected"),
		core.EventType("euclo.route.completed"),
		core.EventType("euclo.execution.complete"),
	} {
		if !hasEventType(telemetry.types(), eventType) {
			t.Fatalf("expected telemetry event %s, got %v", eventType, telemetry.types())
		}
	}
}

func TestDryRunEndToEndAmbiguousInteractionAndHITL(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "mixed.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	broker := authorization.NewHITLBroker(5 * time.Second)
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnvWithModel(caps, stubLanguageModel{})),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithHITLBroker(broker),
	)

	env := contextdata.NewEnvelope("task-interaction", "session-interaction")
	seedTask(env, "implement and review the handler", "mixed.go")
	env.SetWorkingValue("euclo.policy_decision", &policy.PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"approval_required"},
	}, contextdata.MemoryClassTask)
	runPreIngestion(t, env, dir, []string{"mixed.go"})

	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pending := broker.PendingRequests()
			if len(pending) > 0 {
				_ = broker.Approve(authorization.PermissionDecision{
					RequestID:  pending[0].ID,
					Approved:   true,
					ApprovedBy: "testsuite",
					Scope:      authorization.GrantScopeOneTime,
				})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if err := graph.Execute(context.Background(), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}
	<-done

	if !mustBoolValue(t, env, "euclo.interaction.frame_requested") {
		t.Fatal("expected interaction frame to be requested")
	}
	if !mustBoolValue(t, env, "euclo.hitl_triggered") {
		t.Fatal("expected HITL to be triggered")
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete after HITL approval")
	}
}

package testsuite

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/authorization"
	govpolicy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestEndToEndClarificationFirstRouteSelection(t *testing.T) {
	caps := newCapabilityRegistry(t)
	deps := rootGraphDepsWithModel(caps, stubLanguageModel{})
	graph, err := orchestrate.NewRootGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-clarification", "session-clarification")
	seedTask(env, "help me with this")
	rec := &recordingTelemetry{}

	if err := graph.Execute(telemetry.WithTelemetry(context.Background(), rec), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	routeSelection, ok := state.GetRouteSelection(env)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %#v", routeSelection)
	}
	if routeSelection.RouteKind != euclotypes.RouteKindIntent || routeSelection.ThoughtRecipeID != "euclo.thoughtrecipe.intent.clarify" {
		t.Fatalf("unexpected clarification route selection: %+v", routeSelection)
	}
	routeResolution, ok := state.GetRouteResolution(env)
	if !ok || routeResolution == nil {
		t.Fatalf("expected *RouteResolution, got %#v", routeResolution)
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
	if frameValue, ok := contextdata.GetTyped[*orchestrate.ClarificationFrame](env, state.KeyClarificationFrame); !ok || frameValue == nil || !frameValue.Pending() {
		t.Fatalf("unexpected clarification frame: %#v", frameValue)
	}
	if gateValue, ok := state.GetClarificationGateResult(env); !ok || gateValue["decision"] != "clarify" {
		t.Fatalf("unexpected gate result: %#v", gateValue)
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete")
	}
	for _, eventType := range []telemetry.EventType{
		telemetry.EventType("euclo.clarification.started"),
		telemetry.EventType("euclo.route.selected"),
		telemetry.EventType("euclo.route.completed"),
		telemetry.EventType("euclo.execution.complete"),
	} {
		if !hasEventType(rec.types(), eventType) {
			t.Fatalf("expected telemetry event %s, got %v", eventType, rec.types())
		}
	}
}

func TestDryRunEndToEndAmbiguousInteractionAndHITL(t *testing.T) {
	dir := t.TempDir()
	writeWorkspaceFile(t, dir, "mixed.go", "package demo\n")

	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	broker := authorization.NewHITLBroker(5*time.Second, nil)
	deps := rootGraphDepsWithModel(caps, stubLanguageModel{})
	deps.HITLBroker = broker
	graph, err := orchestrate.NewRootGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-interaction", "session-interaction")
	seedTask(env, "implement and review the handler", "mixed.go")
	state.SetPolicyDecision(env, &policy.PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         true,
		VerificationRequired: false,
		ReasonCodes:          []string{"approval_required"},
	})
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
					Scope:      govpolicy.GrantScopeOneTime,
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

	// The write-only frame-requested flag was deleted (D8); the surviving
	// contract is the frame itself: a policy-frame run must have produced at
	// least one interaction frame on the envelope.
	if !envelopeHasInteractionFrame(t, env) {
		t.Fatal("expected an interaction frame on the envelope")
	}
	if !mustBoolValue(t, env, "euclo.hitl_triggered") {
		t.Fatal("expected HITL to be triggered")
	}
	if !mustBoolValue(t, env, "euclo.execution.completed") {
		t.Fatal("expected execution to complete after HITL approval")
	}
}

// envelopeHasInteractionFrame scans the envelope's working memory for any
// interaction frame — the durable artifact of a frame-requesting run.
func envelopeHasInteractionFrame(t *testing.T, env *contextdata.Envelope) bool {
	t.Helper()
	for _, key := range env.WorkingMemoryKeys() {
		if value, ok := contextdata.GetTyped[any](env, key); ok {
			if frame, ok := value.(*interaction.InteractionFrame); ok && frame != nil {
				return true
			}
		}
	}
	return false
}

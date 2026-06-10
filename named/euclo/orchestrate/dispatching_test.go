package orchestrate

import (
	"context"
	"errors"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type telemetrySink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *telemetrySink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *telemetrySink) snapshot() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]telemetry.Event, len(s.events))
	copy(out, s.events)
	return out
}

func testCapabilityDescriptor(id string, priority int, availability descriptor.AvailabilitySpec) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            id,
		Name:          id,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  availability,
		Annotations: map[string]any{
			"euclo.priority": priority,
		},
	}
}

func testThoughtRecipe(id string) *surface.ThoughtRecipe {
	return &surface.ThoughtRecipe{
		ID:   id,
		Name: id,
	}
}

func TestDispatch_ExplicitCapabilityRoute_SelectsRequestedCapability(t *testing.T) {
	reg := registry.NewRegistry()
	desc := testCapabilityDescriptor("euclo:cap.ast_query", 10, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	req := RouteRequest{CapabilityID: desc.ID}

	result, err := Dispatch(context.Background(), env, req, reg, nil)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.RouteKind != "capability" {
		t.Fatalf("expected capability route, got %q", result.RouteKind)
	}
	if result.RouteID != desc.ID {
		t.Fatalf("expected route %q, got %q", desc.ID, result.RouteID)
	}
	routeSelection, ok := state.GetRouteSelection(env)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *euclotypes.RouteSelection, got %#v", routeSelection)
	}
	if routeSelection.RouteKind != "capability" || routeSelection.CapabilityID != desc.ID {
		t.Fatalf("unexpected route selection: %+v", routeSelection)
	}
}

func TestDispatch_ExplicitThoughtRecipeRoute_SelectsRequestedThoughtRecipe(t *testing.T) {
	thoughtrecipes := thoughtrecipepkg.NewThoughtRecipeRegistry()
	thoughtrecipe := testThoughtRecipe("euclo:thoughtrecipe.review")
	if err := thoughtrecipes.Register(thoughtrecipe); err != nil {
		t.Fatalf("register thoughtrecipe: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	req := RouteRequest{ThoughtRecipeID: thoughtrecipe.ID}

	result, err := Dispatch(context.Background(), env, req, nil, thoughtrecipes)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result.RouteKind != "thoughtrecipe" {
		t.Fatalf("expected thoughtrecipe route, got %q", result.RouteKind)
	}
	if result.RouteID != thoughtrecipe.ID {
		t.Fatalf("expected thoughtrecipe %q, got %q", thoughtrecipe.ID, result.RouteID)
	}
}

func TestDispatch_AmbiguousClassificationRoutesToClarificationThoughtRecipe(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetIntentClassification(env, &intake.IntentClassification{
		Ambiguous:  true,
		Confidence: 0.2,
	})
	req := routeRequestFromEnvelope(env)
	if req.ThoughtRecipeID != clarificationThoughtRecipeID {
		t.Fatalf("route request thoughtrecipe id = %q, want %q", req.ThoughtRecipeID, clarificationThoughtRecipeID)
	}
	if got := routeKindFromRequest(req); got != euclotypes.RouteKindIntent {
		t.Fatalf("route kind from request = %q, want intent; request=%+v", got, req)
	}
	directResult, directErr := Dispatch(context.Background(), env, req, nil, nil)
	if directErr != nil {
		t.Fatalf("direct Dispatch failed: %v", directErr)
	}
	if directResult.RouteKind != euclotypes.RouteKindIntent {
		t.Fatalf("direct dispatch route kind = %q, want intent", directResult.RouteKind)
	}

	dispatcher := NewDispatcher("test-dispatch")
	resultCore, err := dispatcher.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Dispatcher.Execute failed: %v", err)
	}
	if resultCore == nil {
		t.Fatal("expected result")
	}
	if selection, ok := state.GetRouteSelection(env); !ok || selection == nil || selection.RouteKind != euclotypes.RouteKindIntent || selection.ThoughtRecipeID != clarificationThoughtRecipeID {
		t.Fatalf("unexpected route selection: %#v", selection)
	}
	if meta, ok := state.GetRouteContinuation(env); !ok || meta == nil || !meta.SharedContext || meta.TargetRouteID != clarificationThoughtRecipeID {
		t.Fatalf("unexpected route continuation: %#v", meta)
	}
	if got := mustStringRouteValue(t, env, "euclo.dispatch.route_kind"); got != euclotypes.RouteKindIntent {
		t.Fatalf("dispatch route kind = %q, want intent", got)
	}
}

func mustStringRouteValue(t *testing.T, env *contextdata.Envelope, key string) string {
	t.Helper()
	value, ok := env.GetWorkingValue(key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	s, ok := value.(string)
	if !ok {
		t.Fatalf("envelope value %q is %T, want string", key, value)
	}
	return s
}

func TestDispatch_FamilyRoute_SelectsBestCandidate(t *testing.T) {
	reg := registry.NewRegistry()
	low := testCapabilityDescriptor("euclo:cap.ast_query", 5, descriptor.AvailabilitySpec{Available: true})
	high := testCapabilityDescriptor("euclo:cap.symbol_trace", 20, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(low); err != nil {
		t.Fatalf("register low capability: %v", err)
	}
	if err := reg.RegisterCapability(high); err != nil {
		t.Fatalf("register high capability: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetFamilySelection(env, "query")

	result, err := Dispatch(context.Background(), env, RouteRequest{FamilyID: "query"}, reg, nil)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result.RouteID != high.ID {
		t.Fatalf("expected highest ranked candidate %q, got %q", high.ID, result.RouteID)
	}
}

func TestRouteMatchesFamily_EmptyFamilyUsesAnalysisHints(t *testing.T) {
	queryCap := testCapabilityDescriptor("euclo:cap.ast_query", 0, descriptor.AvailabilitySpec{Available: true})
	refactorCap := testCapabilityDescriptor("euclo:cap.targeted_refactor", 0, descriptor.AvailabilitySpec{Available: true})

	if !routeMatchesFamily(queryCap, "", "analyze the codebase and explain the flow") {
		t.Fatal("expected analysis instruction to match query capability")
	}
	if routeMatchesFamily(refactorCap, "", "analyze the codebase and explain the flow") {
		t.Fatal("expected analysis instruction not to match mutation capability")
	}
}

func TestRouteMatchesFamily_EmptyFamilyUsesMutationHints(t *testing.T) {
	queryCap := testCapabilityDescriptor("euclo:cap.ast_query", 0, descriptor.AvailabilitySpec{Available: true})
	refactorCap := testCapabilityDescriptor("euclo:cap.targeted_refactor", 0, descriptor.AvailabilitySpec{Available: true})

	if !routeMatchesFamily(refactorCap, "", "modify the handler to support retries") {
		t.Fatal("expected mutation instruction to match mutation capability")
	}
	if routeMatchesFamily(queryCap, "", "modify the handler to support retries") {
		t.Fatal("expected mutation instruction not to match query capability")
	}
}

func TestRouteMatchesFamily_EmptyFamilyRejectsUnrelatedInstruction(t *testing.T) {
	queryCap := testCapabilityDescriptor("euclo:cap.ast_query", 0, descriptor.AvailabilitySpec{Available: true})
	if routeMatchesFamily(queryCap, "", "write a release note") {
		t.Fatal("expected unrelated instruction to not match query capability")
	}
}

func TestDryRun_EmitsRouteDryRunEvent(t *testing.T) {
	reg := registry.NewRegistry()
	low := testCapabilityDescriptor("euclo:cap.ast_query", 5, descriptor.AvailabilitySpec{Available: true})
	high := testCapabilityDescriptor("euclo:cap.symbol_trace", 20, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(low); err != nil {
		t.Fatalf("register low capability: %v", err)
	}
	if err := reg.RegisterCapability(high); err != nil {
		t.Fatalf("register high capability: %v", err)
	}

	sink := &telemetrySink{}
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	report, err := DryRun(ctx, contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{FamilyID: "query", DryRun: true}, reg, nil)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if report == nil {
		t.Fatal("expected dry-run report")
	}
	if report.SelectedRoute != RouteID(high.ID) {
		t.Fatalf("expected selected route %q, got %q", high.ID, report.SelectedRoute)
	}
	events := sink.snapshot()
	found := false
	for _, event := range events {
		if event.Type == telemetry.EventType("euclo.route.dry_run") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected euclo.route.dry_run event, got %v", events)
	}
}

func TestDispatch_EmitsRouteSelectedEvent(t *testing.T) {
	reg := registry.NewRegistry()
	desc := testCapabilityDescriptor("euclo:cap.ast_query", 10, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	sink := &telemetrySink{}
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	env := contextdata.NewEnvelope("task-1", "session-1")
	if _, err := Dispatch(ctx, env, RouteRequest{CapabilityID: desc.ID}, reg, nil); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	events := sink.snapshot()
	found := false
	for _, event := range events {
		if event.Type == telemetry.EventType("euclo.route.selected") {
			found = true
			if got := event.Metadata["route_id"]; got != desc.ID {
				t.Fatalf("expected selected event route_id %q, got %v", desc.ID, got)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected euclo.route.selected event, got %v", events)
	}
}

func TestDispatch_UnavailableRoute_ReturnsError(t *testing.T) {
	reg := registry.NewRegistry()
	desc := testCapabilityDescriptor("euclo:cap.targeted_refactor", 10, descriptor.AvailabilitySpec{
		Available: false,
		Reason:    "tool dependency missing: file_write",
	})
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	_, err := Dispatch(context.Background(), contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{CapabilityID: desc.ID}, reg, nil)
	if err == nil {
		t.Fatal("expected error for unavailable route")
	}
}

func TestDispatch_UnavailableCapability_RemainsUnresolved(t *testing.T) {
	reg := registry.NewRegistry()
	primary := testCapabilityDescriptor("euclo:cap.targeted_refactor", 10, descriptor.AvailabilitySpec{
		Available: false,
		Reason:    "tool dependency missing: file_write",
	})
	fallback := testCapabilityDescriptor("euclo:cap.ast_query", 1, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := reg.RegisterCapability(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	req := RouteRequest{CapabilityID: primary.ID, FallbackID: fallback.ID}

	result, err := Dispatch(context.Background(), env, req, reg, nil)
	if err != nil {
		routeResolutionError := &RouteResolutionError{}
		if errors.As(err, &routeResolutionError) {
			t.Fatalf("expected RouteResolutionError, got %T", err)
		}
	} else {
		t.Fatalf("expected unresolved dispatch, got result %+v", result)
	}
	if resolution, ok := state.GetRouteResolution(env); !ok || resolution == nil || resolution.RouteID() != primary.ID {
		t.Fatalf("expected unresolved route resolution for %q, got %#v", primary.ID, resolution)
	}
}

func TestDispatch_AllUnavailable_HardFailure(t *testing.T) {
	reg := registry.NewRegistry()
	desc := testCapabilityDescriptor("euclo:cap.targeted_refactor", 10, descriptor.AvailabilitySpec{
		Available: false,
		Reason:    "tool dependency missing: file_write",
	})
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	_, err := Dispatch(context.Background(), contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{CapabilityID: desc.ID}, reg, nil)
	if err == nil {
		t.Fatal("expected hard failure when no route is available")
	}
	routeResolutionError := &RouteResolutionError{}
	if errors.As(err, &routeResolutionError) {
		t.Fatalf("expected RouteResolutionError, got %T", err)
	}
}

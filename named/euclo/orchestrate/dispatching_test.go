package orchestrate

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

type telemetrySink struct {
	mu     sync.Mutex
	events []core.Event
}

func (s *telemetrySink) Emit(event core.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *telemetrySink) snapshot() []core.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]core.Event, len(s.events))
	copy(out, s.events)
	return out
}

func testCapabilityDescriptor(id string, priority int, availability core.AvailabilitySpec) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            id,
		Name:          id,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Availability:  availability,
		Annotations: map[string]any{
			"euclo.priority": priority,
		},
	}
}

func testRecipe(id string) *recipepkg.ThoughtRecipe {
	return &recipepkg.ThoughtRecipe{
		ID:   id,
		Name: id,
		Steps: []recipepkg.RecipeStep{
			{ID: "step-1", Type: "verify"},
		},
	}
}

func TestDispatch_ExplicitCapabilityRoute_SelectsRequestedCapability(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	desc := testCapabilityDescriptor("euclo:cap.ast_query", 10, core.AvailabilitySpec{Available: true})
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
	if got, ok := env.GetWorkingValue("euclo.route.kind"); !ok || got != "capability" {
		t.Fatalf("expected envelope route.kind capability, got %v (ok=%v)", got, ok)
	}
}

func TestDispatch_ExplicitRecipeRoute_SelectsRequestedRecipe(t *testing.T) {
	recipes := recipepkg.NewRecipeRegistry()
	recipe := testRecipe("euclo:recipe.review")
	if err := recipes.Register(recipe); err != nil {
		t.Fatalf("register recipe: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	req := RouteRequest{RecipeID: recipe.ID}

	result, err := Dispatch(context.Background(), env, req, nil, recipes)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result.RouteKind != "recipe" {
		t.Fatalf("expected recipe route, got %q", result.RouteKind)
	}
	if result.RouteID != recipe.ID {
		t.Fatalf("expected recipe %q, got %q", recipe.ID, result.RouteID)
	}
}

func TestDispatch_FamilyRoute_SelectsBestCandidate(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	low := testCapabilityDescriptor("euclo:cap.ast_query", 5, core.AvailabilitySpec{Available: true})
	high := testCapabilityDescriptor("euclo:cap.symbol_trace", 20, core.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(low); err != nil {
		t.Fatalf("register low capability: %v", err)
	}
	if err := reg.RegisterCapability(high); err != nil {
		t.Fatalf("register high capability: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.family_selection", "query", contextdata.MemoryClassTask)

	result, err := Dispatch(context.Background(), env, RouteRequest{FamilyID: "query"}, reg, nil)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result.RouteID != high.ID {
		t.Fatalf("expected highest ranked candidate %q, got %q", high.ID, result.RouteID)
	}
}

func TestDryRun_EmitsRouteDryRunEvent(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	low := testCapabilityDescriptor("euclo:cap.ast_query", 5, core.AvailabilitySpec{Available: true})
	high := testCapabilityDescriptor("euclo:cap.symbol_trace", 20, core.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(low); err != nil {
		t.Fatalf("register low capability: %v", err)
	}
	if err := reg.RegisterCapability(high); err != nil {
		t.Fatalf("register high capability: %v", err)
	}

	sink := &telemetrySink{}
	ctx := core.WithTelemetry(context.Background(), sink)

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
		if event.Type == core.EventType("euclo.route.dry_run") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected euclo.route.dry_run event, got %v", events)
	}
}

func TestDispatch_EmitsRouteSelectedEvent(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	desc := testCapabilityDescriptor("euclo:cap.ast_query", 10, core.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	sink := &telemetrySink{}
	ctx := core.WithTelemetry(context.Background(), sink)

	env := contextdata.NewEnvelope("task-1", "session-1")
	if _, err := Dispatch(ctx, env, RouteRequest{CapabilityID: desc.ID}, reg, nil); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	events := sink.snapshot()
	found := false
	for _, event := range events {
		if event.Type == core.EventType("euclo.route.selected") {
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
	reg := capability.NewCapabilityRegistry()
	desc := testCapabilityDescriptor("euclo:cap.targeted_refactor", 10, core.AvailabilitySpec{
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

func TestDispatch_UnavailableCapability_TriesFallback(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	primary := testCapabilityDescriptor("euclo:cap.targeted_refactor", 10, core.AvailabilitySpec{
		Available: false,
		Reason:    "tool dependency missing: file_write",
	})
	fallback := testCapabilityDescriptor("euclo:cap.ast_query", 1, core.AvailabilitySpec{Available: true})
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
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result.RouteID != fallback.ID {
		t.Fatalf("expected fallback route %q, got %q", fallback.ID, result.RouteID)
	}
	if !result.FallbackTaken {
		t.Fatal("expected fallback to be reported as taken")
	}
	if result.FallbackID != fallback.ID {
		t.Fatalf("expected fallback ID %q, got %q", fallback.ID, result.FallbackID)
	}
}

func TestDispatch_AllUnavailable_HardFailure(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	desc := testCapabilityDescriptor("euclo:cap.targeted_refactor", 10, core.AvailabilitySpec{
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
	if _, ok := err.(*RouteResolutionError); !ok {
		t.Fatalf("expected RouteResolutionError, got %T", err)
	}
}

package agenttest

import (
	"testing"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestEvaluateBenchmarkSelectionMatch(t *testing.T) {
	report := &CaseReport{RouteSelection: RouteSelectionReport{
		ChosenRoute:   "euclo.thoughtrecipe.debug_tdd_repair",
		DecidedBy:     "lattice:family_affinity",
		FallbackTaken: false,
	}}
	expected := false
	spec := &BenchmarkSpec{Selection: &SelectionAssertion{
		ChosenRoute:   "euclo.thoughtrecipe.debug_tdd_repair",
		DecidedBy:     "lattice:family_affinity",
		FallbackTaken: &expected,
	}}
	eval := EvaluateBenchmark(report, spec, nil)
	if len(eval.Failures) != 0 {
		t.Fatalf("expected matching selection to pass, got failures: %v", eval.Failures)
	}
	if len(eval.Observations) != 3 {
		t.Fatalf("expected 3 selection observations, got %d", len(eval.Observations))
	}
}

func TestEvaluateBenchmarkSelectionMismatchFails(t *testing.T) {
	report := &CaseReport{RouteSelection: RouteSelectionReport{
		ChosenRoute:   "relurpic:debug_trace",
		DecidedBy:     "lattice:score",
		FallbackTaken: true,
	}}
	expected := false
	spec := &BenchmarkSpec{Selection: &SelectionAssertion{
		ChosenRoute:   "euclo.thoughtrecipe.debug_tdd_repair",
		DecidedBy:     "lattice:family_affinity",
		FallbackTaken: &expected,
	}}
	eval := EvaluateBenchmark(report, spec, nil)
	if len(eval.Failures) != 3 {
		t.Fatalf("expected 3 failures for full mismatch, got %v", eval.Failures)
	}
}

func TestEvaluateBenchmarkSelectionPartialDeclaredOnly(t *testing.T) {
	// Only the decided-by dimension is asserted; the others are unasserted.
	report := &CaseReport{RouteSelection: RouteSelectionReport{ChosenRoute: "euclo:cap.ast_query", DecidedBy: "lattice:score"}}
	spec := &BenchmarkSpec{Selection: &SelectionAssertion{DecidedBy: "lattice:score"}}
	eval := EvaluateBenchmark(report, spec, nil)
	if len(eval.Failures) != 0 {
		t.Fatalf("expected partial assertion to pass, got %v", eval.Failures)
	}
	if len(eval.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(eval.Observations))
	}
}

func TestRouteSelectionFromEvents(t *testing.T) {
	events := []telemetry.Event{
		{Type: "llm_prompt", Metadata: map[string]any{"route_id": "bogus"}},
		{Type: "euclo.route.selected", Metadata: map[string]any{
			"route_id":       "euclo.thoughtrecipe.debug_tdd_repair",
			"decided_by":     "lattice:family_affinity",
			"fallback_taken": true,
		}},
	}
	selection := routeSelectionFromEvents(events)
	if selection.ChosenRoute != "euclo.thoughtrecipe.debug_tdd_repair" {
		t.Fatalf("chosen route = %q", selection.ChosenRoute)
	}
	if selection.DecidedBy != "lattice:family_affinity" {
		t.Fatalf("decided by = %q", selection.DecidedBy)
	}
	if !selection.FallbackTaken {
		t.Fatal("fallback_taken must be true")
	}
}

func TestRouteSelectionFromEventsStableMessage(t *testing.T) {
	// A non-route event must not populate the report.
	selection := routeSelectionFromEvents([]telemetry.Event{{Type: "euclo.route.completed", Metadata: map[string]any{"route_id": "x"}}})
	if selection.ChosenRoute != "" {
		t.Fatalf("route.completed event must not populate selection: %#v", selection)
	}
}

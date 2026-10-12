package testsuite

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestDryRunEndToEndTelemetryOrder(t *testing.T) {
	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	deps := rootGraphDeps(caps)
	graph, err := orchestrate.NewRootGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-telemetry", "session-telemetry")
	seedTask(env, "add a cache to the handler")
	rec := &recordingTelemetry{}

	if err := graph.Execute(telemetry.WithTelemetry(context.Background(), rec), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	assertEventOrder(t, rec.types(), []telemetry.EventType{
		telemetry.EventType("euclo.route.selected"),
		telemetry.EventType("euclo.route.completed"),
		telemetry.EventType("euclo.execution.complete"),
	})
	// This composition arms neither workspace recipes nor the default
	// fallback (no file tools), so the zero-signal instruction stops at the
	// clarification last resort. The honest turn outcome is
	// awaiting_interaction — the graph finished, the clarification did not.
	if got, _ := euclostate.GetOutcomeCategory(env); strings.TrimSpace(got) != "awaiting_interaction" {
		resolution, _ := euclostate.GetRouteResolution(env)
		selection, _ := euclostate.GetRouteSelection(env)
		t.Fatalf("expected awaiting_interaction outcome, got %q (resolution=%+v selection=%+v)", got, resolution, selection)
	}
}

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
	graph := orchestrate.NewRootGraph(
		orchestrate.WithAgentContext(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
	)

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
	if got, _ := euclostate.GetOutcomeCategory(env); strings.TrimSpace(got) != "success" {
		t.Fatal("expected success outcome")
	}
}

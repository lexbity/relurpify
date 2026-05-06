package testsuite

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

func TestDryRunEndToEndTelemetryOrder(t *testing.T) {
	caps := newCapabilityRegistry(t, "euclo:cap.targeted_refactor")
	classifier := &mockTier2Classifier{
		responses: map[string]tier2Response{
			"implementation": {Sequence: []string{"euclo:cap.targeted_refactor"}, Operator: "OR"},
		},
	}
	graph := orchestrate.NewRootGraph(
		orchestrate.WithWorkspaceEnvironment(workspaceEnv(caps)),
		orchestrate.WithCapabilityRegistry(caps),
		orchestrate.WithCapabilityClassifier(classifier),
	)

	env := contextdata.NewEnvelope("task-telemetry", "session-telemetry")
	seedTask(env, "add a cache to the handler")
	telemetry := &recordingTelemetry{}

	if err := graph.Execute(core.WithTelemetry(context.Background(), telemetry), env); err != nil {
		t.Fatalf("graph execute failed: %v", err)
	}

	assertEventOrder(t, telemetry.types(), []core.EventType{
		core.EventType("euclo.route.selected"),
		core.EventType("euclo.route.completed"),
		core.EventType("euclo.execution.complete"),
	})
	if strings.TrimSpace(mustStringValue(t, env, "euclo.outcome.category")) != "success" {
		t.Fatal("expected success outcome")
	}
}

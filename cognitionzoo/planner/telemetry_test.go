package planner

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type plannerSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *plannerSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *plannerSink) hasType(eventType telemetry.EventType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func (s *plannerSink) types() []telemetry.EventType {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []telemetry.EventType
	for _, ev := range s.events {
		out = append(out, ev.Type)
	}
	return out
}

func requirePlannerEvent(t *testing.T, sink *plannerSink, eventType telemetry.EventType) {
	t.Helper()
	if !sink.hasType(eventType) {
		t.Fatalf("expected telemetry event %s, got %v", eventType, sink.types())
	}
}

// TestPlannerAgentEmitsPlanFailed drives the real Execute path with a missing
// model, which fails at graph construction — the planner.plan.failed event must
// surface through the configured telemetry sink (spec §1.7).
func TestPlannerAgentEmitsPlanFailed(t *testing.T) {
	sink := &plannerSink{}
	agent := &PlannerAgent{Config: &execution.Config{Telemetry: sink}}

	ctx := context.Background()
	task := &execution.Task{ID: "planner-fail-1"}
	_, err := agent.Execute(ctx, task, contextdata.NewEnvelope("planner-fail-1", "session"))
	if err == nil {
		t.Fatal("expected graph construction to fail without a model")
	}
	requirePlannerEvent(t, sink, telemetry.EventPlannerPlanFailed)
}

// TestPlannerTelemetryEmitPath verifies the success-path emitters produce the
// expected events with correlation stamping, independent of the LLM-backed
// graph execution.
func TestPlannerTelemetryEmitPath(t *testing.T) {
	sink := &plannerSink{}
	agent := &PlannerAgent{Config: &execution.Config{Telemetry: sink}}

	ctx := telemetry.WithRunContext(context.Background(), telemetry.RunContext{
		SessionID: "sess-1",
		RunID:     "run-1",
		TraceID:   "trace-1",
		AgentID:   "planner",
	})
	task := &execution.Task{ID: "planner-ok-1"}

	agent.planStarted(ctx, task)
	agent.planCompleted(ctx, task, true)

	requirePlannerEvent(t, sink, telemetry.EventPlannerPlanStarted)
	requirePlannerEvent(t, sink, telemetry.EventPlannerPlanCompleted)

	// Correlation must be stamped from ctx (NFR-6), not smuggled: the events
	// carry the first-class fields.
	for _, ev := range sink.events {
		if ev.SessionID != "sess-1" || ev.RunID != "run-1" || ev.TraceID != "trace-1" || ev.AgentID != "planner" {
			t.Fatalf("expected correlation on %s, got %+v", ev.Type, ev)
		}
	}
}

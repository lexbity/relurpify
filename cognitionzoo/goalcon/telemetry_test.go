package goalcon

import (
	"context"
	"errors"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type goalconSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *goalconSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *goalconSink) hasType(eventType telemetry.EventType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func (s *goalconSink) types() []telemetry.EventType {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []telemetry.EventType
	for _, ev := range s.events {
		out = append(out, ev.Type)
	}
	return out
}

func requireGoalEvent(t *testing.T, sink *goalconSink, eventType telemetry.EventType) {
	t.Helper()
	if !sink.hasType(eventType) {
		t.Fatalf("expected telemetry event %s, got %v", eventType, sink.types())
	}
}

// TestGoalConAgentEmitsParadigmTelemetry drives a real backward-chaining solve
// of a single-operator goal against a no-op plan executor and verifies the
// plan/step/execution lifecycle events flow through the configured telemetry
// sink (spec §1.7).
func TestGoalConAgentEmitsParadigmTelemetry(t *testing.T) {
	sink := &goalconSink{}
	agent := &GoalConAgent{
		Config:       &execution.Config{Telemetry: sink},
		Operators:    DefaultOperatorRegistry(),
		PlanExecutor: &noopAgent{},
		GoalOverride: &GoalCondition{Predicates: []Predicate{"file_content_known"}},
	}

	ctx := context.Background()
	task := &execution.Task{ID: "goalcon-telemetry-1"}
	res, err := agent.Execute(ctx, task, contextdata.NewEnvelope("goalcon-telemetry-1", "session"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}

	requireGoalEvent(t, sink, telemetry.EventGoalConPlanStarted)
	requireGoalEvent(t, sink, telemetry.EventGoalConPlanCompleted)
	requireGoalEvent(t, sink, telemetry.EventGoalConStepStarted)
	requireGoalEvent(t, sink, telemetry.EventGoalConStepCompleted)
	requireGoalEvent(t, sink, telemetry.EventGoalConExecutionDone)
}

// TestGoalConAgentEmitsPlanFailed covers the solve-time or execution-time
// failure branch: the plan executor never runs per-step AfterStep on failure,
// so the failure surfaces as a plan.failed event.
func TestGoalConAgentEmitsPlanFailed(t *testing.T) {
	sink := &goalconSink{}
	agent := &GoalConAgent{
		Config:       &execution.Config{Telemetry: sink},
		Operators:    DefaultOperatorRegistry(),
		PlanExecutor: &failingGoalExecutor{},
		GoalOverride: &GoalCondition{Predicates: []Predicate{"file_content_known"}},
	}

	ctx := context.Background()
	task := &execution.Task{ID: "goalcon-telemetry-fail"}
	_, err := agent.Execute(ctx, task, contextdata.NewEnvelope("goalcon-telemetry-fail", "session"))
	if err == nil {
		t.Fatal("expected execution failure")
	}
	requireGoalEvent(t, sink, telemetry.EventGoalConPlanStarted)
	requireGoalEvent(t, sink, telemetry.EventGoalConPlanCompleted)
	requireGoalEvent(t, sink, telemetry.EventGoalConPlanFailed)
}

// failingGoalExecutor returns an error for every plan step.
type failingGoalExecutor struct{}

func (f *failingGoalExecutor) Initialize(*execution.Config) error { return nil }
func (f *failingGoalExecutor) Capabilities() []string             { return nil }
func (f *failingGoalExecutor) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("fail_done")
	_ = g.AddNode(done)
	_ = g.SetStart("fail_done")
	return g, nil
}
func (f *failingGoalExecutor) Execute(context.Context, *execution.Task, *contextdata.Envelope) (*execution.Result, error) {
	return nil, errors.New("goalcon step failed")
}

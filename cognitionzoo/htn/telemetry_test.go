package htn

import (
	"context"
	"errors"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/htn/runtime"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// paradigmSink captures HTN lifecycle events for assertions.
type paradigmSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *paradigmSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *paradigmSink) hasType(eventType telemetry.EventType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func (s *paradigmSink) types() []telemetry.EventType {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []telemetry.EventType
	for _, ev := range s.events {
		out = append(out, ev.Type)
	}
	return out
}

func (s *paradigmSink) firstOfType(eventType telemetry.EventType) *telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.events {
		if s.events[i].Type == eventType {
			ev := s.events[i]
			return &ev
		}
	}
	return nil
}

func requireEvent(t *testing.T, sink *paradigmSink, eventType telemetry.EventType) {
	t.Helper()
	if !sink.hasType(eventType) {
		t.Fatalf("expected telemetry event %s, got %v", eventType, sink.types())
	}
}

// triggerCompiler satisfies contextstream.CompilerInvoker so the HTN streaming
// probe runs without a real context compiler.
type triggerCompiler struct{}

func (triggerCompiler) Compile(context.Context, contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	return &contextports.CompilationResult{StreamedRefs: []string{}}, nil
}

// TestHTNAgentEmitsParadigmTelemetry drives a real HTN decomposition against a
// no-op primitive executor and verifies the paradigm lifecycle events are
// emitted through the configured telemetry sink with correlation stamping
// (spec §1.7).
func TestHTNAgentEmitsParadigmTelemetry(t *testing.T) {
	sink := &paradigmSink{}
	agent := &HTNAgent{
		Config:        &execution.Config{Telemetry: sink},
		Methods:       runtime.NewMethodLibrary(),
		PrimitiveExec: &noopAgent{},
	}

	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(triggerCompiler{}))
	task := &execution.Task{
		ID:          "htn-telemetry-1",
		Type:        string(execution.TaskTypeCodeGeneration),
		Instruction: "implement a greeting feature",
	}
	res, err := agent.Execute(ctx, task, contextdata.NewEnvelope("htn-telemetry-1", "session"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}

	requireEvent(t, sink, telemetry.EventHTNPlanStarted)
	requireEvent(t, sink, telemetry.EventHTNStepStarted)
	requireEvent(t, sink, telemetry.EventHTNStepCompleted)
	requireEvent(t, sink, telemetry.EventHTNExecutionCompleted)
}

// TestHTNAgentEmitsStepFailed covers the failure branch: the plan executor does
// not invoke per-step AfterStep on failure, so the failure surfaces on the
// aggregate execution event.
func TestHTNAgentEmitsStepFailed(t *testing.T) {
	sink := &paradigmSink{}
	agent := &HTNAgent{
		Config:        &execution.Config{Telemetry: sink},
		Methods:       runtime.NewMethodLibrary(),
		PrimitiveExec: &failingPrimitive{},
	}
	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(triggerCompiler{}))
	task := &execution.Task{
		ID:          "htn-telemetry-fail",
		Type:        string(execution.TaskTypeCodeGeneration),
		Instruction: "implement a greeting feature",
	}

	_, err := agent.Execute(ctx, task, contextdata.NewEnvelope("htn-telemetry-fail", "session"))
	if err == nil {
		t.Fatal("expected execution failure")
	}
	requireEvent(t, sink, telemetry.EventHTNPlanStarted)
	requireEvent(t, sink, telemetry.EventHTNStepStarted)
	ev := sink.firstOfType(telemetry.EventHTNExecutionCompleted)
	if ev == nil {
		t.Fatalf("expected htn.execution.completed, got %v", sink.types())
	}
	if success, _ := ev.Metadata["success"].(bool); success {
		t.Fatalf("expected failed execution event, got success=true in %+v", ev.Metadata)
	}
}

// failingPrimitive returns an error for every step so the plan executor can
// observe a failed step.
type failingPrimitive struct{}

func (f *failingPrimitive) Initialize(*execution.Config) error { return nil }
func (f *failingPrimitive) Capabilities() []string             { return nil }
func (f *failingPrimitive) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("fail_done")
	_ = g.AddNode(done)
	_ = g.SetStart("fail_done")
	return g, nil
}
func (f *failingPrimitive) Execute(context.Context, *execution.Task, *contextdata.Envelope) (*execution.Result, error) {
	return nil, errors.New("step failed")
}

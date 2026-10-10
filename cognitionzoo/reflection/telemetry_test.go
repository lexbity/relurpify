package reflection

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type reflectionSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *reflectionSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *reflectionSink) hasType(eventType telemetry.EventType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func (s *reflectionSink) types() []telemetry.EventType {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []telemetry.EventType
	for _, ev := range s.events {
		out = append(out, ev.Type)
	}
	return out
}

func (s *reflectionSink) count(eventType telemetry.EventType) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, ev := range s.events {
		if ev.Type == eventType {
			n++
		}
	}
	return n
}

func requireReflectionEvent(t *testing.T, sink *reflectionSink, eventType telemetry.EventType) {
	t.Helper()
	if !sink.hasType(eventType) {
		t.Fatalf("expected telemetry event %s, got %v", eventType, sink.types())
	}
}

// approvingReviewer is a fake LanguageModel that approves every review.
type approvingReviewer struct{}

func (approvingReviewer) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: `{"approve": true, "issues": []}`}, nil
}
func (approvingReviewer) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (approvingReviewer) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}
func (approvingReviewer) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

// delegateAgent is a trivial graph executor that succeeds immediately.
type delegateAgent struct{}

func (delegateAgent) Initialize(*execution.Config) error { return nil }
func (delegateAgent) Capabilities() []string             { return nil }
func (delegateAgent) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	g := agentgraph.NewGraph()
	done := agentgraph.NewTerminalNode("delegate_done")
	_ = g.AddNode(done)
	_ = g.SetStart("delegate_done")
	return g, nil
}
func (delegateAgent) Execute(context.Context, *execution.Task, *contextdata.Envelope) (*execution.Result, error) {
	return &execution.Result{Success: true, Data: execution.NewToolResultPayload(map[string]any{"summary": "done"})}, nil
}

// TestReflectionAgentEmitsParadigmTelemetry drives the full review graph
// (delegate → review → decide → done) with a fake reviewer and verifies the
// reflection lifecycle events are emitted (spec §1.7).
func TestReflectionAgentEmitsParadigmTelemetry(t *testing.T) {
	sink := &reflectionSink{}
	agent := &ReflectionAgent{
		Reviewer: approvingReviewer{},
		Delegate: delegateAgent{},
		Config:   &execution.Config{Telemetry: sink},
	}

	ctx := context.Background()
	task := &execution.Task{ID: "reflection-telemetry-1", Instruction: "review the implementation"}
	res, err := agent.Execute(ctx, task, contextdata.NewEnvelope("reflection-telemetry-1", "session"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}

	requireReflectionEvent(t, sink, telemetry.EventReflectionIteration)
	requireReflectionEvent(t, sink, telemetry.EventReflectionCompleted)
}

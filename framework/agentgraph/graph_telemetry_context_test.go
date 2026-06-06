package agentgraph

import (
	"context"
	"testing"

	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type telemetryAwareTestNode struct {
	id       string
	sink     telemetry.Telemetry
	nilCheck bool
}

func (n *telemetryAwareTestNode) ID() string { return n.id }

func (n *telemetryAwareTestNode) Type() NodeType { return NodeTypeSystem }

func (n *telemetryAwareTestNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	n.sink = telemetry.TelemetryFromContext(ctx)
	n.nilCheck = n.sink == nil
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

func TestGraph_InjectsTelemetryIntoContext(t *testing.T) {
	sink := &coreTestTelemetrySink{}
	graph := NewGraph()
	graph.SetTelemetry(sink)

	node := &telemetryAwareTestNode{id: "node"}
	done := NewTerminalNode("done")
	if err := graph.AddNode(node); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := graph.AddNode(done); err != nil {
		t.Fatalf("add terminal: %v", err)
	}
	if err := graph.SetStart(node.ID()); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := graph.AddEdge(node.ID(), done.ID(), nil, false); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	if _, err := graph.Execute(context.Background(), env); err != nil {
		t.Fatalf("execute graph: %v", err)
	}

	if node.sink == nil {
		t.Fatalf("expected telemetry sink to be injected")
	}
	if node.sink != sink {
		t.Fatalf("expected same telemetry sink instance, got %#v", node.sink)
	}
	if len(sink.events) == 0 {
		t.Fatalf("expected graph telemetry to emit at least one event")
	}
}

func TestGraph_NilTelemetry_NoInjection(t *testing.T) {
	graph := NewGraph()

	node := &telemetryAwareTestNode{id: "node"}
	done := NewTerminalNode("done")
	if err := graph.AddNode(node); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := graph.AddNode(done); err != nil {
		t.Fatalf("add terminal: %v", err)
	}
	if err := graph.SetStart(node.ID()); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := graph.AddEdge(node.ID(), done.ID(), nil, false); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	if _, err := graph.Execute(context.Background(), env); err != nil {
		t.Fatalf("execute graph: %v", err)
	}

	if !node.nilCheck {
		t.Fatalf("expected telemetry lookup to return nil")
	}
}

type coreTestTelemetrySink struct {
	events []telemetry.Event
}

func (s *coreTestTelemetrySink) Emit(event telemetry.Event) {
	s.events = append(s.events, event)
}

package framework_test

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

type testNode struct {
	id   string
	kind graph.NodeType
	run  func(context.Context, *graph.Context) (*graph.Result, error)
}

// ID returns the configured node identifier for testing dispatch logic.
func (n testNode) ID() string { return n.id }

// Type reports the explicit type or defaults to a tool node for the tests.
func (n testNode) Type() graph.NodeType {
	if n.kind == "" {
		return graph.NodeTypeTool
	}
	return n.kind
}

// Execute runs the injected function or returns a successful result when none
// is provided so the graph tests can focus on traversal mechanics.
func (n testNode) Execute(ctx context.Context, state *graph.Context) (*graph.Result, error) {
	if n.run != nil {
		return n.run(ctx, state)
	}
	return &graph.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{}}, nil
}

// TestGraphExecuteLinear ensures a simple three-node graph runs to completion
// and returns the terminal node when no branches exist.
func TestGraphExecuteLinear(t *testing.T) {
	g := graph.NewGraph()
	ctx := core.NewContext()
	ctx.Set("task.id", "test")

	n1 := testNode{id: "n1"}
	n2 := testNode{id: "n2"}
	n3 := testNode{id: "n3", kind: graph.NodeTypeTerminal}

	if err := g.AddNode(n1); err != nil {
		t.Fatalf("add node n1: %v", err)
	}
	if err := g.AddNode(n2); err != nil {
		t.Fatalf("add node n2: %v", err)
	}
	if err := g.AddNode(n3); err != nil {
		t.Fatalf("add node n3: %v", err)
	}
	if err := g.SetStart("n1"); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := g.AddEdge("n1", "n2", nil, false); err != nil {
		t.Fatalf("edge n1->n2: %v", err)
	}
	if err := g.AddEdge("n2", "n3", nil, false); err != nil {
		t.Fatalf("edge n2->n3: %v", err)
	}

	result, err := g.Execute(context.Background(), ctx)
	if err != nil {
		t.Fatalf("execute graph: %v", err)
	}
	if result == nil || result.NodeID != "n3" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

// TestGraphMissingNode confirms AddEdge refuses connections to unknown nodes,
// preventing runtime panics later in execution.
func TestGraphMissingNode(t *testing.T) {
	g := graph.NewGraph()
	n1 := testNode{id: "n1"}
	n2 := testNode{id: "n2"}
	if err := g.AddNode(n1); err != nil {
		t.Fatalf("add node n1: %v", err)
	}
	if err := g.AddNode(n2); err != nil {
		t.Fatalf("add node n2: %v", err)
	}
	if err := g.SetStart("n1"); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := g.AddEdge("n1", "n2", nil, false); err != nil {
		t.Fatalf("edge n1->n2: %v", err)
	}
	if err := g.AddEdge("n2", "missing", nil, false); err == nil {
		t.Fatalf("expected error for missing node")
	}
}

// TestGraphAllowsCycles verifies the engine can handle loops as long as node
// visit counts stay under the configured limit.
func TestGraphAllowsCycles(t *testing.T) {
	g := graph.NewGraph()
	ctx := core.NewContext()
	counter := testNode{
		id: "counter",
		run: func(ctx context.Context, state *graph.Context) (*graph.Result, error) {
			val, _ := state.Get("count")
			next := 1
			if v, ok := val.(int); ok {
				next = v + 1
			}
			state.Set("count", next)
			return &graph.Result{
				NodeID:  "counter",
				Success: true,
				Data: map[string]interface{}{
					"count": next,
				},
			}, nil
		},
	}
	term := testNode{id: "done", kind: graph.NodeTypeTerminal}
	if err := g.AddNode(counter); err != nil {
		t.Fatalf("add counter: %v", err)
	}
	if err := g.AddNode(term); err != nil {
		t.Fatalf("add term: %v", err)
	}
	if err := g.SetStart("counter"); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := g.AddEdge("counter", "counter", func(result *graph.Result, state *graph.Context) bool {
		count, _ := result.Data["count"].(int)
		return count < 3
	}, false); err != nil {
		t.Fatalf("loop edge: %v", err)
	}
	if err := g.AddEdge("counter", "done", func(result *graph.Result, state *graph.Context) bool {
		count, _ := result.Data["count"].(int)
		return count >= 3
	}, false); err != nil {
		t.Fatalf("exit edge: %v", err)
	}
	result, err := g.Execute(context.Background(), ctx)
	if err != nil {
		t.Fatalf("execute graph: %v", err)
	}
	if result.NodeID != "done" {
		t.Fatalf("expected terminal node, got %s", result.NodeID)
	}
}

// TestGraphMaxNodeVisits ensures runaway cycles trigger a defensive error once
// a node exceeds its allowed visit count.
func TestGraphMaxNodeVisits(t *testing.T) {
	g := graph.NewGraph()
	g.SetMaxNodeVisits(2)
	loop := testNode{id: "loop"}
	if err := g.AddNode(loop); err != nil {
		t.Fatalf("add loop: %v", err)
	}
	if err := g.SetStart("loop"); err != nil {
		t.Fatalf("set start: %v", err)
	}
	if err := g.AddEdge("loop", "loop", nil, false); err != nil {
		t.Fatalf("add loop edge: %v", err)
	}
	_, err := g.Execute(context.Background(), core.NewContext())
	if err == nil {
		t.Fatalf("expected error due to exceeding max node visits")
	}
	if err.Error() != "potential cycle detected at node loop" {
		t.Fatalf("edge n2->n1: %v", err)
	}
}

// TestGraphNodeError validates errors returned by a node bubble up to the
// caller so orchestration layers can surface the failure.
func TestGraphNodeError(t *testing.T) {
	g := graph.NewGraph()
	errNode := testNode{
		id: "err",
		run: func(ctx context.Context, state *graph.Context) (*graph.Result, error) {
			return nil, errors.New("boom")
		},
	}
	if err := g.AddNode(errNode); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := g.SetStart("err"); err != nil {
		t.Fatalf("set start: %v", err)
	}
	_, err := g.Execute(context.Background(), core.NewContext())
	if err == nil {
		t.Fatalf("expected error from err node")
	}
}

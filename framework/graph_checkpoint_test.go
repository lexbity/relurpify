package framework_test

import (
	"context"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"testing"
)

type simpleTestNode struct {
	id string
}

func (n *simpleTestNode) ID() string           { return n.id }
func (n *simpleTestNode) Type() graph.NodeType { return graph.NodeTypeSystem }
func (n *simpleTestNode) Execute(ctx context.Context, state *graph.Context) (*graph.Result, error) {
	state.Set(n.id+".visited", true)
	return &graph.Result{NodeID: n.id, Success: true}, nil
}

func TestGraphCreateCheckpoint(t *testing.T) {
	g := graph.NewGraph()
	node := &simpleTestNode{id: "step"}
	end := graph.NewTerminalNode("done")
	if err := g.AddNode(node); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := g.AddNode(end); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := g.AddEdge(node.ID(), end.ID(), nil, false); err != nil {
		t.Fatalf("add edge: %v", err)
	}
	if err := g.SetStart(node.ID()); err != nil {
		t.Fatalf("set start: %v", err)
	}
	state := core.NewContext()
	state.Set("task.id", "task-ckpt")
	checkpoint, err := g.CreateCheckpoint("task-ckpt", end.ID(), state)
	if err != nil {
		t.Fatalf("CreateCheckpoint error: %v", err)
	}
	if checkpoint.Context == state {
		t.Fatal("expected checkpoint to clone the context")
	}
	if checkpoint.GraphHash == "" {
		t.Fatal("expected graph hash to be populated")
	}
}

func TestGraphResumeFromCheckpoint(t *testing.T) {
	g := graph.NewGraph()
	node := &simpleTestNode{id: "work"}
	done := graph.NewTerminalNode("done")
	if err := g.AddNode(node); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := g.AddNode(done); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := g.AddEdge(node.ID(), done.ID(), nil, false); err != nil {
		t.Fatalf("add edge: %v", err)
	}
	if err := g.SetStart(node.ID()); err != nil {
		t.Fatalf("set start: %v", err)
	}
	state := core.NewContext()
	state.Set("task.id", "resume-task")
	checkpoint, err := g.CreateCheckpoint("resume-task", node.ID(), state)
	if err != nil {
		t.Fatalf("CreateCheckpoint error: %v", err)
	}
	result, err := g.ResumeFromCheckpoint(context.Background(), checkpoint)
	if err != nil {
		t.Fatalf("ResumeFromCheckpoint error: %v", err)
	}
	if result == nil || result.NodeID != "done" {
		t.Fatalf("expected resume to finish at terminal node, got %+v", result)
	}
}

func TestGraphCreateCompressedCheckpoint(t *testing.T) {
	g := graph.NewGraph()
	ctx := core.NewContext()
	for i := 0; i < 6; i++ {
		ctx.AddInteraction("user", "history entry", nil)
	}
	comp := &stubCompressionStrategy{
		compressed: &core.CompressedContext{
			Summary:          "summary",
			KeyFacts:         []core.KeyFact{{Type: "decision", Content: "fact"}},
			OriginalTokens:   40,
			CompressedTokens: 10,
		},
		should: true,
	}
	llm := &stubLLM{text: "Summary: s\nKey Facts: []"}
	checkpoint, err := g.CreateCompressedCheckpoint("task", "node", ctx, llm, comp)
	if err != nil {
		t.Fatalf("CreateCompressedCheckpoint error: %v", err)
	}
	if checkpoint.CompressedContext == nil {
		t.Fatal("expected compressed context to be attached")
	}
	if history := checkpoint.Context.History(); len(history) > 5 {
		t.Fatal("expected checkpoint context history to be trimmed")
	}
}

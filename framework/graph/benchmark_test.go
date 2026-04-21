package graph

import (
	"context"
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type benchmarkNode struct {
	id        string
	writeKey  string
	writeSize int
}

func (n *benchmarkNode) ID() string     { return n.id }
func (n *benchmarkNode) Type() NodeType { return NodeTypeSystem }
func (n *benchmarkNode) Execute(_ context.Context, state *Context) (*Result, error) {
	if n.writeKey != "" {
		state.Set(n.writeKey, map[string]interface{}{
			"text":  string(make([]byte, n.writeSize)),
			"value": n.id,
		})
	}
	return &Result{NodeID: n.id, Success: true, Data: map[string]interface{}{}}, nil
}

func benchmarkGraphFixture(sharedKeys int) (*Graph, *core.ContextSnapshot) {
	g := NewGraph()
	start := &benchmarkNode{id: "start"}
	branchA := &benchmarkNode{id: "branch-a", writeKey: "branch.a", writeSize: 256}
	branchB := &benchmarkNode{id: "branch-b", writeKey: "branch.b", writeSize: 256}
	done := NewTerminalNode("done")

	if err := g.AddNode(start); err != nil {
		panic(err)
	}
	if err := g.AddNode(branchA); err != nil {
		panic(err)
	}
	if err := g.AddNode(branchB); err != nil {
		panic(err)
	}
	if err := g.AddNode(done); err != nil {
		panic(err)
	}
	if err := g.SetStart(start.ID()); err != nil {
		panic(err)
	}
	if err := g.AddEdge(start.ID(), branchA.ID(), nil, true); err != nil {
		panic(err)
	}
	if err := g.AddEdge(start.ID(), branchB.ID(), nil, true); err != nil {
		panic(err)
	}
	if err := g.AddEdge(start.ID(), done.ID(), nil, false); err != nil {
		panic(err)
	}

	state := core.NewContext()
	state.Set("task.id", "benchmark-task")
	for i := 0; i < sharedKeys; i++ {
		state.Set(fmt.Sprintf("shared.%04d", i), map[string]interface{}{
			"value": fmt.Sprintf("payload-%d", i),
			"nums":  []int{i, i + 1, i + 2},
		})
	}
	return g, state.Snapshot()
}

func BenchmarkParallelBranchExecutionSmallSharedState(b *testing.B) {
	graph, snapshot := benchmarkGraphFixture(24)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		state := core.NewContextFromSnapshot(snapshot, nil)
		b.StartTimer()
		if _, err := graph.Execute(context.Background(), state); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelBranchExecutionLargeSharedState(b *testing.B) {
	graph, snapshot := benchmarkGraphFixture(512)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		state := core.NewContextFromSnapshot(snapshot, nil)
		b.StartTimer()
		if _, err := graph.Execute(context.Background(), state); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParallelBranchMergeConflictDetection(b *testing.B) {
	g := NewGraph()
	start := &benchmarkNode{id: "start"}
	branchA := &benchmarkNode{id: "branch-a", writeKey: "conflict", writeSize: 128}
	branchB := &benchmarkNode{id: "branch-b", writeKey: "conflict", writeSize: 256}
	done := NewTerminalNode("done")

	for _, node := range []Node{start, branchA, branchB, done} {
		if err := g.AddNode(node); err != nil {
			b.Fatal(err)
		}
	}
	if err := g.SetStart(start.ID()); err != nil {
		b.Fatal(err)
	}
	if err := g.AddEdge(start.ID(), branchA.ID(), nil, true); err != nil {
		b.Fatal(err)
	}
	if err := g.AddEdge(start.ID(), branchB.ID(), nil, true); err != nil {
		b.Fatal(err)
	}
	if err := g.AddEdge(start.ID(), done.ID(), nil, false); err != nil {
		b.Fatal(err)
	}

	state := core.NewContext()
	state.Set("task.id", "benchmark-task")
	snapshot := state.Snapshot()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runState := core.NewContextFromSnapshot(snapshot, nil)
		if _, err := g.Execute(context.Background(), runState); err == nil {
			b.Fatal("expected merge conflict")
		}
	}
}

func benchmarkMemoryPublicationEnvelopes(n int) []core.MemoryRecordEnvelope {
	out := make([]core.MemoryRecordEnvelope, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, core.MemoryRecordEnvelope{
			Key:         fmt.Sprintf("doc:%03d", i),
			RecordID:    fmt.Sprintf("doc:%03d", i),
			MemoryClass: core.MemoryClassDeclarative,
			Scope:       "project",
			Summary:     fmt.Sprintf("retrieved memory summary %03d", i),
			Text:        fmt.Sprintf("retrieved memory text %03d", i),
			Source:      "retrieval",
			Kind:        "document",
			Score:       1.0 - float64(i)/1000.0,
			Reference: map[string]any{
				"kind": string(core.ContextReferenceRetrievalEvidence),
				"uri":  fmt.Sprintf("memory://runtime/doc:%03d", i),
			},
		})
	}
	return out
}

func BenchmarkBuildMemoryRetrievalPublication(b *testing.B) {
	envelopes := benchmarkMemoryPublicationEnvelopes(24)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		publication := BuildMemoryRetrievalPublication("find memory", envelopes, core.MemoryClassDeclarative)
		if publication == nil || len(publication.Results) != len(envelopes) {
			b.Fatal("expected publication results")
		}
	}
}

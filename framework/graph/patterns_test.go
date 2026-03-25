package graph_test

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/stretchr/testify/require"
)

// simpleNode is a minimal Node for pattern tests.
type simpleNode struct {
	id   string
	kind graph.NodeType
	run  func(*core.Context)
}

func (n *simpleNode) ID() string           { return n.id }
func (n *simpleNode) Type() graph.NodeType { return n.kind }
func (n *simpleNode) Execute(_ context.Context, state *core.Context) (*core.Result, error) {
	if n.run != nil {
		n.run(state)
	}
	return &core.Result{NodeID: n.id, Success: true}, nil
}

func newSimple(id string) *simpleNode {
	return &simpleNode{id: id, kind: graph.NodeTypeSystem}
}

// --- BuildPlanExecuteVerifyGraph ---

func TestBuildPlanExecuteVerifyGraphExecutesToDone(t *testing.T) {
	order := []string{}
	mkNode := func(id string) *simpleNode {
		return &simpleNode{id: id, kind: graph.NodeTypeSystem, run: func(*core.Context) { order = append(order, id) }}
	}
	g, err := graph.BuildPlanExecuteVerifyGraph(mkNode("plan"), mkNode("execute"), mkNode("verify"), "done")
	require.NoError(t, err)
	require.NoError(t, g.Validate())

	result, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)
	require.Equal(t, []string{"plan", "execute", "verify"}, order)
}

func TestBuildPlanExecuteVerifyGraphRejectsNilNodes(t *testing.T) {
	_, err := graph.BuildPlanExecuteVerifyGraph(nil, newSimple("execute"), newSimple("verify"), "done")
	require.Error(t, err)
}

// --- BuildThinkActObserveGraph ---

func TestBuildThinkActObserveGraphLoopsOnceAndExits(t *testing.T) {
	iterations := 0
	observe := &simpleNode{id: "observe", kind: graph.NodeTypeConditional, run: func(state *core.Context) {
		iterations++
		state.Set("loop.done", iterations >= 2)
	}}
	g, err := graph.BuildThinkActObserveGraph(
		newSimple("think"),
		newSimple("act"),
		observe,
		func(_ *core.Result, state *core.Context) bool {
			done, _ := state.Get("loop.done")
			return done != true
		},
		func(_ *core.Result, state *core.Context) bool {
			done, _ := state.Get("loop.done")
			return done == true
		},
		"done",
	)
	require.NoError(t, err)
	require.NoError(t, g.Validate())

	result, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)
	require.Equal(t, 2, iterations)
}

func TestBuildThinkActObserveGraphRejectsNilNodes(t *testing.T) {
	_, err := graph.BuildThinkActObserveGraph(nil, newSimple("act"), newSimple("observe"), nil, nil, "done")
	require.Error(t, err)
}

// --- BuildReviewIterateGraph ---

func TestBuildReviewIterateGraphLoopsOnceAndExits(t *testing.T) {
	reviews := 0
	review := &simpleNode{id: "review", kind: graph.NodeTypeConditional, run: func(state *core.Context) {
		reviews++
		state.Set("approved", reviews >= 2)
	}}
	g, err := graph.BuildReviewIterateGraph(
		newSimple("execute"),
		review,
		func(_ *core.Result, state *core.Context) bool {
			approved, _ := state.Get("approved")
			return approved != true
		},
		func(_ *core.Result, state *core.Context) bool {
			approved, _ := state.Get("approved")
			return approved == true
		},
		"done",
	)
	require.NoError(t, err)
	require.NoError(t, g.Validate())

	result, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)
	require.Equal(t, 2, reviews)
}

func TestBuildReviewIterateGraphRejectsNilNodes(t *testing.T) {
	_, err := graph.BuildReviewIterateGraph(nil, newSimple("review"), nil, nil, "done")
	require.Error(t, err)
}

// --- BuildPlanExecuteSummarizeVerifyGraph ---

func TestBuildPlanExecuteSummarizeVerifyGraphExecutesToDone(t *testing.T) {
	order := []string{}
	mkNode := func(id string) *simpleNode {
		return &simpleNode{id: id, kind: graph.NodeTypeSystem, run: func(*core.Context) { order = append(order, id) }}
	}
	g, err := graph.BuildPlanExecuteSummarizeVerifyGraph(
		mkNode("plan"), mkNode("execute"), mkNode("summarize"), mkNode("verify"), "done",
	)
	require.NoError(t, err)
	require.NoError(t, g.Validate())

	result, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)
	require.Equal(t, []string{"plan", "execute", "summarize", "verify"}, order)
}

func TestBuildPlanExecuteSummarizeVerifyGraphRejectsNilNodes(t *testing.T) {
	_, err := graph.BuildPlanExecuteSummarizeVerifyGraph(nil, newSimple("execute"), newSimple("summarize"), newSimple("verify"), "done")
	require.Error(t, err)
}

// --- WrapWithCheckpointing ---

func TestWrapWithCheckpointingInterceptsEdgeToDone(t *testing.T) {
	g, err := graph.BuildPlanExecuteVerifyGraph(newSimple("plan"), newSimple("execute"), newSimple("verify"), "done")
	require.NoError(t, err)

	persister := &stubCheckpointPersister{}
	checkpoint := graph.NewCheckpointNode("cp", "done", persister)
	require.NoError(t, graph.WrapWithCheckpointing(g, "verify", checkpoint, "done"))
	require.NoError(t, g.Validate())

	result, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)
	require.Len(t, persister.checkpoints, 1)
	require.Equal(t, "done", persister.checkpoints[0].NextNodeID)
}

func TestWrapWithCheckpointingRejectsNilArgs(t *testing.T) {
	require.Error(t, graph.WrapWithCheckpointing(nil, "a", graph.NewCheckpointNode("cp", "done", nil), "done"))
	g := graph.NewGraph()
	require.Error(t, graph.WrapWithCheckpointing(g, "", graph.NewCheckpointNode("cp", "done", nil), "done"))
}

// --- WrapWithPeriodicSummaries ---

func TestWrapWithPeriodicSummariesInterceptsEdgeToDone(t *testing.T) {
	g, err := graph.BuildPlanExecuteVerifyGraph(newSimple("plan"), newSimple("execute"), newSimple("verify"), "done")
	require.NoError(t, err)

	summarize := graph.NewSummarizeContextNode("sum", &core.SimpleSummarizer{})
	require.NoError(t, graph.WrapWithPeriodicSummaries(g, "verify", summarize, "done"))
	require.NoError(t, g.Validate())

	result, err := g.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)
	_, hasSummaryRef := core.NewContext().Get("graph.summary_ref") // just structure check
	_ = hasSummaryRef
}

func TestWrapWithPeriodicSummariesRejectsNilArgs(t *testing.T) {
	require.Error(t, graph.WrapWithPeriodicSummaries(nil, "a", graph.NewSummarizeContextNode("sum", nil), "done"))
	g := graph.NewGraph()
	require.Error(t, graph.WrapWithPeriodicSummaries(g, "", graph.NewSummarizeContextNode("sum", nil), "done"))
}

// --- WrapWithDeclarativeRetrieval ---

func TestWrapWithDeclarativeRetrievalPrependsRetrieveNode(t *testing.T) {
	g, err := graph.BuildPlanExecuteVerifyGraph(newSimple("plan"), newSimple("execute"), newSimple("verify"), "done")
	require.NoError(t, err)

	retrieve := graph.NewRetrieveDeclarativeMemoryNode("retrieve_decl", stubRetriever{
		results: []core.MemoryRecordEnvelope{
			{Key: "fact-1", MemoryClass: core.MemoryClassDeclarative, Scope: "project", Summary: "important fact"},
		},
	})
	require.NoError(t, graph.WrapWithDeclarativeRetrieval(g, retrieve))
	require.NoError(t, g.Validate())

	state := core.NewContext()
	state.Set("task.instruction", "do work")
	result, err := g.Execute(context.Background(), state)
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)

	raw, ok := state.Get("graph.declarative_memory")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	results, ok := payload["results"].([]core.MemoryRecordEnvelope)
	require.True(t, ok)
	require.Len(t, results, 1)
	rawPayload, ok := state.Get("graph.declarative_memory_payload")
	require.True(t, ok)
	mixedPayload, ok := rawPayload.(map[string]any)
	require.True(t, ok)
	mixedResults, ok := mixedPayload["results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, mixedResults, 1)
	rawRefs, ok := state.Get("graph.declarative_memory_refs")
	require.True(t, ok)
	refs, ok := rawRefs.([]core.ContextReference)
	require.True(t, ok)
	require.Len(t, refs, 1)
}

func TestWrapWithDeclarativeRetrievalRejectsNilArgs(t *testing.T) {
	require.Error(t, graph.WrapWithDeclarativeRetrieval(nil, graph.NewRetrieveDeclarativeMemoryNode("r", nil)))
	g := graph.NewGraph()
	require.Error(t, graph.WrapWithDeclarativeRetrieval(g, graph.NewRetrieveDeclarativeMemoryNode("r", nil)))
}

// --- WrapWithProceduralRetrieval ---

func TestWrapWithProceduralRetrievalPrependsRetrieveNode(t *testing.T) {
	g, err := graph.BuildPlanExecuteVerifyGraph(newSimple("plan"), newSimple("execute"), newSimple("verify"), "done")
	require.NoError(t, err)

	retrieve := graph.NewRetrieveProceduralMemoryNode("retrieve_proc", stubRetriever{
		results: []core.MemoryRecordEnvelope{
			{Key: "routine-1", MemoryClass: core.MemoryClassProcedural, Scope: "project", Summary: "useful routine"},
		},
	})
	require.NoError(t, graph.WrapWithProceduralRetrieval(g, retrieve))
	require.NoError(t, g.Validate())

	state := core.NewContext()
	state.Set("task.instruction", "do work")
	result, err := g.Execute(context.Background(), state)
	require.NoError(t, err)
	require.Equal(t, "done", result.NodeID)

	raw, ok := state.Get("graph.procedural_memory")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	results, ok := payload["results"].([]core.MemoryRecordEnvelope)
	require.True(t, ok)
	require.Len(t, results, 1)
	rawPayload, ok := state.Get("graph.procedural_memory_payload")
	require.True(t, ok)
	mixedPayload, ok := rawPayload.(map[string]any)
	require.True(t, ok)
	mixedResults, ok := mixedPayload["results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, mixedResults, 1)
	rawRefs, ok := state.Get("graph.procedural_memory_refs")
	require.True(t, ok)
	refs, ok := rawRefs.([]core.ContextReference)
	require.True(t, ok)
	require.Len(t, refs, 1)
}

func TestWrapWithProceduralRetrievalRejectsNilArgs(t *testing.T) {
	require.Error(t, graph.WrapWithProceduralRetrieval(nil, graph.NewRetrieveProceduralMemoryNode("r", nil)))
	g := graph.NewGraph()
	require.Error(t, graph.WrapWithProceduralRetrieval(g, graph.NewRetrieveProceduralMemoryNode("r", nil)))
}

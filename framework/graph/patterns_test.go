package graph_test

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
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


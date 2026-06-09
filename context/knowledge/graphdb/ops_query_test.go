package graphdb

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestImpactSet_EmptyOrigin(t *testing.T) {
	engine, _ := newTestEngine(t)
	result := engine.ImpactSet([]string{}, []EdgeKind{"calls"}, 2)
	require.Empty(t, result.Affected)
	require.Equal(t, []string{}, result.OriginIDs)
	require.Empty(t, result.ByDepth)
}

func TestImpactSet_MaxDepthZero(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))

	result := engine.ImpactSet([]string{"a"}, []EdgeKind{"calls"}, 0)
	require.ElementsMatch(t, []string{"a"}, result.ByDepth[0])
	require.Empty(t, result.Affected)
}

func TestImpactSet_EdgeKindFiltering(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("b", "c", "imports", "", 1, nil))

	// only follow "calls" edges
	result := engine.ImpactSet([]string{"a"}, []EdgeKind{"calls"}, 2)
	require.ElementsMatch(t, []string{"b"}, result.Affected)
	require.NotContains(t, result.Affected, "c")
}

func TestImpactSet_MultipleOrigins(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "c", "calls", "", 1, nil))
	require.NoError(t, engine.Link("b", "d", "calls", "", 1, nil))

	result := engine.ImpactSet([]string{"a", "b"}, []EdgeKind{"calls"}, 1)
	require.ElementsMatch(t, []string{"c", "d"}, result.Affected)
	require.Len(t, result.ByDepth[1], 2)
}

func TestFindPath_NoPathDueToKind(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "src", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "dst", Kind: "function"}))
	require.NoError(t, engine.Link("src", "dst", "imports", "", 1, nil))

	path, err := engine.FindPath("src", "dst", []EdgeKind{"calls"}, 2)
	require.NoError(t, err)
	require.Nil(t, path)
}

func TestFindPath_SelfPath(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "x", Kind: "function"}))

	path, err := engine.FindPath("x", "x", []EdgeKind{"calls"}, 5)
	require.NoError(t, err)
	require.NotNil(t, path)
	require.Equal(t, []string{"x"}, path.Path)
	require.Empty(t, path.Edges)
}

func TestFindPath_BidirectionalMeet(t *testing.T) {
	engine, _ := newTestEngine(t)
	// linear chain a->b->c->d
	for _, id := range []string{"a", "b", "c", "d"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("b", "c", "calls", "", 1, nil))
	require.NoError(t, engine.Link("c", "d", "calls", "", 1, nil))

	path, err := engine.FindPath("a", "d", []EdgeKind{"calls"}, 10)
	require.NoError(t, err)
	require.NotNil(t, path)
	require.Equal(t, []string{"a", "b", "c", "d"}, path.Path)
	require.Len(t, path.Edges, 3)
}

func TestNeighbors_DirectionIn(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("c", "b", "calls", "", 1, nil))

	neighbors := engine.Neighbors("b", DirectionIn, "calls")
	require.ElementsMatch(t, []string{"a", "c"}, neighbors)
}

func TestNeighbors_EmptyKinds(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("a", "b", "imports", "", 1, nil))

	neighbors := engine.Neighbors("a", DirectionOut)
	require.ElementsMatch(t, []string{"b"}, neighbors) // both edges go to same target
}

func TestSubgraph_DepthZero(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "root", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "other", Kind: "function"}))
	require.NoError(t, engine.Link("root", "other", "calls", "", 1, nil))

	nodes, edges := engine.Subgraph(GraphQuery{
		RootIDs:   []string{"root"},
		Direction: DirectionOut,
		MaxDepth:  0,
	})
	require.Len(t, nodes, 1)
	require.Equal(t, "root", nodes[0].ID)
	require.Empty(t, edges)
}

func TestSubgraph_DirectionBoth(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("c", "b", "calls", "", 1, nil))

	nodes, edges := engine.Subgraph(GraphQuery{
		RootIDs:   []string{"b"},
		Direction: DirectionBoth,
		MaxDepth:  1,
		EdgeKinds: []EdgeKind{"calls"},
	})
	require.Len(t, nodes, 3)
	require.Len(t, edges, 2)
}

// --- Bounded API tests ---

func TestImpactSetContext_StopsAtLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 3, 3)

	result, err := engine.ImpactSetContext(context.Background(), []string{"root"}, []EdgeKind{"calls"}, 10, 5)
	require.ErrorIs(t, err, ErrQueryLimitExceeded)
	require.Len(t, result.Affected, 5)

	// with a higher limit we get more results
	result2, err := engine.ImpactSetContext(context.Background(), []string{"root"}, []EdgeKind{"calls"}, 10, 50)
	require.NoError(t, err)
	require.Greater(t, len(result2.Affected), 5)
}

func TestImpactSetContext_LimitValidation(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, err := engine.ImpactSetContext(context.Background(), []string{"a"}, nil, 1, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "limit must be > 0")

	_, err = engine.ImpactSetContext(context.Background(), []string{"a"}, nil, -1, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maxDepth must be >= 0")
}

func TestImpactSetContext_Cancellation(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 5, 5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := engine.ImpactSetContext(ctx, []string{"root"}, []EdgeKind{"calls"}, 10, 1000)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestImpactSetContext_EmptyOrigin(t *testing.T) {
	engine, _ := newTestEngine(t)
	result, err := engine.ImpactSetContext(context.Background(), []string{}, []EdgeKind{"calls"}, 2, 10)
	require.NoError(t, err)
	require.Empty(t, result.Affected)
}

func TestImpactSetContext_NoErrorWhenWithinLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))

	result, err := engine.ImpactSetContext(context.Background(), []string{"a"}, []EdgeKind{"calls"}, 2, 100)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"b"}, result.Affected)
}

func TestSubgraphContext_StopsAtMaxEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 2, 4)

	nodes, edges, _, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:   []string{"root"},
		Direction: DirectionOut,
		MaxDepth:  10,
		Limit:     100,
		MaxEdges:  3,
	})
	require.ErrorIs(t, err, ErrQueryLimitExceeded)
	require.Len(t, edges, 3)
	// nodes should not include everything
	require.LessOrEqual(t, len(nodes), 100)
}

func TestSubgraphContext_StopsAtLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 2, 4)

	nodes, edges, _, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:   []string{"root"},
		Direction: DirectionOut,
		MaxDepth:  10,
		Limit:     5,
	})
	require.ErrorIs(t, err, ErrQueryLimitExceeded)
	require.Len(t, nodes, 5)
	// edges may be partial too
	require.NotEmpty(t, edges)
}

func TestSubgraphContext_LimitValidation(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, _, _, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:  []string{"a"},
		MaxDepth: 1,
		Limit:    0,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Limit must be > 0")

	_, _, _, err = engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:  []string{"a"},
		MaxDepth: -1,
		Limit:    10,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MaxDepth must be >= 0")
}

func TestSubgraphContext_Cancellation(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 4, 6)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, _, err := engine.SubgraphContext(ctx, GraphQuery{
		RootIDs:   []string{"root"},
		Direction: DirectionOut,
		MaxDepth:  10,
		Limit:     1000,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestSubgraphContext_IncludePropsFalse(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{
		ID:    "a",
		Kind:  "function",
		Props: []byte(`{"key":"val"}`),
	}))
	require.NoError(t, engine.UpsertNode(NodeRecord{
		ID:    "b",
		Kind:  "function",
		Props: []byte(`{"other":1}`),
	}))
	require.NoError(t, engine.LinkEdges([]EdgeRecord{
		{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1, Props: []byte(`{"w":1}`)},
	}))

	nodes, edges, _, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:      []string{"a"},
		Direction:    DirectionOut,
		MaxDepth:     1,
		Limit:        100,
		IncludeProps: false,
	})
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	for _, n := range nodes {
		require.Nil(t, n.Props, "IncludeProps=false should strip Props")
	}
	require.Len(t, edges, 1)
	require.Nil(t, edges[0].Props, "IncludeProps=false should strip edge Props")
}

func TestSubgraphContext_IncludePropsTrue(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{
		ID:    "a",
		Kind:  "function",
		Props: []byte(`{"key":"val"}`),
	}))
	require.NoError(t, engine.UpsertNode(NodeRecord{
		ID:    "b",
		Kind:  "function",
		Props: []byte(`{"other":1}`),
	}))
	require.NoError(t, engine.LinkEdges([]EdgeRecord{
		{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1, Props: []byte(`{"w":1}`)},
	}))

	nodes, edges, _, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:      []string{"a"},
		Direction:    DirectionOut,
		MaxDepth:     1,
		Limit:        100,
		IncludeProps: true,
	})
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	for _, n := range nodes {
		require.NotNil(t, n.Props, "IncludeProps=true should preserve Props")
	}
	require.NotNil(t, edges[0].Props)
}

func TestSubgraphContext_CursorReturnedEmpty(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))

	nodes, edges, cursor, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:   []string{"a"},
		Direction: DirectionOut,
		MaxDepth:  1,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Empty(t, cursor)
	require.Len(t, nodes, 1)
	require.Empty(t, edges)
}

func TestSubgraphContext_DefaultMaxEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	// MaxEdges defaults to Limit*4 = 40
	nodes, edges, _, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:   []string{"a"},
		Direction: DirectionOut,
		MaxDepth:  0,
		Limit:     10,
	})
	require.NoError(t, err)
	_ = nodes
	_ = edges
}

func TestFindPathContext_Cancellation(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 6, 6)
	target := "root-0-5-1-5-2-5-3-5-4-5-5-5"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := engine.FindPathContext(ctx, "root", target, []EdgeKind{"calls"}, 10)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestFindPathContext_NoPath(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "s", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "t", Kind: "function"}))
	require.NoError(t, engine.Link("s", "t", "imports", "", 1, nil))

	path, err := engine.FindPathContext(context.Background(), "s", "t", []EdgeKind{"calls"}, 5)
	require.NoError(t, err)
	require.Nil(t, path)
}

func TestFindPathContext_SelfPath(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "x", Kind: "function"}))

	path, err := engine.FindPathContext(context.Background(), "x", "x", nil, 5)
	require.NoError(t, err)
	require.NotNil(t, path)
	require.Equal(t, []string{"x"}, path.Path)
}

func TestFindPathContext_Validation(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, err := engine.FindPathContext(context.Background(), "", "t", nil, 5)
	require.Error(t, err)

	_, err = engine.FindPathContext(context.Background(), "s", "t", nil, -1)
	require.Error(t, err)
}

func TestLegacyWrappersPreserveBehavior(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("b", "c", "calls", "", 1, nil))

	// ImpactSet legacy wrapper
	result := engine.ImpactSet([]string{"a"}, []EdgeKind{"calls"}, 2)
	require.ElementsMatch(t, []string{"b", "c"}, result.Affected)

	// FindPath legacy wrapper
	path, err := engine.FindPath("a", "c", []EdgeKind{"calls"}, 5)
	require.NoError(t, err)
	require.NotNil(t, path)
	require.Equal(t, []string{"a", "b", "c"}, path.Path)

	// Subgraph legacy wrapper
	nodes, edges := engine.Subgraph(GraphQuery{
		RootIDs:   []string{"a"},
		Direction: DirectionOut,
		MaxDepth:  2,
	})
	require.Len(t, nodes, 3)
	require.Len(t, edges, 2)

	// Subgraph with Limit zero (legacy callers that don't set it)
	nodes2, edges2 := engine.Subgraph(GraphQuery{
		RootIDs:   []string{"a"},
		Direction: DirectionOut,
		MaxDepth:  2,
	})
	require.Len(t, nodes2, 3)
	require.Len(t, edges2, 2)
}

func TestDefaultMaxEdges(t *testing.T) {
	require.Equal(t, 40, defaultMaxEdges(10))
	require.Equal(t, 64000, defaultMaxEdges(0))
	require.Equal(t, 64000, defaultMaxEdges(-1))
	require.Equal(t, 64000, defaultMaxEdges(16000))
	require.Equal(t, 4, defaultMaxEdges(1))
}

func TestImpactSetContext_LargeLimitDoesNotError(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))

	result, err := engine.ImpactSetContext(context.Background(), []string{"a"}, nil, 1, math.MaxInt32)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"b"}, result.Affected)
}

func TestSubgraphContext_MaxEdgesDefault(t *testing.T) {
	engine, _ := newTestEngine(t)
	for range 10 {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: t.Name() + "a", Kind: "function"}))
	}

	nodes, edges, _, err := engine.SubgraphContext(context.Background(), GraphQuery{
		RootIDs:   []string{"a"},
		Direction: DirectionOut,
		MaxDepth:  0,
		Limit:     100,
	})
	require.NoError(t, err)
	_ = nodes
	_ = edges
}

func TestSubgraphContext_ContextTimeout(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 4, 6)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond)

	_, _, _, err := engine.SubgraphContext(ctx, GraphQuery{
		RootIDs:   []string{"root"},
		Direction: DirectionOut,
		MaxDepth:  10,
		Limit:     10000,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}

func TestImpactSetContext_NegativeMaxDepth(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, err := engine.ImpactSetContext(context.Background(), []string{"a"}, nil, -5, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maxDepth must be >= 0")
}

func TestImpactSetContext_OnlyOriginWithinLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))

	result, err := engine.ImpactSetContext(context.Background(), []string{"a"}, nil, 5, 1)
	require.NoError(t, err)
	require.Empty(t, result.Affected)
	require.Contains(t, result.ByDepth[0], "a")
}

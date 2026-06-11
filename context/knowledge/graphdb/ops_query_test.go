package graphdb

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ────────────────────────────────────────────────────────────────────
// ImpactSet tests (converted to SubgraphPage)
// ────────────────────────────────────────────────────────────────────

func collectNodeIDs(page Page[GraphElement]) []string {
	var ids []string
	for _, elem := range page.Items {
		if elem.Node.ID != "" {
			ids = append(ids, elem.Node.ID)
		}
	}
	return ids
}

func collectEdgeRecords(page Page[GraphElement]) []EdgeRecord {
	var edges []EdgeRecord
	for _, elem := range page.Items {
		if elem.Edge.SourceID != "" {
			edges = append(edges, elem.Edge)
		}
	}
	return edges
}

func collectNodeRecords(page Page[GraphElement]) []NodeRecord {
	var nodes []NodeRecord
	for _, elem := range page.Items {
		if elem.Node.ID != "" {
			nodes = append(nodes, elem.Node)
		}
	}
	return nodes
}

func TestImpactSet_EmptyOrigin(t *testing.T) {
	engine, _ := newTestEngine(t)
	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  2,
			Direction: DirectionOut,
			Limit:     10000,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

func TestImpactSet_MaxDepthZero(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  0,
			Direction: DirectionOut,
			Limit:     10000,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodeIDs := collectNodeIDs(page)
	require.ElementsMatch(t, []string{"a"}, nodeIDs)
}

func TestImpactSet_EdgeKindFiltering(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "b", "c", "imports", "", 1, nil))

	// only follow "calls" edges
	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  2,
			Direction: DirectionOut,
			Limit:     10000,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodeIDs := collectNodeIDs(page)
	require.Contains(t, nodeIDs, "a")
	require.Contains(t, nodeIDs, "b")
	require.NotContains(t, nodeIDs, "c")
}

func TestImpactSet_MultipleOrigins(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link(context.TODO(), "a", "c", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "b", "d", "calls", "", 1, nil))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a", "b"},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  1,
			Direction: DirectionOut,
			Limit:     10000,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodeIDs := collectNodeIDs(page)
	require.ElementsMatch(t, []string{"a", "b", "c", "d"}, nodeIDs)
}

// ────────────────────────────────────────────────────────────────────
// Neighbors tests (unchanged logic, uses Neighbors which still exists)
// ────────────────────────────────────────────────────────────────────

func TestNeighbors_DirectionIn(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "c", "b", "calls", "", 1, nil))

	neighbors := engine.Neighbors("b", DirectionIn, "calls")
	require.ElementsMatch(t, []string{"a", "c"}, neighbors)
}

func TestNeighbors_EmptyKinds(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "imports", "", 1, nil))

	neighbors := engine.Neighbors("a", DirectionOut)
	require.ElementsMatch(t, []string{"b"}, neighbors) // both edges go to same target
}

// ────────────────────────────────────────────────────────────────────
// Subgraph tests (converted to SubgraphPage)
// ────────────────────────────────────────────────────────────────────

func TestSubgraph_DepthZero(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "root", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "other", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "root", "other", "calls", "", 1, nil))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			Direction: DirectionOut,
			MaxDepth:  0,
			Limit:     10000,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodes := collectNodeRecords(page)
	edges := collectEdgeRecords(page)
	require.Len(t, nodes, 1)
	require.Equal(t, "root", nodes[0].ID)
	require.Empty(t, edges)
}

func TestSubgraph_DirectionBoth(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "c", "b", "calls", "", 1, nil))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"b"},
			Direction: DirectionBoth,
			MaxDepth:  1,
			EdgeKinds: []EdgeKind{"calls"},
			Limit:     10000,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodes := collectNodeRecords(page)
	edges := collectEdgeRecords(page)
	require.Len(t, nodes, 3)
	require.Len(t, edges, 2)
}

// ────────────────────────────────────────────────────────────────────
// ImpactSet bounded API (via SubgraphPage)
// ────────────────────────────────────────────────────────────────────

func TestImpactSetContext_StopsAtLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 3, 3)

	// With a small limit, the page stops early and has a Next token
	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  10,
			Direction: DirectionOut,
			Limit:     5,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 5)
	require.NotEmpty(t, page.Next, "should have more results")

	// with a higher limit we get more results
	page2, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  10,
			Direction: DirectionOut,
			Limit:     50,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	require.Greater(t, len(page2.Items), 5, "higher limit should return more elements")
}

func TestImpactSetContext_LimitValidation(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			MaxDepth:  1,
			Direction: DirectionOut,
			Limit:     0,
		},
		PageSize: 10000,
	})
	// Limit:0 is no longer an error (defaults to 100)
	require.NoError(t, err)

	_, err = engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			MaxDepth:  -1,
			Direction: DirectionOut,
			Limit:     10,
		},
		PageSize: 10000,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MaxDepth must be >= 0")
}

func TestImpactSetContext_Cancellation(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 5, 5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := engine.SubgraphPage(ctx, GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  10,
			Direction: DirectionOut,
			Limit:     1000,
		},
		PageSize: 10000,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestImpactSetContext_EmptyOrigin(t *testing.T) {
	engine, _ := newTestEngine(t)
	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  2,
			Direction: DirectionOut,
			Limit:     10,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

func TestImpactSetContext_NoErrorWhenWithinLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			EdgeKinds: []EdgeKind{"calls"},
			MaxDepth:  2,
			Direction: DirectionOut,
			Limit:     100,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodeIDs := collectNodeIDs(page)
	require.Contains(t, nodeIDs, "b")
}

// ────────────────────────────────────────────────────────────────────
// Subgraph bounded API (converted to SubgraphPage)
// ────────────────────────────────────────────────────────────────────

func TestSubgraphContext_StopsAtMaxEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 2, 4)

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			Direction: DirectionOut,
			MaxDepth:  10,
			Limit:     100,
			MaxEdges:  3,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	edges := collectEdgeRecords(page)
	require.Len(t, edges, 3)
	require.NotEmpty(t, page.Next, "should have more results")
	nodes := collectNodeRecords(page)
	require.LessOrEqual(t, len(nodes), 100)
}

func TestSubgraphContext_StopsAtLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 2, 4)

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			Direction: DirectionOut,
			MaxDepth:  10,
			Limit:     5,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 5)
	require.NotEmpty(t, page.Next, "should have more results")
	var edges []EdgeRecord
	for _, elem := range page.Items {
		if elem.Edge.SourceID != "" {
			edges = append(edges, elem.Edge)
		}
	}
	require.NotEmpty(t, edges)
}

func TestSubgraphContext_Validation(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:  []string{"a"},
			MaxDepth: 1,
			Limit:    0,
		},
		PageSize: 10000,
	})
	// Limit:0 is now fine (defaults to 100), but MaxDepth: -1 should still error
	require.NoError(t, err)

	_, err = engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:  []string{"a"},
			MaxDepth: -1,
			Limit:    10,
		},
		PageSize: 10000,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MaxDepth must be >= 0")
}

func TestSubgraphContext_Cancellation(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 4, 6)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := engine.SubgraphPage(ctx, GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			Direction: DirectionOut,
			MaxDepth:  10,
			Limit:     1000,
		},
		PageSize: 10000,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSubgraphContext_IncludePropsFalse(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{
		ID:    "a",
		Kind:  "function",
		Props: []byte(`{"key":"val"}`),
	}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{
		ID:    "b",
		Kind:  "function",
		Props: []byte(`{"other":1}`),
	}))
	require.NoError(t, engine.LinkEdges(context.TODO(), []EdgeRecord{
		{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1, Props: []byte(`{"w":1}`)},
	}))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:      []string{"a"},
			Direction:    DirectionOut,
			MaxDepth:     1,
			Limit:        100,
			IncludeProps: false,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodes := collectNodeRecords(page)
	edges := collectEdgeRecords(page)
	require.Len(t, nodes, 2)
	for _, n := range nodes {
		require.Nil(t, n.Props, "IncludeProps=false should strip Props")
	}
	require.Len(t, edges, 1)
	require.Nil(t, edges[0].Props, "IncludeProps=false should strip edge Props")
}

func TestSubgraphContext_IncludePropsTrue(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{
		ID:    "a",
		Kind:  "function",
		Props: []byte(`{"key":"val"}`),
	}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{
		ID:    "b",
		Kind:  "function",
		Props: []byte(`{"other":1}`),
	}))
	require.NoError(t, engine.LinkEdges(context.TODO(), []EdgeRecord{
		{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1, Props: []byte(`{"w":1}`)},
	}))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:      []string{"a"},
			Direction:    DirectionOut,
			MaxDepth:     1,
			Limit:        100,
			IncludeProps: true,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodes := collectNodeRecords(page)
	edges := collectEdgeRecords(page)
	require.Len(t, nodes, 2)
	for _, n := range nodes {
		require.NotNil(t, n.Props, "IncludeProps=true should preserve Props")
	}
	require.NotNil(t, edges[0].Props)
}

func TestSubgraphContext_CursorReturnedEmpty(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			Direction: DirectionOut,
			MaxDepth:  1,
			Limit:     100,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	require.Empty(t, page.Next)
	nodes := collectNodeRecords(page)
	require.Len(t, nodes, 1)
}

func TestSubgraphContext_DefaultMaxEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			Direction: DirectionOut,
			MaxDepth:  0,
			Limit:     10,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	_ = page
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
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			MaxDepth:  1,
			Direction: DirectionOut,
			Limit:     math.MaxInt32,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodeIDs := collectNodeIDs(page)
	require.Contains(t, nodeIDs, "b")
}

func TestSubgraphContext_MaxEdgesDefault(t *testing.T) {
	engine, _ := newTestEngine(t)
	for range 10 {
		require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: t.Name() + "a", Kind: "function"}))
	}

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			Direction: DirectionOut,
			MaxDepth:  0,
			Limit:     100,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	_ = page
}

func TestSubgraphContext_ContextTimeout(t *testing.T) {
	engine, _ := newTestEngine(t)
	buildBranchingGraph(engine, "root", 4, 6)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond)

	_, err := engine.SubgraphPage(ctx, GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"root"},
			Direction: DirectionOut,
			MaxDepth:  10,
			Limit:     10000,
		},
		PageSize: 10000,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}

func TestImpactSetContext_NegativeMaxDepth(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			MaxDepth:  -5,
			Direction: DirectionOut,
			Limit:     10,
		},
		PageSize: 10000,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "MaxDepth must be >= 0")
}

func TestImpactSetContext_OnlyOriginWithinLimit(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))

	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:   []string{"a"},
			MaxDepth:  5,
			Direction: DirectionOut,
			Limit:     1,
		},
		PageSize: 10000,
	})
	require.NoError(t, err)
	nodeIDs := collectNodeIDs(page)
	require.Contains(t, nodeIDs, "a")
}

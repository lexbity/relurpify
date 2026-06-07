package graphdb

import (
	"testing"

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

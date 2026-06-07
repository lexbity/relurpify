package graphdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLink_SingleEdge(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "src", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "tgt", Kind: "function"}))

	err := engine.Link("src", "tgt", "calls", "", 1.5, map[string]any{"site": "x"})
	require.NoError(t, err)

	out := engine.GetOutEdges("src", "calls")
	require.Len(t, out, 1)
	require.Equal(t, "tgt", out[0].TargetID)
	require.Equal(t, float32(1.5), out[0].Weight)
	require.JSONEq(t, `{"site":"x"}`, string(out[0].Props))
}

func TestLink_WithInverse(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))

	err := engine.Link("a", "b", "calls", "called_by", 1, nil)
	require.NoError(t, err)

	outA := engine.GetOutEdges("a", "calls")
	require.Len(t, outA, 1)
	require.Equal(t, "b", outA[0].TargetID)

	outB := engine.GetOutEdges("b", "called_by")
	require.Len(t, outB, 1)
	require.Equal(t, "a", outB[0].TargetID)
}

func TestLinkEdges_Batch(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"n1", "n2", "n3"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}

	edges := []EdgeRecord{
		{SourceID: "n1", TargetID: "n2", Kind: "calls", Weight: 1},
		{SourceID: "n2", TargetID: "n3", Kind: "calls", Weight: 1},
		{SourceID: "n3", TargetID: "n1", Kind: "calls", Weight: 1},
	}
	require.NoError(t, engine.LinkEdges(edges))

	require.Len(t, engine.GetOutEdges("n1"), 1)
	require.Len(t, engine.GetOutEdges("n2"), 1)
	require.Len(t, engine.GetOutEdges("n3"), 1)
}

func TestUnlink_SoftDelete(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))

	// soft delete
	require.NoError(t, engine.Unlink("a", "b", "calls", false))

	out := engine.GetOutEdges("a", "calls")
	require.Empty(t, out) // filtered out because edge is inactive

	// but edge still exists in store with DeletedAt set
	engine.store.mu.RLock()
	forward := engine.store.forward["a"]
	engine.store.mu.RUnlock()
	require.Len(t, forward, 1)
	require.NotZero(t, forward[0].DeletedAt)
}

func TestUnlink_HardDelete(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "x", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "y", Kind: "function"}))
	require.NoError(t, engine.Link("x", "y", "imports", "", 1, nil))

	require.NoError(t, engine.Unlink("x", "y", "imports", true))

	engine.store.mu.RLock()
	forward := engine.store.forward["x"]
	engine.store.mu.RUnlock()
	require.Empty(t, forward) // edge completely removed
}

func TestGetOutEdges_KindFilter(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "p", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "q", Kind: "function"}))
	require.NoError(t, engine.Link("p", "q", "calls", "", 1, nil))
	require.NoError(t, engine.Link("p", "q", "imports", "", 1, nil))

	outCalls := engine.GetOutEdges("p", "calls")
	require.Len(t, outCalls, 1)
	require.Equal(t, EdgeKind("calls"), outCalls[0].Kind)

	outImports := engine.GetOutEdges("p", "imports")
	require.Len(t, outImports, 1)
	require.Equal(t, EdgeKind("imports"), outImports[0].Kind)

	outBoth := engine.GetOutEdges("p")
	require.Len(t, outBoth, 2)
}

func TestGetInEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "from", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "to", Kind: "function"}))
	require.NoError(t, engine.Link("from", "to", "calls", "", 1, nil))

	in := engine.GetInEdges("to", "calls")
	require.Len(t, in, 1)
	require.Equal(t, "from", in[0].SourceID)
}

func TestEdgeUpsert_UpdateExisting(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "u", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "v", Kind: "function"}))

	// first edge
	edge1 := EdgeRecord{SourceID: "u", TargetID: "v", Kind: "calls", Weight: 1, Props: []byte(`{"a":1}`)}
	require.NoError(t, engine.LinkEdges([]EdgeRecord{edge1}))

	// second edge with same source/target/kind but different weight
	edge2 := EdgeRecord{SourceID: "u", TargetID: "v", Kind: "calls", Weight: 2, Props: []byte(`{"a":2}`)}
	require.NoError(t, engine.LinkEdges([]EdgeRecord{edge2}))

	out := engine.GetOutEdges("u", "calls")
	require.Len(t, out, 1)
	require.Equal(t, float32(2), out[0].Weight)
	require.JSONEq(t, `{"a":2}`, string(out[0].Props))
}

package graphdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloneNode_Nil(t *testing.T) {
	out := cloneNode(nil)
	require.Equal(t, NodeRecord{}, out)
}

func TestCloneNode_WithData(t *testing.T) {
	node := &NodeRecord{
		ID:        "orig",
		Kind:      "function",
		Labels:    []string{"a", "b"},
		Props:     []byte(`{"x":1}`),
		CreatedAt: 100,
		UpdatedAt: 200,
		DeletedAt: 0,
	}
	copied := cloneNode(node)
	require.EqualValues(t, *node, copied)
	// ensure slices are copies
	require.NotSame(t, node.Labels, copied.Labels)
	require.NotSame(t, node.Props, copied.Props)
}

func TestCloneEdge(t *testing.T) {
	edge := EdgeRecord{
		SourceID:  "s",
		TargetID:  "t",
		Kind:      "calls",
		Weight:    1.5,
		Props:     []byte(`{"site":"x"}`),
		CreatedAt: 1000,
		DeletedAt: 0,
	}
	copied := cloneEdge(edge)
	require.EqualValues(t, edge, copied)
	require.NotSame(t, edge.Props, copied.Props)
}

func TestCloneEdges_Empty(t *testing.T) {
	out := cloneEdges(nil)
	require.Nil(t, out)
	out = cloneEdges([]EdgeRecord{})
	require.Nil(t, out)
}

func TestKindSet(t *testing.T) {
	kinds := []EdgeKind{"calls", "imports"}
	m := kindSet(kinds)
	require.Len(t, m, 2)
	require.Contains(t, m, EdgeKind("calls"))
	require.Contains(t, m, EdgeKind("imports"))

	// empty slice
	m2 := kindSet([]EdgeKind{})
	require.Nil(t, m2)

	// nil slice
	m3 := kindSet(nil)
	require.Nil(t, m3)
}

func TestMatchKinds_EmptyAllowed(t *testing.T) {
	allowed := kindSet(nil)
	require.True(t, matchKinds("calls", allowed))
	require.True(t, matchKinds("imports", allowed))
}

func TestMatchKinds_SpecificAllowed(t *testing.T) {
	allowed := kindSet([]EdgeKind{"calls", "imports"})
	require.True(t, matchKinds("calls", allowed))
	require.True(t, matchKinds("imports", allowed))
	require.False(t, matchKinds("unknown", allowed))
}

func TestAddRemoveNodeSourceIndex(t *testing.T) {
	store := newAdjacencyStore()
	node := NodeRecord{ID: "id1", SourceID: "src1"}
	store.addNodeSourceIndex(node)
	require.Contains(t, store.bySource["src1"], "id1")

	// adding another node with same source
	node2 := NodeRecord{ID: "id2", SourceID: "src1"}
	store.addNodeSourceIndex(node2)
	require.Len(t, store.bySource["src1"], 2)

	// remove first
	store.removeNodeSourceIndex("id1", "src1")
	require.Contains(t, store.bySource["src1"], "id2")
	require.NotContains(t, store.bySource["src1"], "id1")

	// remove second (map should be deleted)
	store.removeNodeSourceIndex("id2", "src1")
	_, exists := store.bySource["src1"]
	require.False(t, exists)
}

func TestAddNodeSourceIndex_EmptySourceIgnored(t *testing.T) {
	store := newAdjacencyStore()
	store.addNodeSourceIndex(NodeRecord{ID: "x", SourceID: ""})
	require.Empty(t, store.bySource)
}

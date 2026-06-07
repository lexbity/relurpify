package graphdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertNode_NewNode(t *testing.T) {
	engine, _ := newTestEngine(t)
	node := NodeRecord{ID: "id1", Kind: "function", SourceID: "file.go", Labels: []string{"label1"}}
	require.NoError(t, engine.UpsertNode(node))

	retrieved, ok := engine.GetNode("id1")
	require.True(t, ok)
	require.Equal(t, node.ID, retrieved.ID)
	require.Equal(t, node.Kind, retrieved.Kind)
	require.Equal(t, node.SourceID, retrieved.SourceID)
	require.Equal(t, node.Labels, retrieved.Labels)
	require.NotZero(t, retrieved.CreatedAt)
	require.NotZero(t, retrieved.UpdatedAt)
	require.Zero(t, retrieved.DeletedAt)
}

func TestUpsertNode_UpdateExisting(t *testing.T) {
	engine, _ := newTestEngine(t)
	node1 := NodeRecord{ID: "n", Kind: "function", SourceID: "old.go"}
	require.NoError(t, engine.UpsertNode(node1))

	node2 := NodeRecord{ID: "n", Kind: "method", SourceID: "new.go"}
	require.NoError(t, engine.UpsertNode(node2))

	retrieved, ok := engine.GetNode("n")
	require.True(t, ok)
	require.Equal(t, NodeKind("method"), retrieved.Kind)
	require.Equal(t, "new.go", retrieved.SourceID)
	// CreatedAt should remain from first insertion
	require.NotZero(t, retrieved.CreatedAt)
	require.NotZero(t, retrieved.UpdatedAt)
	require.True(t, retrieved.UpdatedAt >= retrieved.CreatedAt)
}

func TestUpsertNodes_Batch(t *testing.T) {
	engine, _ := newTestEngine(t)
	nodes := []NodeRecord{
		{ID: "a", Kind: "function", SourceID: "a.go"},
		{ID: "b", Kind: "function", SourceID: "b.go"},
		{ID: "c", Kind: "method", SourceID: "c.go"},
	}
	require.NoError(t, engine.UpsertNodes(nodes))

	list := engine.ListNodes("function")
	require.Len(t, list, 2)
	ids := []string{list[0].ID, list[1].ID}
	require.ElementsMatch(t, []string{"a", "b"}, ids)
}

func TestDeleteNode_SoftDelete(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "toDelete", Kind: "function", SourceID: "x.go"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "other", Kind: "function", SourceID: "x.go"}))

	require.NoError(t, engine.DeleteNode("toDelete"))

	_, ok := engine.GetNode("toDelete")
	require.False(t, ok)

	// should still be in source index? No, because source index is removed on soft delete
	nodes := engine.NodesBySource("x.go")
	require.Len(t, nodes, 1)
	require.Equal(t, "other", nodes[0].ID)
}

func TestDeleteNodes_Batch(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"d1", "d2", "d3", "keep"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.DeleteNodes([]string{"d1", "d3"}))

	_, ok1 := engine.GetNode("d1")
	_, ok2 := engine.GetNode("d2")
	_, ok3 := engine.GetNode("d3")
	_, okKeep := engine.GetNode("keep")
	require.False(t, ok1)
	require.True(t, ok2)
	require.False(t, ok3)
	require.True(t, okKeep)
}

func TestListNodes_EmptyKind(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "f1", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "m1", Kind: "method"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "f2", Kind: "function"}))

	all := engine.ListNodes("")
	require.Len(t, all, 3)
}

func TestNodesBySource_Empty(t *testing.T) {
	engine, _ := newTestEngine(t)
	nodes := engine.NodesBySource("nonexistent.go")
	require.Empty(t, nodes)
}

func TestNodesBySource_Multiple(t *testing.T) {
	engine, _ := newTestEngine(t)
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function", SourceID: "common.go"}))
	}
	// one node with different source
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "z", Kind: "function", SourceID: "other.go"}))

	nodes := engine.NodesBySource("common.go")
	require.Len(t, nodes, 5)
	for _, n := range nodes {
		require.Equal(t, "common.go", n.SourceID)
	}
}

func TestGetNode_NotExist(t *testing.T) {
	engine, _ := newTestEngine(t)
	_, ok := engine.GetNode("ghost")
	require.False(t, ok)
}

func TestNodeSoftDeleteAlsoSoftDeletesEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "hub", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "leaf", Kind: "function"}))
	require.NoError(t, engine.Link("hub", "leaf", "calls", "", 1, nil))
	require.NoError(t, engine.Link("leaf", "hub", "called_by", "", 1, nil))

	require.NoError(t, engine.DeleteNode("hub"))

	// edges should be soft‑deleted
	out := engine.GetOutEdges("hub")
	require.Empty(t, out)
	in := engine.GetInEdges("hub")
	require.Empty(t, in)

	// leaf's outgoing edge to hub should also be soft‑deleted
	leafOut := engine.GetOutEdges("leaf", "called_by")
	require.Empty(t, leafOut)
}

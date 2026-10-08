package graphdb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpen_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	// should be usable
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "test", Kind: "function"}))
	node, ok := engine.GetNode("test")
	require.True(t, ok)
	require.Equal(t, "test", node.ID)
}

func TestOpen_WithExistingData(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))

	// create engine, write data, close (Badger persists on commit)
	eng1, err := Open(context.Background(), opts)
	require.NoError(t, err)
	require.NoError(t, eng1.UpsertNode(context.TODO(), NodeRecord{ID: "persisted", Kind: "function", SourceID: "x.go"}))
	require.NoError(t, eng1.Link(context.TODO(), "persisted", "other", "calls", "", 1, nil))
	require.NoError(t, eng1.Close(context.Background()))

	// reopen
	eng2, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = eng2.Close(context.Background()) }()

	node, ok := eng2.GetNode("persisted")
	require.True(t, ok)
	require.Equal(t, "x.go", node.SourceID)
	// edge should be present (target node may not exist, but edge record exists)
	edges := eng2.GetOutEdges("persisted")
	require.Len(t, edges, 1)
	require.Equal(t, "other", edges[0].TargetID)
}

func TestReopen_Badger(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)

	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "snap", Kind: "function"}))
	require.NoError(t, engine.Close(context.Background()))

	eng2, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = eng2.Close(context.Background()) }()
	_, ok := eng2.GetNode("snap")
	require.True(t, ok)
}

func TestCloseAndReopen_Badger(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)

	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "persist", Kind: "function"}))
	require.NoError(t, engine.Close(context.Background()))

	eng2, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = eng2.Close(context.Background()) }()
	_, ok := eng2.GetNode("persist")
	require.True(t, ok)
}

func TestApplyBinaryOp_UnknownCode(t *testing.T) {
	engine := &Engine{store: newAdjacencyStore()}
	op := binaryOp{code: 0xFF, data: []byte{}}
	err := engine.applyBinaryOp(op)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown binary op code")
}

func TestApplyLegacyJSONOp_InvalidJSON(t *testing.T) {
	engine := &Engine{store: newAdjacencyStore()}
	err := engine.applyLegacyJSONOp([]byte(`{not json}`))
	require.Error(t, err)
}

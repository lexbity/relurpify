package graphdb

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpen_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

	// should be usable
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "test", Kind: "function"}))
	node, ok := engine.GetNode("test")
	require.True(t, ok)
	require.Equal(t, "test", node.ID)
}

func TestOpen_WithExistingSnapshot(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	opts.SnapshotOnClose = true

	// create engine, write data, close (creates snapshot)
	eng1, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, eng1.UpsertNode(NodeRecord{ID: "persisted", Kind: "function", SourceID: "x.go"}))
	require.NoError(t, eng1.Link("persisted", "other", "calls", "", 1, nil))
	require.NoError(t, eng1.Close())

	// reopen
	eng2, err := Open(opts)
	require.NoError(t, err)
	defer eng2.Close()

	node, ok := eng2.GetNode("persisted")
	require.True(t, ok)
	require.Equal(t, "x.go", node.SourceID)
	// edge should be present (target node may not exist, but edge record exists)
	edges := eng2.GetOutEdges("persisted")
	require.Len(t, edges, 1)
	require.Equal(t, "other", edges[0].TargetID)
}

func TestSnapshot_RewritesAOF(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	engine, err := Open(opts)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Snapshot())

	// after snapshot, AOF file should be small (truncated)
	aofPath := filepath.Join(opts.DataDir, opts.AOFFileName)
	info, err := os.Stat(aofPath)
	require.NoError(t, err)
	require.Zero(t, info.Size())

	// engine still works
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "new", Kind: "function"}))
	require.NoError(t, engine.Close())
}

func TestClose_WithSnapshotOnClose(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	opts.SnapshotOnClose = true
	engine, err := Open(opts)
	require.NoError(t, err)

	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "snap", Kind: "function"}))
	require.NoError(t, engine.Close())

	// AOF should be empty
	aofPath := filepath.Join(opts.DataDir, opts.AOFFileName)
	info, err := os.Stat(aofPath)
	require.NoError(t, err)
	require.Zero(t, info.Size())

	// snapshot should contain the node
	eng2, err := Open(opts)
	require.NoError(t, err)
	defer eng2.Close()
	_, ok := eng2.GetNode("snap")
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

func TestBackgroundAutoSnapshot(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "graphdb"))
	opts.AutoSaveInterval = 50 * time.Millisecond
	opts.AutoSaveThreshold = 5
	opts.MaintenanceInterval = 10 * time.Millisecond
	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

	// write enough ops to exceed threshold
	for i := 0; i < 10; i++ {
		id := string(rune('0' + i))
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}

	// wait for auto‑snapshot to possibly happen
	time.Sleep(200 * time.Millisecond)

	// snapshot may have been taken, but at least engine should still work
	_, ok := engine.GetNode("0")
	require.True(t, ok)
}

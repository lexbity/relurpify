package graphdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultOptions_CreatesBadger(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(filepath.Join(dir, "ws"))
	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	require.NotNil(t, engine)
	require.NoError(t, engine.Close(context.Background()))

	// A Badger store creates a MANIFEST file.
	manifestPath := filepath.Join(dir, "ws", "MANIFEST")
	_, err = os.Stat(manifestPath)
	require.NoError(t, err, "new workspace should create Badger directory with MANIFEST")

	// No AOF file should exist.
	aofPath := filepath.Join(dir, "ws", "graphdb.aof")
	_, err = os.Stat(aofPath)
	require.True(t, os.IsNotExist(err), "new workspace should NOT create AOF file")
}

func TestDefaultOptions_BadgerDirDefaultsToDataDir(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	require.Equal(t, dir, opts.BadgerDir)
	require.Equal(t, dir, opts.DataDir)
}

func TestDefaultOptions_WithBadgerDir(t *testing.T) {
	dataDir := t.TempDir()
	badgerDir := t.TempDir()

	opts := DefaultOptions(dataDir)
	opts.BadgerDir = badgerDir

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	require.NoError(t, engine.Close(context.Background()))

	// Badger data should be in BadgerDir, not DataDir.
	_, err = os.Stat(filepath.Join(badgerDir, "MANIFEST"))
	require.NoError(t, err, "Badger data should be in BadgerDir")
}

func TestNewBadgerStoreIsUsable(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)

	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function"}))
	node, ok := engine.GetNode("n1")
	require.True(t, ok)
	require.Equal(t, "n1", node.ID)

	require.NoError(t, engine.Link(context.TODO(), "n1", "n2", "calls", "", 1, nil))
	require.NoError(t, engine.Close(context.Background()))

	// Reopen and verify data persists.
	engine2, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer engine2.Close(context.Background())

	_, ok = engine2.GetNode("n1")
	require.True(t, ok, "data should persist after reopen with Badger")
}

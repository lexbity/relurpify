package graphdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteReadSnapshot_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.snap")
	state := snapshotState{}
	require.NoError(t, writeSnapshot(path, state))

	read, err := readSnapshot(path)
	require.NoError(t, err)
	require.Empty(t, read.Nodes)
	require.Empty(t, read.Forward)
}

func TestWriteReadSnapshot_WithData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.snap")
	state := snapshotState{
		Nodes: []NodeRecord{
			{ID: "n1", Kind: "function", SourceID: "f.go"},
			{ID: "n2", Kind: "method", SourceID: "f.go"},
		},
		Forward: []EdgeRecord{
			{SourceID: "n1", TargetID: "n2", Kind: "calls", Weight: 1},
		},
	}
	require.NoError(t, writeSnapshot(path, state))

	read, err := readSnapshot(path)
	require.NoError(t, err)
	require.Len(t, read.Nodes, 2)
	require.Len(t, read.Forward, 1)
	require.Equal(t, "n1", read.Nodes[0].ID)
	require.Equal(t, "n1", read.Forward[0].SourceID)
}

func TestReadSnapshot_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.snap")
	state, err := readSnapshot(path)
	require.NoError(t, err)
	require.Empty(t, state.Nodes)
	require.Empty(t, state.Forward)
}

func TestReadSnapshot_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.snap")
	require.NoError(t, os.WriteFile(path, []byte(`{not json}`), 0o644))

	_, err := readSnapshot(path)
	require.Error(t, err)
}

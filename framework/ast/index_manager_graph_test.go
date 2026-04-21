package ast

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"github.com/stretchr/testify/require"
)

func TestIndexManagerPopulatesGraphDBForGoFile(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(tmpDir, "graphdb")))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close())
	})
	manager.GraphDB = graphEngine

	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package sample
import "fmt"
func Helper() {}
func Hello() { Helper(); _ = fmt.Sprintf }
`), 0o644))

	require.NoError(t, manager.IndexFile(path))

	graphNodes := manager.GraphDB.NodesBySource(path)
	require.NotEmpty(t, graphNodes)
	for _, node := range graphNodes {
		require.Equal(t, path, node.SourceID)
	}

	meta, err := manager.Store().GetFileByPath(path)
	require.NoError(t, err)
	require.NotNil(t, meta)

	importEdges := manager.GraphDB.GetOutEdges(meta.RootNodeID, EdgeKindImports)
	require.NotEmpty(t, importEdges)

	helloNodeID := meta.ID + ":func:Hello"
	callEdges := manager.GraphDB.GetOutEdges(helloNodeID, EdgeKindCalls)
	require.NotEmpty(t, callEdges)
	require.Equal(t, meta.ID+":func:Helper", callEdges[0].TargetID)

	containsEdges := manager.GraphDB.GetOutEdges(meta.RootNodeID, EdgeKindContains)
	require.NotEmpty(t, containsEdges)
}

func TestIndexManagerReindexReplacesGraphNodesForSource(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(tmpDir, "graphdb")))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close())
	})
	manager.GraphDB = graphEngine

	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package sample
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	require.NoError(t, os.WriteFile(path, []byte(`package sample
func Goodbye() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	graphNodes := manager.GraphDB.NodesBySource(path)
	var ids []string
	for _, node := range graphNodes {
		ids = append(ids, node.ID)
	}
	require.Contains(t, ids, GenerateFileID(path)+":func:Goodbye")
	require.NotContains(t, ids, GenerateFileID(path)+":func:Hello")
}

func TestIndexManagerGraphDBNilDoesNotPanic(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package sample
func Hello() {}
`), 0o644))

	require.NotPanics(t, func() {
		require.NoError(t, manager.IndexFile(path))
	})
}

func TestIndexManagerRefreshFilesRemovesGraphNodesForDeletedFile(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(tmpDir, "graphdb")))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close())
	})
	manager.GraphDB = graphEngine

	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package sample
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))
	require.NotEmpty(t, manager.GraphDB.NodesBySource(path))

	require.NoError(t, os.Remove(path))
	require.NoError(t, manager.RefreshFiles([]string{path}))
	require.Empty(t, manager.GraphDB.NodesBySource(path))
}

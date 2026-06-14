package ast

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

const (
	Graphdb_index_manager_graph_test                = "graphdb"
	MainGoFile_index_manager_graph_test             = "main.go"
	PackagesamplefuncHello_index_manager_graph_test = "package sample\nfunc Hello() {}\n"
)

func TestIndexManagerPopulatesGraphDBForGoFile(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	graphEngine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(filepath.Join(tmpDir, Graphdb_index_manager_graph_test)))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close(context.Background()))
	})
	manager.GraphDB = graphEngine

	path := filepath.Join(tmpDir, MainGoFile_index_manager_graph_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte("package sample\nimport \"fmt\"\nfunc Helper() {}\nfunc Hello() { Helper(); _ = fmt.Sprintf }\n")))

	require.NoError(t, manager.IndexFile(context.Background(), path))

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
	graphEngine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(filepath.Join(tmpDir, Graphdb_index_manager_graph_test)))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close(context.Background()))
	})
	manager.GraphDB = graphEngine

	path := filepath.Join(tmpDir, MainGoFile_index_manager_graph_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagesamplefuncHello_index_manager_graph_test)))
	require.NoError(t, manager.IndexFile(context.Background(), path))

	require.NoError(t, fs.WriteFileSecure(path, []byte("package sample\nfunc Goodbye() {}\n")))
	require.NoError(t, manager.IndexFile(context.Background(), path))

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
	path := filepath.Join(tmpDir, MainGoFile_index_manager_graph_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagesamplefuncHello_index_manager_graph_test)))

	require.NotPanics(t, func() {
		require.NoError(t, manager.IndexFile(context.Background(), path))
	})
}

func TestIndexManagerRefreshFilesRemovesGraphNodesForDeletedFile(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	graphEngine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(filepath.Join(tmpDir, Graphdb_index_manager_graph_test)))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close(context.Background()))
	})
	manager.GraphDB = graphEngine

	path := filepath.Join(tmpDir, MainGoFile_index_manager_graph_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagesamplefuncHello_index_manager_graph_test)))
	require.NoError(t, manager.IndexFile(context.Background(), path))
	require.NotEmpty(t, manager.GraphDB.NodesBySource(path))

	require.NoError(t, os.Remove(path))
	require.NoError(t, manager.RefreshFiles(context.Background(), []string{path}))
	require.Empty(t, manager.GraphDB.NodesBySource(path))
}

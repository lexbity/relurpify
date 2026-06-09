package testsuite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
)

// TestBadgerRuntime_OpenClose verifies a Badger-backed engine opens and
// closes cleanly.
func TestBadgerRuntime_OpenClose(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	require.NotNil(t, engine)
	require.NoError(t, engine.Close())
}

// TestBadgerRuntime_ChunkStore verifies the knowledge ChunkStore works
// with a Badger-backed graphdb engine.
func TestBadgerRuntime_ChunkStore(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer engine.Close()

	store := &knowledge.ChunkStore{Graph: engine}

	// Save a chunk
	chunk := knowledge.KnowledgeChunk{
		ID:          "chunk:test",
		WorkspaceID: "ws-1",
		Body:        knowledge.ChunkBody{Raw: "test content", Fields: map[string]any{"key": "val"}},
		Freshness:   knowledge.FreshnessValid,
	}
	saved, err := store.Save(chunk)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, knowledge.ChunkID("chunk:test"), saved.ID)

	// Load the chunk
	loaded, ok, err := store.Load("chunk:test")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "test content", loaded.Body.Raw)
	require.Equal(t, "ws-1", loaded.WorkspaceID)
}

// TestBadgerRuntime_ChunkEdges verifies edge operations in the chunk
// store work with a Badger-backed engine.
func TestBadgerRuntime_ChunkEdges(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer engine.Close()

	store := &knowledge.ChunkStore{Graph: engine}

	_, err = store.Save(knowledge.KnowledgeChunk{ID: "chunk:a", WorkspaceID: "ws", Body: knowledge.ChunkBody{Raw: "a"}})
	require.NoError(t, err)
	_, err = store.Save(knowledge.KnowledgeChunk{ID: "chunk:b", WorkspaceID: "ws", Body: knowledge.ChunkBody{Raw: "b"}})
	require.NoError(t, err)

	saved, err := store.SaveEdge(knowledge.ChunkEdge{
		FromChunk: "chunk:a",
		ToChunk:   "chunk:b",
		Kind:      knowledge.EdgeKindRequiresContext,
		Weight:    1,
	})
	require.NoError(t, err)
	require.NotNil(t, saved)

	loaded, ok, err := store.LoadEdge("chunk:a", "chunk:b", knowledge.EdgeKindRequiresContext)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, knowledge.EdgeKindRequiresContext, loaded.Kind)

	edges, err := store.LoadEdgesFrom("chunk:a", knowledge.EdgeKindRequiresContext)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.Equal(t, "chunk:b", string(edges[0].ToChunk))
}

// TestBadgerRuntime_ASTIndexStore verifies the graph-backed AST index
// store works with a Badger engine.
func TestBadgerRuntime_ASTIndexStore(t *testing.T) {
	dir := t.TempDir()
	engine, err := graphdb.Open(graphdb.DefaultOptions(dir))
	require.NoError(t, err)
	defer engine.Close()

	astStore := ast.NewGraphIndexStore(engine)

	// Save a file metadata node
	fileMeta := &ast.FileMetadata{
		ID:           "file:main.go",
		Path:         filepath.Join(dir, "src", "main.go"),
		RelativePath: "src/main.go",
		Language:     "go",
		Category:     ast.CategoryCode,
	}
	err = astStore.SaveFile(fileMeta)
	require.NoError(t, err)

	// Retrieve by path
	loaded, err := astStore.GetFileByPath(fileMeta.Path)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "go", loaded.Language)

	// Save an AST node
	astNode := &ast.Node{
		ID:     "node:func1",
		FileID: "file:main.go",
		Type:   ast.NodeTypeFunction,
		Name:   "main",
	}
	err = astStore.SaveNodes([]*ast.Node{astNode})
	require.NoError(t, err)

	// Query nodes by file
	nodes, err := astStore.GetNodesByFile("file:main.go")
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "main", nodes[0].Name)
}

// TestBadgerRuntime_Retriever verifies the retrieval path works with a
// Badger-backed graphdb engine.
func TestBadgerRuntime_Retriever(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer engine.Close()

	store := &knowledge.ChunkStore{Graph: engine}

	_, err = store.Save(knowledge.KnowledgeChunk{
		ID: "chunk:searchable", WorkspaceID: "ws",
		Body: knowledge.ChunkBody{Raw: "important content", Fields: map[string]any{"content": "important content"}},
	})
	require.NoError(t, err)

	registry := retrieval.NewRankerRegistry()
	registry.Register(&retrieval.KeywordRanker{K1: 1.2, B: 0.75})

	retriever := retrieval.NewRetriever(registry, store)
	results, err := retriever.Retrieve(context.Background(), retrieval.RetrievalQuery{Text: "important"})
	require.NoError(t, err)
	require.NotEmpty(t, results)
}

// TestBadgerRuntime_CloseCleanly verifies that closing a Badger-backed
// engine that has been used for chunk storage and AST indexing does not
// error.
func TestBadgerRuntime_CloseCleanly(t *testing.T) {
	dir := t.TempDir()
	engine, err := graphdb.Open(graphdb.DefaultOptions(dir))
	require.NoError(t, err)

	store := &knowledge.ChunkStore{Graph: engine}
	_, err = store.Save(knowledge.KnowledgeChunk{ID: "chunk:preclose", WorkspaceID: "ws", Body: knowledge.ChunkBody{Raw: "data"}})
	require.NoError(t, err)

	astStore := ast.NewGraphIndexStore(engine)
	err = astStore.SaveFile(&ast.FileMetadata{
		ID: "file:test.go", Path: filepath.Join(dir, "test.go"), Language: "go", Category: ast.CategoryCode,
	})
	require.NoError(t, err)

	// Verify edge operations before close
	_, err = store.SaveEdge(knowledge.ChunkEdge{FromChunk: "chunk:preclose", ToChunk: "chunk:preclose", Kind: knowledge.EdgeKindRequiresContext})
	require.NoError(t, err)

	// Close should succeed
	require.NoError(t, engine.Close())
}

package ast

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SQLiteStore Additional Tests ====================

func TestSQLiteStoreListFiles(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	// Create files in different categories
	files := []*FileMetadata{
		{ID: "f1", Path: "/a.go", RelativePath: "a.go", Language: "go", Category: CategoryCode, ContentHash: "h1", IndexedAt: time.Now()},
		{ID: "f2", Path: "/b.md", RelativePath: "b.md", Language: "markdown", Category: CategoryDoc, ContentHash: "h2", IndexedAt: time.Now()},
		{ID: "f3", Path: "/c.yaml", RelativePath: "c.yaml", Language: "yaml", Category: CategoryConfig, ContentHash: "h3", IndexedAt: time.Now()},
	}
	for _, f := range files {
		require.NoError(t, store.SaveFile(f))
	}

	// List all files
	all, err := store.ListFiles("")
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// List by category
	codeFiles, err := store.ListFiles(CategoryCode)
	require.NoError(t, err)
	assert.Len(t, codeFiles, 1)
	assert.Equal(t, "go", codeFiles[0].Language)
}

func TestSQLiteStoreGetNodesByTypeAndName(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n3", FileID: fileID, Type: NodeTypeStruct, Name: "MyStruct", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// Get by type
	funcs, err := store.GetNodesByType(NodeTypeFunction)
	require.NoError(t, err)
	assert.Len(t, funcs, 2)

	structs, err := store.GetNodesByType(NodeTypeStruct)
	require.NoError(t, err)
	assert.Len(t, structs, 1)

	// Get by name
	hello, err := store.GetNodesByName("Hello")
	require.NoError(t, err)
	assert.Len(t, hello, 1)
	assert.Equal(t, "n1", hello[0].ID)
}

func TestSQLiteStoreDeleteNodeAndEdge(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Delete node
	require.NoError(t, store.DeleteNode("n1"))
	_, err = store.GetNode("n1")
	assert.Error(t, err)

	// Delete edge
	require.NoError(t, store.DeleteEdge("e1"))
	_, err = store.GetEdge("e1")
	assert.Error(t, err)
}

func TestSQLiteStoreGetEdge(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls, Attributes: map[string]interface{}{"line": 42}},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Get edge
	edge, err := store.GetEdge("e1")
	require.NoError(t, err)
	assert.Equal(t, "e1", edge.ID)
	assert.Equal(t, EdgeTypeCalls, edge.Type)
}

func TestSQLiteStoreGetEdgesBySourceAndTarget(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n3", FileID: fileID, Type: NodeTypeFunction, Name: "Foo", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls},
		{ID: "e2", SourceID: "n1", TargetID: "n3", Type: EdgeTypeCalls},
		{ID: "e3", SourceID: "n2", TargetID: "n3", Type: EdgeTypeImports},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Get by source
	sourceEdges, err := store.GetEdgesBySource("n1")
	require.NoError(t, err)
	assert.Len(t, sourceEdges, 2)

	// Get by target
	targetEdges, err := store.GetEdgesByTarget("n3")
	require.NoError(t, err)
	assert.Len(t, targetEdges, 2)
}

func TestSQLiteStoreGetEdgesByType(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls},
		{ID: "e2", SourceID: "n2", TargetID: "n1", Type: EdgeTypeImports},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Get by type
	calls, err := store.GetEdgesByType(EdgeTypeCalls)
	require.NoError(t, err)
	assert.Len(t, calls, 1)

	imports, err := store.GetEdgesByType(EdgeTypeImports)
	require.NoError(t, err)
	assert.Len(t, imports, 1)
}

func TestSQLiteStoreSearchEdges(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n3", FileID: fileID, Type: NodeTypeFunction, Name: "Foo", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls},
		{ID: "e2", SourceID: "n1", TargetID: "n3", Type: EdgeTypeImports},
		{ID: "e3", SourceID: "n2", TargetID: "n3", Type: EdgeTypeCalls},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Search by type
	results, err := store.SearchEdges(EdgeQuery{Types: []EdgeType{EdgeTypeCalls}})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Search by source
	results, err = store.SearchEdges(EdgeQuery{SourceIDs: []string{"n1"}})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Search by target
	results, err = store.SearchEdges(EdgeQuery{TargetIDs: []string{"n3"}})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Search with limit
	results, err = store.SearchEdges(EdgeQuery{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestSQLiteStoreGetCalleesAndCallers(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n3", FileID: fileID, Type: NodeTypeFunction, Name: "Foo", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// n1 calls n2, n1 calls n3
	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls},
		{ID: "e2", SourceID: "n1", TargetID: "n3", Type: EdgeTypeCalls},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Get callees (who n1 calls)
	callees, err := store.GetCallees("n1")
	require.NoError(t, err)
	assert.Len(t, callees, 2)

	// Get callers (who calls n2)
	callers, err := store.GetCallers("n2")
	require.NoError(t, err)
	assert.Len(t, callers, 1)
	assert.Equal(t, "n1", callers[0].ID)
}

func TestSQLiteStoreGetImportsAndImportedBy(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypePackage, Name: "main", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeImport, Name: "fmt", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeImports},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Get imports
	imports, err := store.GetImports("n1")
	require.NoError(t, err)
	assert.Len(t, imports, 1)
	assert.Equal(t, "n2", imports[0].ID)

	// Get imported by
	importedBy, err := store.GetImportedBy("n2")
	require.NoError(t, err)
	assert.Len(t, importedBy, 1)
	assert.Equal(t, "n1", importedBy[0].ID)
}

func TestSQLiteStoreGetReferencesAndReferencedBy(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeReferences},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Get references
	refs, err := store.GetReferences("n1")
	require.NoError(t, err)
	assert.Len(t, refs, 1)

	// Get referenced by
	refBy, err := store.GetReferencedBy("n2")
	require.NoError(t, err)
	assert.Len(t, refBy, 1)
}

func TestSQLiteStoreGetDependenciesAndDependents(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypePackage, Name: "main", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeImport, Name: "fmt", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n3", FileID: fileID, Type: NodeTypeImport, Name: "strings", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// n1 -> n2 -> n3 (chain of imports)
	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeImports},
		{ID: "e2", SourceID: "n2", TargetID: "n3", Type: EdgeTypeDependsOn},
	}
	require.NoError(t, store.SaveEdges(edges))

	// Get dependencies
	deps, err := store.GetDependencies("n1")
	require.NoError(t, err)
	assert.Len(t, deps, 2)

	// Get dependents
	dependents, err := store.GetDependents("n3")
	require.NoError(t, err)
	assert.Len(t, dependents, 2)
}

func TestSQLiteStoreVacuum(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	// Vacuum should not error
	require.NoError(t, store.Vacuum())
}

func TestSQLiteStoreTransactionDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	tx, err := store.BeginTransaction()
	require.NoError(t, err)

	require.NoError(t, tx.DeleteFile(fileID))
	require.NoError(t, tx.Commit())

	_, err = store.GetFile(fileID)
	assert.Error(t, err)
}

func TestSQLiteStorePlaceholders(t *testing.T) {
	// placeholders is an internal function, tested via SearchNodes with multiple types
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode, ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeStruct, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n3", FileID: fileID, Type: NodeTypeInterface, Name: "Foo", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// Search with multiple types (triggers placeholders function)
	results, err := store.SearchNodes(NodeQuery{Types: []NodeType{NodeTypeFunction, NodeTypeStruct, NodeTypeInterface}})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

// ==================== IndexManager Query Tests ====================

func TestIndexManagerQuerySymbol(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
func HelloWorld() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	// Query for symbols
	results, err := manager.QuerySymbol("Hello")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestIndexManagerSearchNodes(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
func World() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	// Search nodes
	results, err := manager.SearchNodes(NodeQuery{NamePattern: "Hello"})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestIndexManagerStats(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	stats, err := manager.Stats()
	require.NoError(t, err)
	assert.Greater(t, stats.TotalFiles, 0)
	assert.Greater(t, stats.TotalNodes, 0)
}

func TestIndexManagerLastIndexedAt(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	ts, err := manager.LastIndexedAt(path)
	require.NoError(t, err)
	assert.False(t, ts.IsZero())
}

func TestIndexManagerLastIndexError(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	// Initially no error
	err := manager.LastIndexError()
	assert.NoError(t, err)
}

func TestIndexManagerGetCallGraph(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Helper() {}
func Hello() { Helper() }
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	graph, err := manager.GetCallGraph("Hello")
	require.NoError(t, err)
	assert.NotNil(t, graph.Root)
	assert.Equal(t, "Hello", graph.Root.Name)
}

func TestIndexManagerGetCallGraphNotFound(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	_, err := manager.GetCallGraph("NonExistent")
	assert.Error(t, err)
}

func TestIndexManagerGetDependencyGraph(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Helper() {}
func Hello() { Helper() }
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	graph, err := manager.GetDependencyGraph("Hello")
	require.NoError(t, err)
	assert.NotNil(t, graph.Root)
}

func TestIndexManagerGetDependencyGraphNotFound(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	_, err := manager.GetDependencyGraph("NonExistent")
	assert.Error(t, err)
}

func TestIndexManagerSetPathFilter(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Set a filter that allows everything
	manager.SetPathFilter(func(path string, isDir bool) bool {
		return true
	})

	// Set a filter that blocks everything
	manager.SetPathFilter(func(path string, isDir bool) bool {
		return false
	})

	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))

	// With filter blocking, file should not be indexed
	err = manager.IndexFile(path)
	// No error, but file should not be indexed due to filter
	assert.NoError(t, err)
}

// ==================== Parser Tests ====================

func TestGoParserCategoryAndSupportsIncremental(t *testing.T) {
	parser := NewGoParser()
	assert.Equal(t, CategoryCode, parser.Category())
	assert.False(t, parser.SupportsIncremental())

	_, err := parser.ParseIncremental(nil, nil)
	assert.Error(t, err)
}

func TestMarkdownParserCategoryAndSupportsIncremental(t *testing.T) {
	parser := NewMarkdownParser()
	assert.Equal(t, CategoryDoc, parser.Category())
	assert.False(t, parser.SupportsIncremental())

	_, err := parser.ParseIncremental(nil, nil)
	assert.Error(t, err)
}

func TestLanguageDetectorDetectCategory(t *testing.T) {
	detector := NewLanguageDetector()

	// Code languages
	assert.Equal(t, CategoryCode, detector.DetectCategory("go"))
	assert.Equal(t, CategoryCode, detector.DetectCategory("python"))
	assert.Equal(t, CategoryCode, detector.DetectCategory("javascript"))
	assert.Equal(t, CategoryCode, detector.DetectCategory("typescript"))
	assert.Equal(t, CategoryCode, detector.DetectCategory("java"))
	assert.Equal(t, CategoryCode, detector.DetectCategory("c"))
	assert.Equal(t, CategoryCode, detector.DetectCategory("cpp"))
	assert.Equal(t, CategoryCode, detector.DetectCategory("rust"))

	// Doc languages
	assert.Equal(t, CategoryDoc, detector.DetectCategory("markdown"))
	assert.Equal(t, CategoryDoc, detector.DetectCategory("restructuredtext"))
	assert.Equal(t, CategoryDoc, detector.DetectCategory("plaintext"))
	assert.Equal(t, CategoryDoc, detector.DetectCategory("asciidoc"))

	// Config languages
	assert.Equal(t, CategoryConfig, detector.DetectCategory("yaml"))
	assert.Equal(t, CategoryConfig, detector.DetectCategory("json"))
	assert.Equal(t, CategoryConfig, detector.DetectCategory("toml"))
	assert.Equal(t, CategoryConfig, detector.DetectCategory("xml"))
	assert.Equal(t, CategoryConfig, detector.DetectCategory("ini"))

	// Schema languages
	assert.Equal(t, CategorySchema, detector.DetectCategory("sql"))
	assert.Equal(t, CategorySchema, detector.DetectCategory("graphql"))
	assert.Equal(t, CategorySchema, detector.DetectCategory("protobuf"))

	// Infra languages
	assert.Equal(t, CategoryInfra, detector.DetectCategory("terraform"))
	assert.Equal(t, CategoryInfra, detector.DetectCategory("docker"))
	assert.Equal(t, CategoryInfra, detector.DetectCategory("docker-compose"))

	// Unknown falls back to doc
	assert.Equal(t, CategoryDoc, detector.DetectCategory("unknown"))
}

// ==================== GraphSchema Tests ====================

func TestGraphNodeRecordNil(t *testing.T) {
	// Test that graphNodeRecord handles nil node
	result, ok := graphNodeRecord(nil, "path")
	assert.False(t, ok)
	assert.Empty(t, result.ID)
}

func TestGraphNodeKind(t *testing.T) {
	tests := []struct {
		nodeType NodeType
		expected string
	}{
		{NodeTypeFunction, "function"},
		{NodeTypeMethod, "method"},
		{NodeTypeInterface, "interface"},
		{NodeTypeStruct, "struct"},
		{NodeTypePackage, "package"},
		{NodeTypeImport, "import"},
		{NodeTypeType, "type"},
		{NodeTypeDocument, "document"},
		{NodeTypeSection, "section"},
		{NodeTypeHeading, "section"},
		{NodeTypeVariable, "variable"},
	}

	for _, tt := range tests {
		kind, ok := graphNodeKind(tt.nodeType)
		assert.True(t, ok)
		assert.Equal(t, graphdb.NodeKind(tt.expected), kind)
	}
}

func TestGraphEdgeKinds(t *testing.T) {
	tests := []struct {
		edgeType     EdgeType
		expectedKind string
		expectedInv  string
		ok           bool
	}{
		{EdgeTypeCalls, "calls", "called_by", true},
		{EdgeTypeImports, "imports", "imported_by", true},
		{EdgeTypeImplements, "implements", "implemented_by", true},
		{EdgeTypeContains, "contains", "contained_by", true},
		{EdgeTypeDependsOn, "depends_on", "dependency_of", true},
		{EdgeTypeReferences, "references", "referenced_by", true},
		{EdgeTypeExtends, "extends", "extended_by", true},
		{EdgeTypeLinks, "", "", false},
	}

	for _, tt := range tests {
		kind, inv, ok := graphEdgeKinds(tt.edgeType)
		assert.Equal(t, tt.ok, ok)
		if tt.ok {
			assert.Equal(t, graphdb.EdgeKind(tt.expectedKind), kind)
			assert.Equal(t, graphdb.EdgeKind(tt.expectedInv), inv)
		}
	}
}

// ==================== IndexManager Lifecycle Edge Cases ====================

func TestIndexManagerNilReceiver(t *testing.T) {
	var manager *IndexManager

	// These should not panic
	assert.False(t, manager.Ready())
	assert.NoError(t, manager.WaitUntilReady(context.Background()))
	assert.NoError(t, manager.LastIndexError())
	assert.Nil(t, manager.Store())
	assert.NoError(t, manager.Close())
}

func TestIndexManagerIndexWorkspaceAlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir, ParallelWorkers: 1})

	// Mark as running
	manager.workspaceIndex.running = true

	// Should return error that index is in progress
	err = manager.IndexWorkspaceContext(context.Background())
	assert.Error(t, err)
}

func TestIndexManagerShouldIgnore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{
		WorkspacePath:  tmpDir,
		IgnorePatterns: []string{"*.tmp", "vendor"},
	})

	assert.True(t, manager.shouldIgnore("/path/file.tmp"))
	assert.True(t, manager.shouldIgnore("/path/vendor/file.go"))
	assert.False(t, manager.shouldIgnore("/path/file.go"))
}

func TestIndexManagerIndexFileConcurrentAccess(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))

	// First index
	require.NoError(t, manager.IndexFile(path))

	// Second index should be idempotent (hash check)
	require.NoError(t, manager.IndexFile(path))
}

func TestIndexManagerRegisterParserNil(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	// Should not panic
	manager.RegisterParser(nil)
}

func TestIndexManagerUseSymbolProviderNil(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	// Should not panic
	manager.UseSymbolProvider(nil)
}

func TestIndexManagerRefreshFilesEmpty(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	// Should not error with empty paths
	assert.NoError(t, manager.RefreshFiles([]string{}))
	assert.NoError(t, manager.RefreshFiles(nil))
}

func TestIndexManagerRefreshFilesWhitespace(t *testing.T) {
	manager, _ := newTestIndexManager(t)
	// Should handle whitespace paths
	assert.NoError(t, manager.RefreshFiles([]string{"  ", ""}))
}

func TestIndexManagerSearchNodesWithQuery(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
func World() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	// Search with type filter
	results, err := manager.SearchNodes(NodeQuery{
		Types: []NodeType{NodeTypeFunction},
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestIndexManagerTransactionRollback(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	tx, err := store.BeginTransaction()
	require.NoError(t, err)

	// Rollback should not error
	assert.NoError(t, tx.Rollback())
}

func TestSQLiteStoreNewSQLiteStoreError(t *testing.T) {
	// Invalid path should error
	_, err := NewSQLiteStore("/invalid/path/to/db.sqlite")
	assert.Error(t, err)
}

func TestSQLiteStoreGetFileByPathNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	_, err = store.GetFileByPath("/nonexistent.go")
	assert.Error(t, err)
}

func TestSQLiteStoreGetNodeNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	_, err = store.GetNode("nonexistent")
	assert.Error(t, err)
}

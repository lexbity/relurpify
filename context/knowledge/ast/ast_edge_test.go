package ast

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/fs"
)

const (
	IndexDb_ast_edge_test              = "index.db"
	MainGoFile_ast_edge_test           = "main.go"
	PackagemainfuncHello_ast_edge_test = "package main\nfunc Hello() {}\n"
)

// ==================== IndexManager Edge Cases ====================

func TestIndexManagerStartIndexingWhenReady(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))
	require.NoError(t, manager.IndexWorkspace())
	require.True(t, manager.Ready())

	// Starting indexing when already ready should return nil (no error)
	err := manager.StartIndexing(context.Background())
	require.NoError(t, err)
}

func TestIndexManagerStartIndexingWhenRunning(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a file to index
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))

	// First start indexing
	require.NoError(t, manager.StartIndexing(context.Background()))

	// Starting again while running should return nil (no error)
	err = manager.StartIndexing(context.Background())
	require.NoError(t, err)
}

func TestIndexManagerRefreshFileWithPathFilter(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))
	require.NoError(t, manager.IndexFile(context.Background(), path))

	// Verify file was indexed
	meta, err := manager.Store().GetFileByPath(path)
	require.NoError(t, err)
	require.NotNil(t, meta)

	// Set filter to block the path AFTER indexing
	manager.SetPathFilter(func(path string, isDir bool) bool {
		return false
	})

	// Delete the file
	require.NoError(t, os.Remove(path))

	// Refresh should remove the file because filter blocks it
	err = manager.RefreshFiles(context.Background(), []string{path})
	require.NoError(t, err)

	// File should be removed from index
	file, err := manager.Store().GetFileByPath(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Nil(t, file)
}

func TestIndexManagerRemoveIndexedFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Remove a file that was never indexed - should not error under graphdb
	err = manager.removeIndexedFile(context.Background(), "/nonexistent/path.go")
	require.NoError(t, err)
}

func TestIndexManagerRemoveIndexedFileWithError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))
	require.NoError(t, manager.IndexFile(context.Background(), path))

	// Close the store to cause errors
	_ = store.Close()

	// This should return an error because store is closed
	err = manager.removeIndexedFile(context.Background(), path)
	require.Error(t, err)
}

func TestIndexManagerCloseWithGraphDB(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a mock graph DB (can't easily test actual one without more setup)
	// Just verify Close works without GraphDB
	err = manager.Close(context.Background())
	require.NoError(t, err)
}

func TestIndexManagerLastIndexedAtNotFound(t *testing.T) {
	manager, _ := newTestIndexManager(t)

	// Query for non-existent file - returns no error, zero time
	ts, err := manager.LastIndexedAt("/nonexistent.go")
	require.NoError(t, err)
	assert.True(t, ts.IsZero())
}

func TestIndexManagerPersistErrorCases(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Test persist with nil metadata
	err = manager.persist(context.Background(), &ParseResult{Metadata: nil}, "hash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing metadata")
}

func TestIndexManagerIndexFileReadError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Try to index a non-existent file
	err = manager.IndexFile(context.Background(), "/nonexistent/path.go")
	require.Error(t, err)
}

func TestIndexManagerIndexFileWithConcurrentAccess(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))

	// Lock the indexing map to simulate concurrent access
	manager.mu.Lock()
	manager.indexing[path] = true
	manager.mu.Unlock()

	// This should fail because indexing is "already in progress"
	err := manager.IndexFile(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index already running")

	// Clean up
	manager.mu.Lock()
	delete(manager.indexing, path)
	manager.mu.Unlock()
}

func TestIndexManagerBuildSymbolNodesWithChildren(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	now := time.Now()
	symbols := []DocumentSymbol{
		{
			Name:      "Parent",
			Kind:      NodeTypeFunction,
			StartLine: 1,
			EndLine:   10,
			Children: []DocumentSymbol{
				{
					Name:      "Child",
					Kind:      NodeTypeVariable,
					StartLine: 2,
					EndLine:   5,
				},
			},
		},
	}

	nodes := manager.buildSymbolNodes(symbols, "parent-id", "file-id", CategoryCode, "go", now)
	assert.Len(t, nodes, 2) // Parent + child
}

func TestIndexManagerBuildSymbolNodesWithEmptyKind(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	now := time.Now()
	symbols := []DocumentSymbol{
		{
			Name:      "Test",
			Kind:      "", // Empty kind
			StartLine: 0,  // Invalid line
			EndLine:   -1, // Invalid line
		},
	}

	nodes := manager.buildSymbolNodes(symbols, "parent-id", "file-id", CategoryCode, "go", now)
	assert.Len(t, nodes, 1)
	assert.Equal(t, NodeTypeSection, nodes[0].Type) // Should default to section
	assert.Equal(t, 1, nodes[0].StartLine)          // Should be corrected to 1
	assert.Equal(t, 1, nodes[0].EndLine)            // Should be corrected to start
}

func TestIndexManagerSanitizeSymbolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "symbol"},
		{"Test Function", "test_function"},
		{"path/to/file", "path_to_file"},
		{"path\\to\\file", "path_to_file"},
		{"namespace:value", "namespace_value"},
		{"UPPERCASE", "uppercase"},
	}

	for _, tt := range tests {
		result := sanitizeSymbolName(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestIndexManagerWaitUntilReadyAlreadyReady(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))
	require.NoError(t, manager.IndexWorkspace())

	// Should return immediately since already ready
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := manager.WaitUntilReady(ctx)
	require.NoError(t, err)
}

func TestIndexManagerWaitUntilReadyWithError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Set an error manually
	manager.workspaceIndex.err = errors.New("test error")

	// Should return the error
	err = manager.WaitUntilReady(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test error")
}

func TestIndexManagerIndexWorkspaceContextCanceled(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a file
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.IndexWorkspaceContext(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestIndexManagerGetCallGraphWithMultipleResults(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)

	// Create file with multiple functions
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte("package main\nfunc A() {}\nfunc B() { A() }\nfunc C() { A() }\n")))
	require.NoError(t, manager.IndexFile(context.Background(), path))

	// Get call graph for A
	graph, err := manager.GetCallGraph("A")
	require.NoError(t, err)
	assert.NotNil(t, graph.Root)
	assert.Equal(t, "A", graph.Root.Name)
}

func TestIndexManagerGetCallGraphStoreError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Index a file first
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))
	require.NoError(t, manager.IndexFile(context.Background(), path))

	// Close store to cause errors
	_ = store.Close()

	// Should error because store is closed
	_, err = manager.GetCallGraph("Hello")
	require.Error(t, err)
}

func TestIndexManagerRunWorkspaceIndexWithContextError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a file
	path := filepath.Join(tmpDir, MainGoFile_ast_edge_test)
	require.NoError(t, fs.WriteFileSecure(path, []byte(PackagemainfuncHello_ast_edge_test)))

	// Use canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.runWorkspaceIndex(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestIndexManagerIndexFilesParallelError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{
		WorkspacePath:   tmpDir,
		ParallelWorkers: 2,
	})

	// Create some files with unsupported extension that will cause errors
	files := []string{
		filepath.Join(tmpDir, "test1.py"),
		filepath.Join(tmpDir, "test2.py"),
	}
	for _, path := range files {
		require.NoError(t, fs.WriteFileSecure(path, []byte(`print("hello")`)))
	}

	// Should handle errors from parallel indexing
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.indexFilesParallel(ctx, files)
	require.Error(t, err)
}

func TestLanguageDetectorDetectEmptyPath(t *testing.T) {
	detector := NewLanguageDetector()

	// Empty path
	lang := detector.Detect("")
	assert.Equal(t, "unknown", lang)
}

func TestGoParserParseError(t *testing.T) {
	parser := NewGoParser()

	// Invalid Go code
	_, err := parser.Parse(`invalid go code {{{`, "test.go")
	require.Error(t, err)
}

func TestGoParserParseIncremental(t *testing.T) {
	parser := NewGoParser()

	_, err := parser.ParseIncremental(nil, nil)
	require.Error(t, err)
}

func TestMarkdownParserParseIncremental(t *testing.T) {
	parser := NewMarkdownParser()

	_, err := parser.ParseIncremental(nil, nil)
	require.Error(t, err)
}

func TestIndexManagerIndexFileParseErrorFallbackToSymbols(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	// The .err extension detects as language "unknown"; register the failing
	// parser under that language so the parse-error path is exercised.
	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})
	manager.RegisterParser(&errorParser{language: "unknown"})

	path := filepath.Join(tmpDir, "test.err")
	require.NoError(t, fs.WriteFileSecure(path, []byte(`some content`)))

	// Without a symbol provider the parse error surfaces.
	err = manager.IndexFile(context.Background(), path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse error")

	// With a symbol provider attached, the parse error falls back to
	// symbols and indexing succeeds.
	manager.UseSymbolProvider(&stubSymbolProvider{})
	require.NoError(t, manager.IndexFile(context.Background(), path))
}

func TestIndexManagerIndexFileNoParserSkipsWithoutSymbolProvider(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, IndexDb_ast_edge_test))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	// No parser for yaml and no symbol provider: the file is not an
	// indexable code file — skip it, not an error.
	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})
	path := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, fs.WriteFileSecure(path, []byte("key: value\n")))
	require.NoError(t, manager.IndexFile(context.Background(), path))
}

type errorParser struct {
	language string
}

func (p *errorParser) Parse(content string, filePath string) (*ParseResult, error) {
	return nil, errors.New("parse error")
}

func (p *errorParser) ParseIncremental(oldAST *ParseResult, changes []ContentChange) (*ParseResult, error) {
	return nil, errors.New("incremental not supported")
}

func (p *errorParser) Language() string {
	if p.language == "" {
		return "errlang"
	}
	return p.language
}

func (p *errorParser) Category() Category        { return CategoryCode }
func (p *errorParser) SupportsIncremental() bool { return false }

// stubSymbolProvider returns one section symbol for any file.
type stubSymbolProvider struct{}

func (s *stubSymbolProvider) DocumentSymbols(ctx context.Context, path string) ([]DocumentSymbol, error) {
	return []DocumentSymbol{{Name: "stub", Kind: NodeTypeSection, StartLine: 1, EndLine: 1}}, nil
}

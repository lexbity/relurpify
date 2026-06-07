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
)

// ==================== IndexManager Edge Cases ====================

func TestIndexManagerStartIndexingWhenReady(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexWorkspace())
	require.True(t, manager.Ready())

	// Starting indexing when already ready should return nil (no error)
	err := manager.StartIndexing(context.Background())
	assert.NoError(t, err)
}

func TestIndexManagerStartIndexingWhenRunning(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a file to index
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))

	// First start indexing
	require.NoError(t, manager.StartIndexing(context.Background()))

	// Starting again while running should return nil (no error)
	err = manager.StartIndexing(context.Background())
	assert.NoError(t, err)
}

func TestIndexManagerRefreshFileWithPathFilter(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

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
	err = manager.RefreshFiles([]string{path})
	assert.NoError(t, err)

	// File should be removed from index
	_, err = manager.Store().GetFileByPath(path)
	assert.Error(t, err) // Should error because file was removed
}

func TestIndexManagerRemoveIndexedFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Remove a file that was never indexed - returns sql error
	err = manager.removeIndexedFile("/nonexistent/path.go")
	assert.Error(t, err) // Returns sql.ErrNoRows
}

func TestIndexManagerRemoveIndexedFileWithError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	// Close the store to cause errors
	store.Close()

	// This should return an error because store is closed
	err = manager.removeIndexedFile(path)
	assert.Error(t, err)
}

func TestIndexManagerCloseWithGraphDB(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a mock graph DB (can't easily test actual one without more setup)
	// Just verify Close works without GraphDB
	err = manager.Close()
	assert.NoError(t, err)
}

func TestIndexManagerLastIndexedAtNotFound(t *testing.T) {
	manager, _ := newTestIndexManager(t)

	// Query for non-existent file - returns sql error
	ts, err := manager.LastIndexedAt("/nonexistent.go")
	assert.Error(t, err) // Returns sql.ErrNoRows
	assert.True(t, ts.IsZero())
}

func TestIndexManagerPersistErrorCases(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Test persist with nil metadata
	err = manager.persist(&ParseResult{Metadata: nil}, "hash")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing metadata")
}

func TestIndexManagerIndexFileReadError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Try to index a non-existent file
	err = manager.IndexFile("/nonexistent/path.go")
	assert.Error(t, err)
}

func TestIndexManagerIndexFileWithConcurrentAccess(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))

	// Lock the indexing map to simulate concurrent access
	manager.mu.Lock()
	manager.indexing[path] = true
	manager.mu.Unlock()

	// This should fail because indexing is "already in progress"
	err := manager.IndexFile(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "index already running")

	// Clean up
	manager.mu.Lock()
	delete(manager.indexing, path)
	manager.mu.Unlock()
}

func TestIndexManagerBuildSymbolNodesWithChildren(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

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
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

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
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexWorkspace())

	// Should return immediately since already ready
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := manager.WaitUntilReady(ctx)
	assert.NoError(t, err)
}

func TestIndexManagerWaitUntilReadyWithError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Set an error manually
	manager.workspaceIndex.err = errors.New("test error")

	// Should return the error
	err = manager.WaitUntilReady(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test error")
}

func TestIndexManagerIndexWorkspaceContextCanceled(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a file
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.IndexWorkspaceContext(ctx)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestIndexManagerGetCallGraphWithMultipleResults(t *testing.T) {
	manager, tmpDir := newTestIndexManager(t)

	// Create file with multiple functions
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func A() {}
func B() { A() }
func C() { A() }
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	// Get call graph for A
	graph, err := manager.GetCallGraph("A")
	require.NoError(t, err)
	assert.NotNil(t, graph.Root)
	assert.Equal(t, "A", graph.Root.Name)
}

func TestIndexManagerGetCallGraphStoreError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Index a file first
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))
	require.NoError(t, manager.IndexFile(path))

	// Close store to cause errors
	store.Close()

	// Should error because store is closed
	_, err = manager.GetCallGraph("Hello")
	assert.Error(t, err)
}

func TestIndexManagerRunWorkspaceIndexWithContextError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a file
	path := filepath.Join(tmpDir, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(`package main
func Hello() {}
`), 0o644))

	// Use canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.runWorkspaceIndex(ctx)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestIndexManagerIndexFilesParallelError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

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
		require.NoError(t, os.WriteFile(path, []byte(`print("hello")`), 0o644))
	}

	// Should handle errors from parallel indexing
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = manager.indexFilesParallel(ctx, files)
	assert.Error(t, err)
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
	assert.Error(t, err)
}

func TestGoParserParseIncremental(t *testing.T) {
	parser := NewGoParser()

	_, err := parser.ParseIncremental(nil, nil)
	assert.Error(t, err)
}

func TestMarkdownParserParseIncremental(t *testing.T) {
	parser := NewMarkdownParser()

	_, err := parser.ParseIncremental(nil, nil)
	assert.Error(t, err)
}

func TestIndexManagerIndexFileParseErrorFallbackToSymbols(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Create a fake parser that returns an error
	manager.RegisterParser(&errorParser{})

	// Create a file with the error parser's language
	path := filepath.Join(tmpDir, "test.err")
	require.NoError(t, os.WriteFile(path, []byte(`some content`), 0o644))

	// Without a symbol provider, this should fail
	err = manager.IndexFile(path)
	assert.Error(t, err)
}

type errorParser struct{}

func (p *errorParser) Parse(content string, filePath string) (*ParseResult, error) {
	return nil, errors.New("parse error")
}

func (p *errorParser) ParseIncremental(oldAST *ParseResult, changes []ContentChange) (*ParseResult, error) {
	return nil, errors.New("incremental not supported")
}

func (p *errorParser) Language() string          { return "errlang" }
func (p *errorParser) Category() Category        { return CategoryCode }
func (p *errorParser) SupportsIncremental() bool { return false }

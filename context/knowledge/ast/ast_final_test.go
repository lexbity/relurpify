package ast

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== Persist and Transaction Tests ====================

func TestIndexManagerPersistWithTransactionError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Close the store to cause transaction errors
	store.Close()

	now := time.Now()
	fileID := GenerateFileID("/test.go")
	result := &ParseResult{
		RootNode: &Node{ID: fileID + ":root", FileID: fileID, Type: NodeTypePackage, Name: "main", CreatedAt: now, UpdatedAt: now},
		Nodes: []*Node{
			{ID: fileID + ":root", FileID: fileID, Type: NodeTypePackage, Name: "main", CreatedAt: now, UpdatedAt: now},
		},
		Edges: []*Edge{},
		Metadata: &FileMetadata{
			ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
			ContentHash: "hash", IndexedAt: now,
		},
	}

	err = manager.persist(result, "hash")
	assert.Error(t, err)
}

func TestIndexManagerPersistWithContentHash(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	now := time.Now()
	fileID := GenerateFileID("/test.go")
	result := &ParseResult{
		RootNode: &Node{ID: fileID + ":root", FileID: fileID, Type: NodeTypePackage, Name: "main", CreatedAt: now, UpdatedAt: now},
		Nodes: []*Node{
			{ID: fileID + ":root", FileID: fileID, Type: NodeTypePackage, Name: "main", CreatedAt: now, UpdatedAt: now},
		},
		Edges: []*Edge{},
		Metadata: &FileMetadata{
			ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
			ContentHash: "", IndexedAt: now, // Empty hash
		},
	}

	// Should set content hash if empty
	err = manager.persist(result, "newhash")
	require.NoError(t, err)
	assert.Equal(t, "newhash", result.Metadata.ContentHash)
}

// ==================== Go Parser Additional Tests ====================

func TestGoParserBuildGenDeclNodesWithInterface(t *testing.T) {
	source := `package sample
type Reader interface {
	Read(p []byte) (n int, err error)
}`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	require.NoError(t, err)

	// Should have package node and interface node
	var foundInterface bool
	for _, node := range result.Nodes {
		if node.Type == NodeTypeInterface {
			foundInterface = true
			assert.Equal(t, "Reader", node.Name)
		}
	}
	assert.True(t, foundInterface)
}

func TestGoParserBuildGenDeclNodesWithTypeAlias(t *testing.T) {
	source := `package sample
type MyString = string`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	require.NoError(t, err)

	// Should have package node and type alias node
	var foundType bool
	for _, node := range result.Nodes {
		if node.Type == NodeTypeType {
			foundType = true
		}
	}
	assert.True(t, foundType)
}

func TestGoParserSignatureWithMultipleResults(t *testing.T) {
	source := `package sample
func Multi() (int, string, error) {
	return 0, "", nil
}`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	require.NoError(t, err)

	var found bool
	for _, node := range result.Nodes {
		if node.Type == NodeTypeFunction && node.Name == "Multi" {
			found = true
			assert.Contains(t, node.Signature, "(int, string, error)")
		}
	}
	assert.True(t, found)
}

func TestGoParserSignatureWithNamedResults(t *testing.T) {
	source := `package sample
func Named() (x int, y string) {
	return 0, ""
}`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	require.NoError(t, err)

	var found bool
	for _, node := range result.Nodes {
		if node.Type == NodeTypeFunction && node.Name == "Named" {
			found = true
			assert.Contains(t, node.Signature, "x int, y string")
		}
	}
	assert.True(t, found)
}

func TestGoParserDocStringEmpty(t *testing.T) {
	source := `package sample
func NoDocs() {}`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	require.NoError(t, err)

	var found bool
	for _, node := range result.Nodes {
		if node.Type == NodeTypeFunction && node.Name == "NoDocs" {
			found = true
			assert.Empty(t, node.DocString)
		}
	}
	assert.True(t, found)
}

func TestGoParserMethodWithReceiver(t *testing.T) {
	source := `package sample
type MyStruct struct {}
func (m MyStruct) Method() {}
func (m *MyStruct) PtrMethod() {}`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	require.NoError(t, err)

	var methodCount int
	for _, node := range result.Nodes {
		if node.Type == NodeTypeMethod {
			methodCount++
			assert.NotNil(t, node.Attributes["receiver"])
		}
	}
	assert.Equal(t, 2, methodCount)
}

func TestGoParserCollectCallEdgesEmptyBody(t *testing.T) {
	source := `package sample
func External() // declared but not defined`
	parser := NewGoParser()
	result, err := parser.Parse(source, "sample.go")
	require.NoError(t, err)

	var found bool
	for _, node := range result.Nodes {
		if node.Type == NodeTypeFunction && node.Name == "External" {
			found = true
		}
	}
	assert.True(t, found)
}

// ==================== SQLiteStore Additional Tests ====================

func TestSQLiteStoreSaveNodesWithNilNode(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
		ContentHash: "hash", IndexedAt: time.Now(),
	}))

	// Save with a nil node in the slice
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		nil, // nil node should be skipped
	}
	require.NoError(t, store.SaveNodes(nodes))

	// Verify only n1 was saved
	node, err := store.GetNode("n1")
	require.NoError(t, err)
	assert.Equal(t, "Hello", node.Name)
}

func TestSQLiteStoreSaveEdgesWithNilEdge(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
		ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// Save with a nil edge in the slice
	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls},
		nil, // nil edge should be skipped
	}
	require.NoError(t, store.SaveEdges(edges))

	// Verify only e1 was saved
	edge, err := store.GetEdge("e1")
	require.NoError(t, err)
	assert.Equal(t, EdgeTypeCalls, edge.Type)
}

func TestSQLiteStoreInsertNodesWithEmptySlice(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	// Should not error with empty slice
	err = store.SaveNodes([]*Node{})
	assert.NoError(t, err)
}

func TestSQLiteStoreInsertEdgesWithEmptySlice(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	// Should not error with empty slice
	err = store.SaveEdges([]*Edge{})
	assert.NoError(t, err)
}

func TestSQLiteStoreSearchNodesWithExportedFilter(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
		ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	exported := true
	notExported := false
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", IsExported: true, CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "world", Category: CategoryCode, Language: "go", IsExported: false, CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// Search for exported nodes
	results, err := store.SearchNodes(NodeQuery{IsExported: &exported})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Hello", results[0].Name)

	// Search for non-exported nodes
	results, err = store.SearchNodes(NodeQuery{IsExported: &notExported})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "world", results[0].Name)
}

func TestSQLiteStoreSearchNodesWithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
		ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "A", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "B", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n3", FileID: fileID, Type: NodeTypeFunction, Name: "C", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// Search with offset
	results, err := store.SearchNodes(NodeQuery{Offset: 1, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, results, 2) // Should skip first node
}

func TestSQLiteStoreGetStatsWithData(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
		ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeStruct, Name: "World", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	edges := []*Edge{
		{ID: "e1", SourceID: "n1", TargetID: "n2", Type: EdgeTypeCalls},
	}
	require.NoError(t, store.SaveEdges(edges))

	stats, err := store.GetStats()
	require.NoError(t, err)
	assert.Equal(t, 1, stats.TotalFiles)
	assert.Equal(t, 2, stats.TotalNodes)
	assert.Equal(t, 1, stats.TotalEdges)
	assert.Greater(t, len(stats.NodesByType), 0)
	assert.Greater(t, len(stats.EdgesByType), 0)
}

// ==================== Graph Schema Additional Tests ====================

func TestGraphNodeRecordWithInvalidMarshal(t *testing.T) {
	// Test with a node that has problematic attributes
	node := &Node{
		ID:         "test",
		Type:       NodeTypeFunction,
		Name:       "Test",
		Category:   CategoryCode,
		Language:   "go",
		StartLine:  1,
		EndLine:    10,
		IsExported: true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Attributes: map[string]interface{}{
			"invalid": make(chan int), // channels can't be marshaled
		},
	}

	record, ok := graphNodeRecord(node, "/test.go")
	// Should still succeed, props may contain partial data or be empty depending on implementation
	assert.True(t, ok)
	// Props could be nil or valid JSON (without the invalid field)
	// Just verify the record was created successfully
	assert.Equal(t, "test", record.ID)
}

func TestGraphEdgeRecordsWithProps(t *testing.T) {
	records, err := graphEdgeRecords("src", "dst", "calls", "called_by", 1.0, map[string]any{
		"line": 42,
	})
	require.NoError(t, err)
	assert.Len(t, records, 2)
	assert.NotEmpty(t, records[0].Props)
	assert.NotEmpty(t, records[1].Props)
}

// ==================== Parser Registry Tests ====================

func TestParserRegistryRegisterNil(t *testing.T) {
	registry := NewParserRegistry()

	// Should not panic
	registry.Register(nil)

	// Should have no parsers
	langs := registry.SupportedLanguages()
	assert.Empty(t, langs)
}

func TestParserRegistryGetParserNotFound(t *testing.T) {
	registry := NewParserRegistry()

	parser, ok := registry.GetParser("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, parser)
}

// ==================== IndexManager Close Tests ====================

func TestIndexManagerCloseWhileRunning(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Set as running without a ready channel
	manager.workspaceIndex.running = true
	manager.workspaceIndex.readyCh = make(chan struct{})

	// Close in background
	done := make(chan error)
	go func() {
		done <- manager.Close()
	}()

	// Close the ready channel to unblock
	close(manager.workspaceIndex.readyCh)

	err = <-done
	assert.NoError(t, err)
}

// ==================== Language Detector Additional Tests ====================

func TestLanguageDetectorDetectFilename(t *testing.T) {
	detector := NewLanguageDetector()

	// Dockerfile
	assert.Equal(t, "docker", detector.Detect("Dockerfile"))

	// docker-compose.yml
	assert.Equal(t, "docker-compose", detector.Detect("docker-compose.yml"))

	// Unknown extension
	assert.Equal(t, "unknown", detector.Detect("/path/file.unknownext"))
}

// ==================== Markdown Parser Additional Tests ====================

func TestMarkdownParserParseEmpty(t *testing.T) {
	parser := NewMarkdownParser()
	result, err := parser.Parse("", "empty.md")
	require.NoError(t, err)
	assert.NotNil(t, result.RootNode)
	assert.Equal(t, NodeTypeDocument, result.RootNode.Type)
}

func TestMarkdownParserParseWithMalformedCodeBlock(t *testing.T) {
	// Code block that's not properly closed
	content := "# Title\n\n```go\nunclosed code"
	parser := NewMarkdownParser()
	result, err := parser.Parse(content, "test.md")
	require.NoError(t, err)
	assert.NotNil(t, result.RootNode)
}

// ==================== Utility Function Tests ====================

func TestGenerateFileIDUnique(t *testing.T) {
	id1 := GenerateFileID("/path/to/file1.go")
	id2 := GenerateFileID("/path/to/file2.go")
	id3 := GenerateFileID("/path/to/file1.go") // Same as id1

	assert.NotEqual(t, id1, id2)
	assert.Equal(t, id1, id3)
}

func TestHashContentConsistent(t *testing.T) {
	hash1 := HashContent("test content")
	hash2 := HashContent("test content")
	hash3 := HashContent("different content")

	assert.Equal(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
}

// ==================== IndexManager BeginTransaction Test ====================

func TestSQLiteStoreBeginTransactionError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// Should error because db is closed
	_, err = store.BeginTransaction()
	assert.Error(t, err)
}

// ==================== SQLiteStore SaveFile Error ====================

func TestSQLiteStoreSaveFileNil(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	err = store.SaveFile(nil)
	assert.Error(t, err)
}

// ==================== SQLiteStore Close Nil DB ====================

func TestSQLiteStoreCloseNil(t *testing.T) {
	store := &SQLiteStore{db: nil}
	err := store.Close()
	assert.NoError(t, err)
}

// ==================== scanFiles Error Path ====================

func TestSQLiteStoreScanFilesError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// This should error because db is closed
	_, err = store.ListFiles(CategoryCode)
	assert.Error(t, err)
}

// ==================== scanFile Error Path ====================

func TestSQLiteStoreScanFileError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// This should error because db is closed
	_, err = store.GetFile("nonexistent")
	assert.Error(t, err)
}

// ==================== scanNode Error Path ====================

func TestSQLiteStoreScanNodeError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// This should error because db is closed
	_, err = store.GetNode("nonexistent")
	assert.Error(t, err)
}

// ==================== scanNodes Error Path ====================

func TestSQLiteStoreScanNodesError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// This should error because db is closed
	_, err = store.GetNodesByType(NodeTypeFunction)
	assert.Error(t, err)
}

// ==================== scanEdge Error Path ====================

func TestSQLiteStoreScanEdgeError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// This should error because db is closed
	_, err = store.GetEdge("nonexistent")
	assert.Error(t, err)
}

// ==================== scanEdges Error Path ====================

func TestSQLiteStoreScanEdgesError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// This should error because db is closed
	_, err = store.GetEdgesByType(EdgeTypeCalls)
	assert.Error(t, err)
}

// ==================== placeholders Edge Cases ====================

func TestPlaceholdersZeroAndNegative(t *testing.T) {
	// Test with 0
	result := placeholders(0)
	assert.Equal(t, "", result)

	// Test with negative (shouldn't happen but check robustness)
	result = placeholders(-1)
	assert.Equal(t, "", result)
}

// ==================== getRelatedNodes Error Path ====================

func TestSQLiteStoreGetRelatedNodesError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	store.Close()

	// This should error because db is closed
	_, err = store.getRelatedNodes("node1", EdgeTypeCalls, true)
	assert.Error(t, err)
}

// ==================== SearchNodes Query Builder ====================

func TestSQLiteStoreSearchNodesWithMultipleFilters(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
		ContentHash: "hash", IndexedAt: time.Now(),
	}))

	now := time.Now()
	nodes := []*Node{
		{ID: "n1", FileID: fileID, Type: NodeTypeFunction, Name: "Hello", Category: CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "n2", FileID: fileID, Type: NodeTypeFunction, Name: "World", Category: CategoryCode, Language: "python", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.SaveNodes(nodes))

	// Search with multiple filters
	results, err := store.SearchNodes(NodeQuery{
		Types:       []NodeType{NodeTypeFunction},
		Categories:  []Category{CategoryCode},
		Languages:   []string{"go"},
		FileIDs:     []string{fileID},
		NamePattern: "Hel%",
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Hello", results[0].Name)
}

// ==================== SearchEdges Query Builder ====================

func TestSQLiteStoreSearchEdgesWithMultipleFilters(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	fileID := GenerateFileID("/test.go")
	require.NoError(t, store.SaveFile(&FileMetadata{
		ID: fileID, Path: "/test.go", Language: "go", Category: CategoryCode,
		ContentHash: "hash", IndexedAt: time.Now(),
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
	}
	require.NoError(t, store.SaveEdges(edges))

	// Search with multiple filters
	results, err := store.SearchEdges(EdgeQuery{
		Types:     []EdgeType{EdgeTypeCalls},
		SourceIDs: []string{"n1"},
		TargetIDs: []string{"n2"},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "e1", results[0].ID)
}

// ==================== Test sql.ErrNoRows handling ====================

func TestGetFileByPathNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	_, err = store.GetFileByPath("/nonexistent.go")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestGetEdgeNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer store.Close()

	_, err = store.GetEdge("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

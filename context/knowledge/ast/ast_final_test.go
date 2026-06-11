package ast

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== Persist and Transaction Tests ====================

func TestIndexManagerPersistWithTransactionError(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Close the store to cause transaction errors
	_ = store.Close()

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

	err = manager.persist(context.Background(), result, "hash")
	assert.Error(t, err)
}

func TestIndexManagerPersistWithContentHash(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

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
	err = manager.persist(context.Background(), result, "newhash")
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
		Attributes: map[string]any{
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
	store, err := NewTestStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)

	manager := NewIndexManager(store, IndexConfig{WorkspacePath: tmpDir})

	// Set as running without a ready channel
	manager.workspaceIndex.running = true
	manager.workspaceIndex.readyCh = make(chan struct{})

	// Close in background
	done := make(chan error)
	go func() {
		done <- manager.Close(context.Background())
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

// ==================== Test not found handling ====================

func TestGetFileByPathNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	file, err := store.GetFileByPath("/nonexistent.go")
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Nil(t, file)
}

func TestGetEdgeNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTestStore(filepath.Join(tmpDir, "index.db"))
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	edge, err := store.GetEdge("nonexistent")
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Nil(t, edge)
}

package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

// TestTextIngestion validates that text content can be ingested
// and stored as chunks with stable identifiers.
func TestTextIngestion(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Ingest a simple text chunk
	ctx := context.Background()
	resp := &model.LLMResponse{
		Text: "This is test content for ingestion.",
	}

	chunk, err := ingester.IngestLLMResponseFull(ctx, resp)
	if err != nil {
		t.Fatalf("failed to ingest text: %v", err)
	}

	// Assert chunk was created with required fields
	if chunk == nil {
		t.Fatal("expected chunk to be created, got nil")
	}
	if chunk.ID == "" {
		t.Error("expected chunk ID to be set")
	}
	if chunk.Body.Raw != resp.Text {
		t.Errorf("expected chunk content %q, got %q", resp.Text, chunk.Body.Raw)
	}
	if chunk.ContentHash == "" {
		t.Error("expected content hash to be set")
	}
	if chunk.TokenEstimate == 0 {
		t.Error("expected token estimate to be set")
	}
	if chunk.SourceOrigin != knowledge.SourceOriginLLM {
		t.Errorf("expected source origin %s, got %s", knowledge.SourceOriginLLM, chunk.SourceOrigin)
	}
	if chunk.MemoryClass != knowledge.MemoryClassStreamed {
		t.Errorf("expected memory class %s, got %s", knowledge.MemoryClassStreamed, chunk.MemoryClass)
	}
	if chunk.StorageMode != knowledge.StorageModeSummarized {
		t.Errorf("expected storage mode %s, got %s", knowledge.StorageModeSummarized, chunk.StorageMode)
	}
	if chunk.TrustClass != "llm-generated" {
		t.Errorf("expected trust class 'llm-generated' for LLM response, got %s", chunk.TrustClass)
	}
	if chunk.Freshness != knowledge.FreshnessValid {
		t.Errorf("expected freshness %s, got %s", knowledge.FreshnessValid, chunk.Freshness)
	}

	// Verify chunk can be retrieved from store
	retrieved, ok, err := store.Load(chunk.ID)
	if err != nil {
		t.Fatalf("failed to load chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected chunk to be found in store")
	}
	if retrieved.ID != chunk.ID {
		t.Errorf("retrieved chunk ID mismatch: %s vs %s", retrieved.ID, chunk.ID)
	}
	if retrieved.Body.Raw != chunk.Body.Raw {
		t.Errorf("retrieved chunk content mismatch")
	}
}

// TestMetadataPropagation validates that metadata survives the ingestion pipeline.
func TestMetadataPropagation(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Create a test file with known metadata
	testPath := filepath.Join(env.WorkspacePath, "test.txt")
	testContent := "Test file content for metadata propagation."
	if err := fs.WriteFileSecure(testPath, []byte(testContent)); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Get file metadata
	fileInfo, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	// Ingest as tool result with metadata
	ctx := context.Background()
	chunk, err := ingester.IngestToolResult(ctx, "read_file", []byte(testContent))
	if err != nil {
		t.Fatalf("failed to ingest tool result: %v", err)
	}

	// Assert metadata fields are present
	if chunk == nil {
		t.Fatal("expected chunk to be created, got nil")
	}
	if chunk.Body.Fields == nil {
		t.Fatal("expected chunk fields to be set")
	}

	// Verify tool name is in metadata
	toolName, ok := chunk.Body.Fields["tool_name"].(string)
	if !ok {
		t.Error("expected tool_name in chunk fields")
	}
	if toolName != "read_file" {
		t.Errorf("expected tool_name 'read_file', got %s", toolName)
	}

	// Verify raw bytes size is in metadata
	rawBytes, ok := chunk.Body.Fields["raw_bytes"].(int)
	if !ok {
		t.Error("expected raw_bytes in chunk fields")
	}
	if rawBytes != len(testContent) {
		t.Errorf("expected raw_bytes %d, got %d", len(testContent), rawBytes)
	}

	// Verify source origin is tool
	if chunk.SourceOrigin != knowledge.SourceOriginTool {
		t.Errorf("expected source origin %s, got %s", knowledge.SourceOriginTool, chunk.SourceOrigin)
	}

	// Verify memory class is working
	if chunk.MemoryClass != knowledge.MemoryClassWorking {
		t.Errorf("expected memory class %s, got %s", knowledge.MemoryClassWorking, chunk.MemoryClass)
	}

	// Verify trust class is tool result
	if chunk.TrustClass != "tool-result" {
		t.Errorf("expected trust class 'tool-result' for tool result, got %s", chunk.TrustClass)
	}

	// Add file path metadata to chunk
	chunk.Body.Fields["file_path"] = testPath
	chunk.Body.Fields["file_size"] = fileInfo.Size()
	chunk.Body.Fields["file_extension"] = ".txt"

	// Save chunk with metadata
	saved, err := store.Save(context.TODO(), *chunk)
	if err != nil {
		t.Fatalf("failed to save chunk with metadata: %v", err)
	}

	// Verify metadata survives round-trip
	retrieved, ok, err := store.Load(saved.ID)
	if err != nil {
		t.Fatalf("failed to load chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected chunk to be found in store")
	}

	// Verify file path metadata
	filePath, ok := retrieved.Body.Fields["file_path"].(string)
	if !ok {
		t.Error("expected file_path to survive round-trip")
	}
	if filePath != testPath {
		t.Errorf("file_path mismatch: %s vs %s", testPath, filePath)
	}

	// Verify file size metadata
	fileSize, ok := retrieved.Body.Fields["file_size"].(int64)
	if !ok {
		// Try int as fallback
		if fileSizeInt, ok := retrieved.Body.Fields["file_size"].(int); ok {
			fileSize = int64(fileSizeInt)
		} else if fileSizeFloat, ok := retrieved.Body.Fields["file_size"].(float64); ok {
			fileSize = int64(fileSizeFloat)
		} else {
			t.Error("expected file_size to survive round-trip")
		}
	}
	if fileSize != fileInfo.Size() {
		t.Errorf("file_size mismatch: %d vs %d", fileInfo.Size(), fileSize)
	}

	// Verify file extension metadata
	fileExt, ok := retrieved.Body.Fields["file_extension"].(string)
	if !ok {
		t.Error("expected file_extension to survive round-trip")
	}
	if fileExt != ".txt" {
		t.Errorf("file_extension mismatch: .txt vs %s", fileExt)
	}
}

// TestChunkSourceRetention validates that source information
// is retained through the ingestion pipeline.
func TestChunkSourceRetention(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Create a context with envelope for source tracking
	sessionID := "test-session"
	workflowID := "test-workflow"
	nodeID := "test-node"

	envCtx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", sessionID)
	envelope.NodeID = nodeID
	envelope.SetWorkingValueWithClass("workflow.id", workflowID, contextdata.MemoryClassTask)
	envCtx = contextdata.WithEnvelope(envCtx, envelope)

	// Ingest with source context
	ctx := contextdata.WithEnvelope(envCtx, envelope)
	resp := &model.LLMResponse{
		Text: "Content derived from sources.",
	}

	chunk, err := ingester.IngestLLMResponseFull(ctx, resp)
	if err != nil {
		t.Fatalf("failed to ingest text: %v", err)
	}

	// Assert source fields are retained
	if chunk == nil {
		t.Fatal("expected chunk to be created, got nil")
	}

	// Verify workspace ID is set from session
	if chunk.WorkspaceID != sessionID {
		t.Errorf("expected workspace ID %s, got %s", sessionID, chunk.WorkspaceID)
	}

	// Verify provenance session ID
	if chunk.Provenance.SessionID != sessionID {
		t.Errorf("expected provenance session ID %s, got %s", sessionID, chunk.Provenance.SessionID)
	}

	// Verify provenance workflow ID
	if chunk.Provenance.WorkflowID != workflowID {
		t.Errorf("expected provenance workflow ID %s, got %s", workflowID, chunk.Provenance.WorkflowID)
	}

	// Verify provenance timestamp is set
	if chunk.Provenance.Timestamp.IsZero() {
		t.Error("expected provenance timestamp to be set")
	}

	// Verify acquisition method
	if chunk.AcquisitionMethod != knowledge.AcquisitionMethodRuntimeWrite {
		t.Errorf("expected acquisition method %s, got %s", knowledge.AcquisitionMethodRuntimeWrite, chunk.AcquisitionMethod)
	}

	// Verify acquired at timestamp
	if chunk.AcquiredAt.IsZero() {
		t.Error("expected acquired at timestamp to be set")
	}

	// Verify compiled by
	if chunk.Provenance.CompiledBy != knowledge.CompilerLLMAssisted {
		t.Errorf("expected compiled by %s, got %s", knowledge.CompilerLLMAssisted, chunk.Provenance.CompiledBy)
	}

	// Verify provenance timestamp
	if chunk.Provenance.Timestamp.IsZero() {
		t.Error("expected provenance timestamp to be set")
	}

	// Save and verify persistence
	saved, err := store.Save(context.TODO(), *chunk)
	if err != nil {
		t.Fatalf("failed to save chunk: %v", err)
	}

	retrieved, ok, err := store.Load(saved.ID)
	if err != nil {
		t.Fatalf("failed to load chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected chunk to be found in store")
	}

	// Verify source information survived persistence
	if retrieved.WorkspaceID != sessionID {
		t.Error("workspace ID did not survive persistence")
	}
	if retrieved.Provenance.SessionID != sessionID {
		t.Error("provenance session ID did not survive persistence")
	}
	if retrieved.Provenance.WorkflowID != workflowID {
		t.Error("provenance workflow ID did not survive persistence")
	}
}

// TestRepeatIngestionConsistency validates that ingesting the same
// content twice produces consistent results.
func TestRepeatIngestionConsistency(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Ingest the same content twice
	ctx := context.Background()
	content := "Consistent test content for repeat ingestion."
	resp1 := &model.LLMResponse{Text: content}
	resp2 := &model.LLMResponse{Text: content}

	chunk1, err := ingester.IngestLLMResponseFull(ctx, resp1)
	if err != nil {
		t.Fatalf("first ingestion failed: %v", err)
	}

	chunk2, err := ingester.IngestLLMResponseFull(ctx, resp2)
	if err != nil {
		t.Fatalf("second ingestion failed: %v", err)
	}

	// Assert chunks have the same ID (content-based deduplication)
	if chunk1.ID != chunk2.ID {
		t.Errorf("expected same chunk ID for identical content, got %s vs %s", chunk1.ID, chunk2.ID)
	}

	// Assert content hash is identical
	if chunk1.ContentHash != chunk2.ContentHash {
		t.Errorf("expected same content hash, got %s vs %s", chunk1.ContentHash, chunk2.ContentHash)
	}

	// Assert raw content is identical
	if chunk1.Body.Raw != chunk2.Body.Raw {
		t.Errorf("expected same raw content, got %q vs %q", chunk1.Body.Raw, chunk2.Body.Raw)
	}

	// Assert token estimate is consistent
	if chunk1.TokenEstimate != chunk2.TokenEstimate {
		t.Errorf("expected same token estimate, got %d vs %d", chunk1.TokenEstimate, chunk2.TokenEstimate)
	}

	// Assert source origin is consistent
	if chunk1.SourceOrigin != chunk2.SourceOrigin {
		t.Errorf("expected same source origin, got %s vs %s", chunk1.SourceOrigin, chunk2.SourceOrigin)
	}

	// Assert memory class is consistent
	if chunk1.MemoryClass != chunk2.MemoryClass {
		t.Errorf("expected same memory class, got %s vs %s", chunk1.MemoryClass, chunk2.MemoryClass)
	}

	// Assert storage mode is consistent
	if chunk1.StorageMode != chunk2.StorageMode {
		t.Errorf("expected same storage mode, got %s vs %s", chunk1.StorageMode, chunk2.StorageMode)
	}

	// Verify only one chunk exists in store (deduplication)
	chunks, err := store.FindByContentHash(chunk1.ContentHash)
	if err != nil {
		t.Fatalf("failed to query by content hash: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk in store after duplicate ingestion, got %d", len(chunks))
	}
}

// TestIngestionWithDifferentContent validates that different content
// produces different chunks.
func TestIngestionWithDifferentContent(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Ingest different content
	ctx := context.Background()
	resp1 := &model.LLMResponse{Text: "First content."}
	resp2 := &model.LLMResponse{Text: "Second content."}

	chunk1, err := ingester.IngestLLMResponseFull(ctx, resp1)
	if err != nil {
		t.Fatalf("first ingestion failed: %v", err)
	}

	chunk2, err := ingester.IngestLLMResponseFull(ctx, resp2)
	if err != nil {
		t.Fatalf("second ingestion failed: %v", err)
	}

	// Assert chunks have different IDs
	if chunk1.ID == chunk2.ID {
		t.Error("expected different chunk IDs for different content")
	}

	// Assert content hashes are different
	if chunk1.ContentHash == chunk2.ContentHash {
		t.Error("expected different content hashes for different content")
	}

	// Assert raw content is different
	if chunk1.Body.Raw == chunk2.Body.Raw {
		t.Error("expected different raw content")
	}

	// Verify both chunks exist in store
	chunks1, err := store.FindByContentHash(chunk1.ContentHash)
	if err != nil {
		t.Fatalf("failed to query first content hash: %v", err)
	}
	if len(chunks1) != 1 {
		t.Errorf("expected 1 chunk for first content, got %d", len(chunks1))
	}

	chunks2, err := store.FindByContentHash(chunk2.ContentHash)
	if err != nil {
		t.Fatalf("failed to query second content hash: %v", err)
	}
	if len(chunks2) != 1 {
		t.Errorf("expected 1 chunk for second content, got %d", len(chunks2))
	}
}

// TestToolResultIngestion validates that tool results are ingested
// with appropriate metadata and storage mode.
func TestToolResultIngestion(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Ingest a tool result
	ctx := context.Background()
	toolName := "execute_command"
	result := []byte("Command output: success")

	chunk, err := ingester.IngestToolResult(ctx, toolName, result)
	if err != nil {
		t.Fatalf("failed to ingest tool result: %v", err)
	}

	// Assert chunk was created with tool-specific metadata
	if chunk == nil {
		t.Fatal("expected chunk to be created, got nil")
	}
	if chunk.SourceOrigin != knowledge.SourceOriginTool {
		t.Errorf("expected source origin %s, got %s", knowledge.SourceOriginTool, chunk.SourceOrigin)
	}
	if chunk.MemoryClass != knowledge.MemoryClassWorking {
		t.Errorf("expected memory class %s, got %s", knowledge.MemoryClassWorking, chunk.MemoryClass)
	}
	if chunk.Body.Raw != string(result) {
		t.Errorf("expected raw content %q, got %q", string(result), chunk.Body.Raw)
	}

	// Verify tool metadata
	toolNameField, ok := chunk.Body.Fields["tool_name"].(string)
	if !ok {
		t.Error("expected tool_name in chunk fields")
	}
	if toolNameField != toolName {
		t.Errorf("expected tool_name %s, got %s", toolName, toolNameField)
	}

	rawBytesField, ok := chunk.Body.Fields["raw_bytes"].(int)
	if !ok {
		t.Error("expected raw_bytes in chunk fields")
	}
	if rawBytesField != len(result) {
		t.Errorf("expected raw_bytes %d, got %d", len(result), rawBytesField)
	}

	// Verify storage mode based on size (small = inline)
	if len(result) <= 8192 && chunk.StorageMode != knowledge.StorageModeInline {
		t.Errorf("expected storage mode %s for small result, got %s", knowledge.StorageModeInline, chunk.StorageMode)
	}
}

// TestObservationIngestion validates that observations are ingested
// with appropriate metadata.
func TestObservationIngestion(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Ingest an observation
	ctx := context.Background()
	observation := "User performed action X"

	chunk, err := ingester.IngestObservation(ctx, observation)
	if err != nil {
		t.Fatalf("failed to ingest observation: %v", err)
	}

	// Assert chunk was created with observation-specific metadata
	if chunk == nil {
		t.Fatal("expected chunk to be created, got nil")
	}
	if chunk.SourceOrigin != knowledge.SourceOriginDerivation {
		t.Errorf("expected source origin %s, got %s", knowledge.SourceOriginDerivation, chunk.SourceOrigin)
	}
	if chunk.MemoryClass != knowledge.MemoryClassStreamed {
		t.Errorf("expected memory class %s, got %s", knowledge.MemoryClassStreamed, chunk.MemoryClass)
	}
	if chunk.Body.Raw != observation {
		t.Errorf("expected raw content %q, got %q", observation, chunk.Body.Raw)
	}

	// Verify observation metadata
	observationField, ok := chunk.Body.Fields["observation"].(string)
	if !ok {
		t.Error("expected observation in chunk fields")
	}
	if observationField != observation {
		t.Errorf("expected observation %s, got %s", observation, observationField)
	}
}

// TestChunkVersionIncrement validates that updating a chunk
// increments its version.
func TestChunkVersionIncrement(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close(context.Background())

	store := &knowledge.ChunkStore{Graph: graph}

	// Create and save a chunk
	chunk := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("version-test"),
		WorkspaceID: "test-workspace",
		Body:        knowledge.ChunkBody{Raw: "initial content"},
		Provenance: knowledge.ChunkProvenance{
			Sources:   []knowledge.ProvenanceSource{{Kind: "user", Ref: "ref"}},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}

	saved1, err := store.Save(context.TODO(), chunk)
	if err != nil {
		t.Fatalf("failed to save initial chunk: %v", err)
	}

	// Update the chunk
	saved1.Body.Raw = "updated content"
	saved2, err := store.Save(context.TODO(), *saved1)
	if err != nil {
		t.Fatalf("failed to save updated chunk: %v", err)
	}

	// Assert version incremented
	if saved2.Version <= saved1.Version {
		t.Errorf("expected version increment, got %d -> %d", saved1.Version, saved2.Version)
	}

	// Verify updated content
	retrieved, ok, err := store.Load(saved2.ID)
	if err != nil {
		t.Fatalf("failed to load chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected chunk to be found in store")
	}
	if retrieved.Body.Raw != "updated content" {
		t.Errorf("expected updated content, got %s", retrieved.Body.Raw)
	}
}

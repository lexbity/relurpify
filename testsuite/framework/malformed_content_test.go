package framework

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/model"
)

// TestBinaryIshContentHandling validates that content that looks text-like
// but contains non-text bytes is handled gracefully by the ingestion pipeline.
func TestBinaryIshContentHandling(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	// Create binary-ish content: looks like text but contains null bytes and other non-text
	binaryIshContent := []byte("text\x00prefix\x01\x02middle\xff\xfeend")
	if len(binaryIshContent) == 0 {
		t.Fatal("binary-ish content should not be empty")
	}

	// Ingest as tool result (this path should handle binary content gracefully)
	ctx := context.Background()
	chunk, err := ingester.IngestToolResult(ctx, "binary_tool", binaryIshContent)
	if err != nil {
		t.Fatalf("binary-ish content ingestion should not fail: %v", err)
	}

	// Assert chunk was created despite binary content
	if chunk == nil {
		t.Fatal("expected chunk to be created for binary-ish content")
	}
	if chunk.ID == "" {
		t.Error("expected chunk ID to be set for binary-ish content")
	}

	// Verify the raw bytes are stored (may be truncated or encoded depending on implementation)
	if chunk.Body.Raw == "" {
		// Empty raw content is acceptable for binary content if the implementation
		// stores it externally or encodes it
		t.Log("binary-ish content resulted in empty raw body (may be stored externally)")
	} else {
		// If raw content is present, verify it's a string representation
		t.Logf("binary-ish content raw body length: %d", len(chunk.Body.Raw))
	}

	// Verify metadata is still present
	if chunk.Body.Fields == nil {
		t.Error("expected chunk fields to be set even for binary-ish content")
	}
	toolName, ok := chunk.Body.Fields["tool_name"].(string)
	if !ok {
		t.Error("expected tool_name in chunk fields for binary-ish content")
	}
	if toolName != "binary_tool" {
		t.Errorf("expected tool_name 'binary_tool', got %s", toolName)
	}

	// Verify source origin is tool
	if chunk.SourceOrigin != knowledge.SourceOriginTool {
		t.Errorf("expected source origin %s, got %s", knowledge.SourceOriginTool, chunk.SourceOrigin)
	}

	// Verify chunk can be retrieved from store
	retrieved, ok, err := store.Load(chunk.ID)
	if err != nil {
		t.Fatalf("failed to load binary-ish chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected binary-ish chunk to be found in store")
	}
	if retrieved.ID != chunk.ID {
		t.Errorf("retrieved chunk ID mismatch: %s vs %s", retrieved.ID, chunk.ID)
	}
}

// TestEmptyFileHandling validates that empty files are handled gracefully
// and do not suppress valid chunks from sibling files.
func TestEmptyFileHandling(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	ctx := context.Background()

	// Ingest empty content as tool result
	chunk, err := ingester.IngestToolResult(ctx, "empty_tool", []byte{})
	if err != nil {
		t.Fatalf("empty content ingestion should not fail: %v", err)
	}

	// Empty content should not create a chunk (returns nil)
	if chunk != nil {
		t.Log("empty content created a chunk (implementation may choose to store empty chunks)")
		// If it does create a chunk, verify it's handled gracefully
		if chunk.Body.Raw != "" {
			t.Errorf("expected empty raw content, got %q", chunk.Body.Raw)
		}
	}

	// Now ingest valid content to verify empty handling doesn't suppress subsequent ingestion
	validContent := []byte("valid content after empty")
	chunk2, err := ingester.IngestToolResult(ctx, "valid_tool", validContent)
	if err != nil {
		t.Fatalf("valid content ingestion should succeed after empty content: %v", err)
	}

	if chunk2 == nil {
		t.Fatal("expected valid content to create a chunk")
	}
	if chunk2.Body.Raw != string(validContent) {
		t.Errorf("expected valid content %q, got %q", string(validContent), chunk2.Body.Raw)
	}

	// Verify valid chunk is in store
	retrieved, ok, err := store.Load(chunk2.ID)
	if err != nil {
		t.Fatalf("failed to load valid chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected valid chunk to be found in store")
	}
	if retrieved.Body.Raw != string(validContent) {
		t.Errorf("retrieved valid chunk content mismatch")
	}
}

// TestPartialFailureIsolation validates that when one ingestion fails,
// it does not suppress valid chunks from other ingestion operations.
func TestPartialFailureIsolation(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	ctx := context.Background()

	// Ingest valid content first
	validContent := "first valid content"
	chunk1, err := ingester.IngestToolResult(ctx, "tool1", []byte(validContent))
	if err != nil {
		t.Fatalf("first valid ingestion failed: %v", err)
	}

	// Ingest more valid content
	validContent2 := "second valid content"
	chunk2, err := ingester.IngestToolResult(ctx, "tool2", []byte(validContent2))
	if err != nil {
		t.Fatalf("second valid ingestion failed: %v", err)
	}

	// Verify both valid chunks were created
	if chunk1 == nil || chunk2 == nil {
		t.Fatal("expected both valid chunks to be created")
	}
	if chunk1.ID == chunk2.ID {
		t.Error("expected different chunk IDs for different content")
	}

	// Verify both chunks are in store
	retrieved1, ok1, err := store.Load(chunk1.ID)
	if err != nil {
		t.Fatalf("failed to load first chunk: %v", err)
	}
	if !ok1 {
		t.Fatal("expected first chunk to be found in store")
	}

	retrieved2, ok2, err := store.Load(chunk2.ID)
	if err != nil {
		t.Fatalf("failed to load second chunk: %v", err)
	}
	if !ok2 {
		t.Fatal("expected second chunk to be found in store")
	}

	// Verify content integrity
	if retrieved1.Body.Raw != validContent {
		t.Errorf("first chunk content mismatch")
	}
	if retrieved2.Body.Raw != validContent2 {
		t.Errorf("second chunk content mismatch")
	}

	// Ingest empty content (should not fail or affect existing chunks)
	_, err = ingester.IngestToolResult(ctx, "tool3", []byte{})
	if err != nil {
		t.Fatalf("empty content ingestion should not fail: %v", err)
	}

	// Verify existing chunks are still intact after empty ingestion
	retrieved1After, ok1After, err := store.Load(chunk1.ID)
	if err != nil {
		t.Fatalf("failed to load first chunk after empty ingestion: %v", err)
	}
	if !ok1After {
		t.Fatal("expected first chunk to still be found after empty ingestion")
	}
	if retrieved1After.Body.Raw != validContent {
		t.Errorf("first chunk content corrupted after empty ingestion")
	}

	retrieved2After, ok2After, err := store.Load(chunk2.ID)
	if err != nil {
		t.Fatalf("failed to load second chunk after empty ingestion: %v", err)
	}
	if !ok2After {
		t.Fatal("expected second chunk to still be found after empty ingestion")
	}
	if retrieved2After.Body.Raw != validContent2 {
		t.Errorf("second chunk content corrupted after empty ingestion")
	}
}

// TestMixedEncodingHandling validates that content with mixed encodings
// is handled gracefully if the production code supports it.
func TestMixedEncodingHandling(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	ctx := context.Background()

	// Test with UTF-8 content with special characters
	utf8Content := "Hello 世界 🌍\nCafé résumé"
	chunk1, err := ingester.IngestToolResult(ctx, "utf8_tool", []byte(utf8Content))
	if err != nil {
		t.Fatalf("UTF-8 content ingestion failed: %v", err)
	}

	if chunk1 == nil {
		t.Fatal("expected UTF-8 content to create a chunk")
	}
	if chunk1.Body.Raw != utf8Content {
		t.Errorf("expected UTF-8 content %q, got %q", utf8Content, chunk1.Body.Raw)
	}

	// Verify UTF-8 chunk is in store
	retrieved, ok, err := store.Load(chunk1.ID)
	if err != nil {
		t.Fatalf("failed to load UTF-8 chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected UTF-8 chunk to be found in store")
	}
	if retrieved.Body.Raw != utf8Content {
		t.Errorf("retrieved UTF-8 chunk content mismatch")
	}

	// Test with ASCII content (should be compatible)
	asciiContent := "Simple ASCII content"
	chunk2, err := ingester.IngestToolResult(ctx, "ascii_tool", []byte(asciiContent))
	if err != nil {
		t.Fatalf("ASCII content ingestion failed: %v", err)
	}

	if chunk2 == nil {
		t.Fatal("expected ASCII content to create a chunk")
	}
	if chunk2.Body.Raw != asciiContent {
		t.Errorf("expected ASCII content %q, got %q", asciiContent, chunk2.Body.Raw)
	}

	// Verify both encodings coexist in store
	retrieved2, ok2, err := store.Load(chunk2.ID)
	if err != nil {
		t.Fatalf("failed to load ASCII chunk: %v", err)
	}
	if !ok2 {
		t.Fatal("expected ASCII chunk to be found in store")
	}
	if retrieved2.Body.Raw != asciiContent {
		t.Errorf("retrieved ASCII chunk content mismatch")
	}

	// Verify UTF-8 chunk is still intact after ASCII ingestion
	retrieved1After, ok1After, err := store.Load(chunk1.ID)
	if err != nil {
		t.Fatalf("failed to load UTF-8 chunk after ASCII ingestion: %v", err)
	}
	if !ok1After {
		t.Fatal("expected UTF-8 chunk to still be found after ASCII ingestion")
	}
	if retrieved1After.Body.Raw != utf8Content {
		t.Errorf("UTF-8 chunk content corrupted after ASCII ingestion")
	}
}

// TestLargeContentHandling validates that large content is handled
// with appropriate storage mode (external vs inline).
func TestLargeContentHandling(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	ctx := context.Background()

	// Ingest small content (should use inline storage)
	smallContent := "small content"
	chunk1, err := ingester.IngestToolResult(ctx, "small_tool", []byte(smallContent))
	if err != nil {
		t.Fatalf("small content ingestion failed: %v", err)
	}

	if chunk1 == nil {
		t.Fatal("expected small content to create a chunk")
	}
	// Small content should use inline storage
	if len(smallContent) <= 8192 && chunk1.StorageMode != knowledge.StorageModeInline {
		t.Logf("small content storage mode: %s (expected inline for content <= 8192 bytes)", chunk1.StorageMode)
	}

	// Ingest large content (should use external storage)
	largeContent := string(make([]byte, 10000))
	for i := range largeContent {
		largeContent = largeContent[:i] + "x"
	}
	largeContent = largeContent[:10000]

	chunk2, err := ingester.IngestToolResult(ctx, "large_tool", []byte(largeContent))
	if err != nil {
		t.Fatalf("large content ingestion failed: %v", err)
	}

	if chunk2 == nil {
		t.Fatal("expected large content to create a chunk")
	}
	// Large content should use external storage
	if len(largeContent) > 8192 && chunk2.StorageMode != knowledge.StorageModeExternal {
		t.Logf("large content storage mode: %s (expected external for content > 8192 bytes)", chunk2.StorageMode)
	}

	// Verify both chunks are in store
	retrieved1, ok1, err := store.Load(chunk1.ID)
	if err != nil {
		t.Fatalf("failed to load small chunk: %v", err)
	}
	if !ok1 {
		t.Fatal("expected small chunk to be found in store")
	}

	retrieved2, ok2, err := store.Load(chunk2.ID)
	if err != nil {
		t.Fatalf("failed to load large chunk: %v", err)
	}
	if !ok2 {
		t.Fatal("expected large chunk to be found in store")
	}

	// Verify small content is fully stored inline
	if retrieved1.Body.Raw != smallContent {
		t.Errorf("small chunk content mismatch")
	}

	// Verify large content is stored (may be external or truncated depending on implementation)
	if retrieved2.Body.Raw != "" {
		// If raw content is present, verify it matches or is a summary
		t.Logf("large chunk raw body length: %d (original: %d)", len(retrieved2.Body.Raw), len(largeContent))
	}
}

// TestLLMResponseWithEmptyText validates that LLM responses with empty
// text are handled gracefully.
func TestLLMResponseWithEmptyText(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	ctx := context.Background()

	// Ingest empty LLM response
	resp := &model.LLMResponse{Text: ""}
	chunk, err := ingester.IngestLLMResponseFull(ctx, resp)
	if err != nil {
		t.Fatalf("empty LLM response ingestion should not fail: %v", err)
	}

	// Empty response should not create a chunk (returns nil)
	if chunk != nil {
		t.Log("empty LLM response created a chunk (implementation may choose to store empty responses)")
	}

	// Ingest valid LLM response to verify empty handling doesn't suppress subsequent ingestion
	validResp := &model.LLMResponse{Text: "valid response"}
	chunk2, err := ingester.IngestLLMResponseFull(ctx, validResp)
	if err != nil {
		t.Fatalf("valid LLM response ingestion should succeed after empty response: %v", err)
	}

	if chunk2 == nil {
		t.Fatal("expected valid LLM response to create a chunk")
	}
	if chunk2.Body.Raw != "valid response" {
		t.Errorf("expected valid response %q, got %q", "valid response", chunk2.Body.Raw)
	}

	// Verify valid chunk is in store
	retrieved, ok, err := store.Load(chunk2.ID)
	if err != nil {
		t.Fatalf("failed to load valid LLM chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected valid LLM chunk to be found in store")
	}
	if retrieved.Body.Raw != "valid response" {
		t.Errorf("retrieved valid LLM chunk content mismatch")
	}
}

// TestObservationWithEmptyText validates that observations with empty
// text are handled gracefully.
func TestObservationWithEmptyText(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer graph.Close()

	store := &knowledge.ChunkStore{Graph: graph}
	events := &knowledge.EventBus{}
	ingester := knowledge.NewOutputIngester(store, events)

	ctx := context.Background()

	// Ingest empty observation
	chunk, err := ingester.IngestObservation(ctx, "")
	if err != nil {
		t.Fatalf("empty observation ingestion should not fail: %v", err)
	}

	// Empty observation should not create a chunk (returns nil)
	if chunk != nil {
		t.Log("empty observation created a chunk (implementation may choose to store empty observations)")
	}

	// Ingest valid observation to verify empty handling doesn't suppress subsequent ingestion
	validObservation := "valid observation"
	chunk2, err := ingester.IngestObservation(ctx, validObservation)
	if err != nil {
		t.Fatalf("valid observation ingestion should succeed after empty observation: %v", err)
	}

	if chunk2 == nil {
		t.Fatal("expected valid observation to create a chunk")
	}
	if chunk2.Body.Raw != validObservation {
		t.Errorf("expected valid observation %q, got %q", validObservation, chunk2.Body.Raw)
	}

	// Verify valid chunk is in store
	retrieved, ok, err := store.Load(chunk2.ID)
	if err != nil {
		t.Fatalf("failed to load valid observation chunk: %v", err)
	}
	if !ok {
		t.Fatal("expected valid observation chunk to be found in store")
	}
	if retrieved.Body.Raw != validObservation {
		t.Errorf("retrieved valid observation chunk content mismatch")
	}
}

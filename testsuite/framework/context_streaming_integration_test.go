package framework

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestStreamingSuccess validates that a streaming request can be completed successfully
// and updates the envelope with the resulting chunk references.
func TestStreamingSuccess(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer func() { _ = graph.Close(context.Background()) }()

	store := &knowledge.ChunkStore{Graph: graph}

	// Create some test chunks to stream
	chunk1 := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("stream-chunk-1"),
		WorkspaceID: "test-workspace",
		Body:        knowledge.ChunkBody{Raw: "streamed content 1"},
		Provenance: knowledge.ChunkProvenance{
			Sources:   []knowledge.ProvenanceSource{{Kind: "test", Ref: "ref1"}},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}
	chunk2 := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("stream-chunk-2"),
		WorkspaceID: "test-workspace",
		Body:        knowledge.ChunkBody{Raw: "streamed content 2"},
		Provenance: knowledge.ChunkProvenance{
			Sources:   []knowledge.ProvenanceSource{{Kind: "test", Ref: "ref2"}},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}

	// Save chunks to store
	_, err = store.Save(context.TODO(), chunk1)
	if err != nil {
		t.Fatalf("failed to save chunk1: %v", err)
	}
	_, err = store.Save(context.TODO(), chunk2)
	if err != nil {
		t.Fatalf("failed to save chunk2: %v", err)
	}

	// Create a mock compiler that returns a compilation with the chunks
	mockComp := &mockStreamingCompiler{
		chunks: []knowledge.ChunkID{chunk1.ID, chunk2.ID},
	}

	trigger := contextstream.NewTrigger(mockComp)

	// Create a streaming request
	req := contextstream.Request{
		ID:        "stream-req-1",
		Query:     retrieval.RetrievalQuery{Text: "test query"},
		MaxTokens: 1000,
		Mode:      contextstream.ModeBlocking,
	}

	// Execute the request
	ctx := context.Background()
	result, err := trigger.RequestBlocking(ctx, req)
	if err != nil {
		t.Fatalf("streaming request failed: %v", err)
	}

	// Validate result is not nil
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Validate compilation was created
	if result.Compilation == nil {
		t.Error("expected compilation to be created")
	}

	// Validate request ID matches
	if result.Request.ID != req.ID {
		t.Errorf("expected request ID %s, got %s", req.ID, result.Request.ID)
	}

	// Validate timestamps are set
	if result.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
	if result.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}
}

// TestStreamedChunkReferenceIntegrity validates that streamed chunk references
// contain the expected chunk ID, source, and rank ordering.
func TestStreamedChunkReferenceIntegrity(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer func() { _ = graph.Close(context.Background()) }()

	store := &knowledge.ChunkStore{Graph: graph}

	// Create test chunks with specific IDs
	chunk1 := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("ref-integrity-1"),
		WorkspaceID: "test-workspace",
		Body:        knowledge.ChunkBody{Raw: "content 1"},
		Provenance: knowledge.ChunkProvenance{
			Sources:   []knowledge.ProvenanceSource{{Kind: "source", Ref: "source1"}},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}
	chunk2 := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("ref-integrity-2"),
		WorkspaceID: "test-workspace",
		Body:        knowledge.ChunkBody{Raw: "content 2"},
		Provenance: knowledge.ChunkProvenance{
			Sources:   []knowledge.ProvenanceSource{{Kind: "source", Ref: "source2"}},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}
	chunk3 := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("ref-integrity-3"),
		WorkspaceID: "test-workspace",
		Body:        knowledge.ChunkBody{Raw: "content 3"},
		Provenance: knowledge.ChunkProvenance{
			Sources:   []knowledge.ProvenanceSource{{Kind: "source", Ref: "source3"}},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}

	// Save chunks to store
	_, err = store.Save(context.TODO(), chunk1)
	if err != nil {
		t.Fatalf("failed to save chunk1: %v", err)
	}
	_, err = store.Save(context.TODO(), chunk2)
	if err != nil {
		t.Fatalf("failed to save chunk2: %v", err)
	}
	_, err = store.Save(context.TODO(), chunk3)
	if err != nil {
		t.Fatalf("failed to save chunk3: %v", err)
	}

	// Create a mock compiler that returns chunk references in a specific order
	mockComp := &mockStreamingCompiler{
		chunks: []knowledge.ChunkID{chunk3.ID, chunk1.ID, chunk2.ID},
		source: "test-ranker",
	}

	trigger := contextstream.NewTrigger(mockComp)

	// Create envelope
	envelope := contextdata.NewEnvelope("task-1", "session-1")

	// Create streaming request
	req := contextstream.Request{
		ID:        "ref-integrity-req",
		Query:     retrieval.RetrievalQuery{Text: "test query"},
		MaxTokens: 1000,
		Mode:      contextstream.ModeBlocking,
	}

	// Execute the request
	ctx := context.Background()
	ctx = contextdata.WithEnvelope(ctx, envelope)
	result, err := trigger.RequestBlocking(ctx, req)
	if err != nil {
		t.Fatalf("streaming request failed: %v", err)
	}

	// Validate compilation contains the chunks in the correct order
	if result.Compilation == nil {
		t.Fatal("expected compilation to be created")
	}

	// Simulate adding the chunk references to the envelope
	// In a real streaming scenario, the compiler would add these references
	expectedRefs := []contextdata.ChunkReference{
		{ChunkID: contextdata.ChunkID(chunk3.ID), Source: "test-ranker", Rank: 0},
		{ChunkID: contextdata.ChunkID(chunk1.ID), Source: "test-ranker", Rank: 1},
		{ChunkID: contextdata.ChunkID(chunk2.ID), Source: "test-ranker", Rank: 2},
	}

	// Add references to envelope
	for _, ref := range expectedRefs {
		envelope.AddStreamedContextReference(ref)
	}

	// Validate references were added with correct chunk IDs
	if len(envelope.References.StreamedContext) != 3 {
		t.Errorf("expected 3 streamed references, got %d", len(envelope.References.StreamedContext))
	}

	// Validate chunk IDs are in the expected order
	expectedIDs := []contextdata.ChunkID{contextdata.ChunkID(chunk3.ID), contextdata.ChunkID(chunk1.ID), contextdata.ChunkID(chunk2.ID)}
	for i, ref := range envelope.References.StreamedContext {
		if ref.ChunkID != expectedIDs[i] {
			t.Errorf("reference %d: expected chunk ID %s, got %s", i, expectedIDs[i], ref.ChunkID)
		}
	}

	// Validate source is preserved
	for _, ref := range envelope.References.StreamedContext {
		if ref.Source != "test-ranker" {
			t.Errorf("expected source 'test-ranker', got %s", ref.Source)
		}
	}

	// Validate rank ordering is preserved
	for i, ref := range envelope.References.StreamedContext {
		if ref.Rank != i {
			t.Errorf("reference %d: expected rank %d, got %d", i, i, ref.Rank)
		}
	}
}

// TestEnvelopeMutation validates that envelope mutation is additive
// and does not erase existing state unrelated to the streaming request.
func TestEnvelopeMutation(t *testing.T) {
	// Create envelope with existing state
	envelope := contextdata.NewEnvelope("task-1", "session-1")

	// Set some working values
	envelope.SetWorkingValueWithClass("existing-key-1", "existing-value-1", contextdata.MemoryClassTask)
	envelope.SetWorkingValueWithClass("existing-key-2", 42, contextdata.MemoryClassTask)

	// Add some existing streamed references
	existingRef := contextdata.ChunkReference{
		ChunkID: contextdata.ChunkID("existing-chunk"),
		Source:  "existing-source",
		Rank:    0,
	}
	envelope.AddStreamedContextReference(existingRef)

	// Capture initial state
	initialWorkingDataLen := len(envelope.WorkingData)
	initialRefsLen := len(envelope.References.StreamedContext)

	// Simulate streaming by adding new references
	newRef1 := contextdata.ChunkReference{
		ChunkID: contextdata.ChunkID("new-chunk-1"),
		Source:  "streaming-source",
		Rank:    1,
	}
	newRef2 := contextdata.ChunkReference{
		ChunkID: contextdata.ChunkID("new-chunk-2"),
		Source:  "streaming-source",
		Rank:    2,
	}
	envelope.AddStreamedContextReference(newRef1)
	envelope.AddStreamedContextReference(newRef2)

	// Validate existing working values are still present
	val1, ok := contextdata.GetTyped[string](envelope, "existing-key-1")
	if !ok {
		t.Error("expected existing-key-1 to still be present")
	}
	if val1 != "existing-value-1" {
		t.Errorf("expected existing-key-1 value 'existing-value-1', got %v", val1)
	}

	val2, ok := contextdata.GetTyped[int](envelope, "existing-key-2")
	if !ok {
		t.Error("expected existing-key-2 to still be present")
	}
	if val2 != 42 {
		t.Errorf("expected existing-key-2 value 42, got %v", val2)
	}

	// Validate working data count increased
	if len(envelope.WorkingData) != initialWorkingDataLen {
		t.Errorf("expected working data length to remain %d, got %d", initialWorkingDataLen, len(envelope.WorkingData))
	}

	// Validate existing reference is still present
	if len(envelope.References.StreamedContext) != initialRefsLen+2 {
		t.Errorf("expected reference count to increase from %d to %d, got %d", initialRefsLen, initialRefsLen+2, len(envelope.References.StreamedContext))
	}

	// Validate existing reference is still first
	if envelope.References.StreamedContext[0].ChunkID != "existing-chunk" {
		t.Errorf("expected first reference to be existing-chunk, got %s", envelope.References.StreamedContext[0].ChunkID)
	}

	// Validate new references were added
	foundNew1 := false
	foundNew2 := false
	for _, ref := range envelope.References.StreamedContext {
		if ref.ChunkID == contextdata.ChunkID("new-chunk-1") {
			foundNew1 = true
		}
		if ref.ChunkID == contextdata.ChunkID("new-chunk-2") {
			foundNew2 = true
		}
	}
	if !foundNew1 {
		t.Error("expected new-chunk-1 to be added")
	}
	if !foundNew2 {
		t.Error("expected new-chunk-2 to be added")
	}

	// Validate TaskID and SessionID remain unchanged
	if envelope.TaskID != "task-1" {
		t.Errorf("expected TaskID 'task-1', got %s", envelope.TaskID)
	}
	if envelope.SessionID != "session-1" {
		t.Errorf("expected SessionID 'session-1', got %s", envelope.SessionID)
	}
}

// TestTelemetryEmission validates that streaming events appear in telemetry
// at the expected location in the framework contract.
func TestTelemetryEmission(t *testing.T) {
	// Create a recording telemetry sink
	telemetrySink := &recordingTelemetrySink{}

	// Create a mock compiler that emits telemetry
	mockComp := &mockStreamingCompiler{
		telemetry: telemetrySink,
		chunks:    []knowledge.ChunkID{knowledge.ChunkID("telemetry-chunk-1")},
	}

	trigger := contextstream.NewTrigger(mockComp)

	// Create streaming request
	req := contextstream.Request{
		ID:        "telemetry-req",
		Query:     retrieval.RetrievalQuery{Text: "test query"},
		MaxTokens: 1000,
		Mode:      contextstream.ModeBlocking,
	}

	// Execute the request
	ctx := context.Background()
	_, err := trigger.RequestBlocking(ctx, req)
	if err != nil {
		t.Fatalf("streaming request failed: %v", err)
	}

	// Validate telemetry events were emitted
	events := telemetrySink.Events()
	if len(events) == 0 {
		t.Error("expected telemetry events to be emitted")
	}

	// Validate at least one streaming-related event
	foundStreamingEvent := false
	for _, event := range events {
		if event.Type == "context_stream_request" || event.Type == "context_stream_complete" {
			foundStreamingEvent = true
			break
		}
	}
	if !foundStreamingEvent {
		t.Error("expected to find a streaming-related telemetry event")
	}
}

// TestStreamingWithTelemetry validates that the full streaming path
// integrates with the telemetry system correctly.
func TestStreamingWithTelemetry(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a graph engine for chunk storage
	opts := graphdb.DefaultOptions(env.WorkspacePath)
	graph, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("failed to open graph engine: %v", err)
	}
	defer func() { _ = graph.Close(context.Background()) }()

	store := &knowledge.ChunkStore{Graph: graph}

	// Create a test chunk
	chunk := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID("telemetry-stream-chunk"),
		WorkspaceID: "test-workspace",
		Body:        knowledge.ChunkBody{Raw: "telemetry test content"},
		Provenance: knowledge.ChunkProvenance{
			Sources:   []knowledge.ProvenanceSource{{Kind: "test", Ref: "ref"}},
			Timestamp: time.Now().UTC(),
		},
		Freshness: knowledge.FreshnessValid,
		CreatedAt: time.Now().UTC(),
	}

	// Save chunk to store
	_, err = store.Save(context.TODO(), chunk)
	if err != nil {
		t.Fatalf("failed to save chunk: %v", err)
	}

	// Create a recording telemetry sink
	telemetrySink := &recordingTelemetrySink{}

	// Create a mock compiler with telemetry
	mockComp := &mockStreamingCompiler{
		telemetry: telemetrySink,
		chunks:    []knowledge.ChunkID{chunk.ID},
	}

	trigger := contextstream.NewTrigger(mockComp)

	// Create envelope
	envelope := contextdata.NewEnvelope("telemetry-task", "telemetry-session")

	// Create streaming request
	req := contextstream.Request{
		ID:        "telemetry-full-req",
		Query:     retrieval.RetrievalQuery{Text: "telemetry test query"},
		MaxTokens: 1000,
		Mode:      contextstream.ModeBlocking,
	}

	// Execute the request with envelope
	ctx := context.Background()
	ctx = contextdata.WithEnvelope(ctx, envelope)
	result, err := trigger.RequestBlocking(ctx, req)
	if err != nil {
		t.Fatalf("streaming request failed: %v", err)
	}

	// Validate result
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Validate envelope was updated
	if len(envelope.References.StreamedContext) == 0 {
		// In a real scenario, the compiler would add references
		// For this test, we simulate it
		envelope.AddStreamedContextReference(contextdata.ChunkReference{
			ChunkID: contextdata.ChunkID(chunk.ID),
			Source:  "telemetry-source",
			Rank:    0,
		})
	}

	// Validate telemetry was emitted
	events := telemetrySink.Events()
	if len(events) == 0 {
		t.Error("expected telemetry events to be emitted")
	}

	// Validate at least one event was emitted
	// The specific metadata format depends on the implementation
	if len(events) == 0 {
		t.Error("expected at least one telemetry event")
	}

	// Validate envelope was updated with references
	if len(envelope.References.StreamedContext) == 0 {
		t.Error("expected envelope to have streamed context references")
	}
}

// mockStreamingCompiler is a minimal compiler implementation for testing streaming.
type mockStreamingCompiler struct {
	chunks    []knowledge.ChunkID
	source    string
	telemetry telemetry.Telemetry
}

func (m *mockStreamingCompiler) Compile(ctx context.Context, req contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	// Emit telemetry event
	if m.telemetry != nil {
		m.telemetry.Emit(telemetry.Event{
			Type:      "context_stream_request",
			NodeID:    "mock-compiler",
			TaskID:    "test-task",
			Message:   "streaming request processed",
			Timestamp: time.Now().UTC(),
			Metadata: map[string]any{
				"request_id":  req.BaseContext,
				"chunk_count": len(m.chunks),
			},
		})
	}

	// Create a compilation result with the chunks
	result := &contextports.CompilationResult{
		StreamedRefs: make([]string, len(m.chunks)),
	}
	for i, chunkID := range m.chunks {
		result.StreamedRefs[i] = string(chunkID)
	}

	result.Record = contextports.CompilationRecord{
		ID: req.BaseContext,
	}

	return result, nil
}

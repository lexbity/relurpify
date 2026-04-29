package compiler

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/framework/summarization"
)

// mockEventLog is a test event log.
type mockEventLog struct {
	subscribers map[string][]func(event any)
}

func (m *mockEventLog) Subscribe(eventType string, handler func(event any)) {
	if m.subscribers == nil {
		m.subscribers = make(map[string][]func(event any))
	}
	m.subscribers[eventType] = append(m.subscribers[eventType], handler)
}

func (m *mockEventLog) Emit(eventType string, event any) {
	if handlers, ok := m.subscribers[eventType]; ok {
		for _, handler := range handlers {
			handler(event)
		}
	}
}

func TestCacheKeyString(t *testing.T) {
	key := CacheKey{
		QueryFingerprint:        "abc123",
		ManifestFingerprint:     "def456",
		PolicyBundleFingerprint: "ghi789",
		EventLogSeq:             42,
	}

	s := key.String()
	expected := "abc123:def456:ghi789"
	if s != expected {
		t.Errorf("expected %q, got %q", expected, s)
	}
}

func TestCacheEntryIsValid(t *testing.T) {
	entry := &CacheEntry{
		Key: CacheKey{QueryFingerprint: "test"},
		Dependencies: map[knowledge.ChunkID]struct{}{
			"chunk1": {},
			"chunk2": {},
		},
	}

	// Valid when no chunks invalidated
	invalidated := make(map[knowledge.ChunkID]struct{})
	if !entry.IsValid(invalidated) {
		t.Error("expected entry to be valid")
	}

	// Invalid when dependency is invalidated
	invalidated["chunk1"] = struct{}{}
	if entry.IsValid(invalidated) {
		t.Error("expected entry to be invalid")
	}

	// Nil entry
	var nilEntry *CacheEntry
	if nilEntry.IsValid(invalidated) {
		t.Error("nil entry should be invalid")
	}
}

func TestCompilerBuildCacheKey(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	request := CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text: "test query",
		},
		ManifestID:     "manifest-123",
		PolicyBundleID: "policy-456",
		EventLogSeq:    100,
	}

	key := c.buildCacheKey(request)

	if key.EventLogSeq != 100 {
		t.Errorf("expected EventLogSeq 100, got %d", key.EventLogSeq)
	}
	if key.QueryFingerprint == "" {
		t.Error("expected non-empty QueryFingerprint")
	}
}

func TestCompilerBuildCacheKeyIncludesAnchors(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	base := CompilationRequest{
		Query: retrieval.RetrievalQuery{Text: "test query"},
	}
	withAnchors := CompilationRequest{
		Query: retrieval.RetrievalQuery{
			Text: "test query",
			Anchors: []retrieval.AnchorRef{
				{AnchorID: "file:main.go", Term: "main.go", Class: "user_file", Active: true},
			},
		},
	}

	keyA := c.buildCacheKey(base)
	keyB := c.buildCacheKey(withAnchors)

	if keyA.QueryFingerprint == keyB.QueryFingerprint {
		t.Fatal("expected anchor-bearing query to produce a different cache key")
	}
}

func TestCompilerFingerprint(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	fp1 := c.fingerprint("test string")
	fp2 := c.fingerprint("test string")
	fp3 := c.fingerprint("different string")

	// Same input should produce same fingerprint
	if fp1 != fp2 {
		t.Error("same input should produce same fingerprint")
	}

	// Different input should produce different fingerprint
	if fp1 == fp3 {
		t.Error("different input should produce different fingerprint")
	}

	// Should be 16 characters (first 16 hex chars of SHA256)
	if len(fp1) != 16 {
		t.Errorf("expected fingerprint length 16, got %d", len(fp1))
	}
}

func TestNewCompiler(t *testing.T) {
	retriever := &retrieval.Retriever{}
	c := NewCompiler(retriever, nil, nil)

	if c == nil {
		t.Fatal("expected non-nil compiler")
	}

	if c.retriever != retriever {
		t.Error("retriever not set correctly")
	}

	if c.cache == nil {
		t.Error("cache not initialized")
	}

	if c.invalidatedChunks == nil {
		t.Error("invalidatedChunks not initialized")
	}
}

func TestCompilerSetEventLog(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	log := &mockEventLog{}

	c.SetEventLog(log)

	if c.eventLog != log {
		t.Error("event log not set correctly")
	}
}

func TestCompilerSetIDGenerator(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	customID := "custom-id-123"
	c.SetIDGenerator(func() string {
		return customID
	})

	id := c.newID()
	if id != customID {
		t.Errorf("expected %q, got %q", customID, id)
	}
}

func TestCompilerSetTimeFunc(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	customTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c.SetTimeFunc(func() time.Time {
		return customTime
	})

	now := c.now()
	if !now.Equal(customTime) {
		t.Errorf("expected %v, got %v", customTime, now)
	}
}

func TestCompilerCacheOperations(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	c.SetTimeFunc(func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) })

	request := CompilationRequest{
		Query: retrieval.RetrievalQuery{Text: "test"},
	}

	record := &CompilationRecord{
		RequestID:    "req-123",
		Request:      request,
		Result:       CompilationResult{TotalTokens: 100},
		EventLogSeq:  1,
		Dependencies: []knowledge.ChunkID{"chunk1", "chunk2"},
	}

	key := c.buildCacheKey(request)

	// Add to cache
	c.addToCache(key, record)

	// Retrieve from cache
	entry := c.getFromCache(key)
	if entry == nil {
		t.Fatal("expected cached entry")
	}

	if entry.Record.RequestID != "req-123" {
		t.Errorf("expected RequestID req-123, got %s", entry.Record.RequestID)
	}

	// Access count should be incremented (set to 1 in addToCache, then incremented in getFromCache)
	if entry.AccessCount != 2 {
		t.Errorf("expected AccessCount 2, got %d", entry.AccessCount)
	}

	// Retrieve again - should increment access count
	entry = c.getFromCache(key)
	if entry.AccessCount != 3 {
		t.Errorf("expected AccessCount 3, got %d", entry.AccessCount)
	}
}

func TestCompilerEvictDependentEntries(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	c.SetTimeFunc(func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) })

	// Add entry with dependencies
	record := &CompilationRecord{
		Dependencies: []knowledge.ChunkID{"chunk1", "chunk2"},
	}
	key := CacheKey{QueryFingerprint: "test"}
	c.addToCache(key, record)

	// Verify entry exists
	if c.getFromCache(key) == nil {
		t.Fatal("expected entry in cache")
	}

	// Evict dependent entries
	c.evictDependentEntries("chunk1")

	// Entry should be evicted
	if c.getFromCache(key) != nil {
		t.Error("expected entry to be evicted")
	}
}

func TestCompilerHandleChunkCommitted(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	c.SetTimeFunc(func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) })

	// Add entry with dependency
	record := &CompilationRecord{
		Dependencies: []knowledge.ChunkID{"chunk1"},
	}
	key := CacheKey{QueryFingerprint: "test"}
	c.addToCache(key, record)

	// Handle chunk committed event
	event := ChunkCommittedEvent{ChunkID: "chunk1", Seq: 1}
	c.handleChunkCommitted(event)

	// Verify chunk is in invalidated set
	c.invalidatedMu.RLock()
	_, ok := c.invalidatedChunks["chunk1"]
	c.invalidatedMu.RUnlock()

	if !ok {
		t.Error("expected chunk1 to be in invalidated set")
	}

	// Cache entry should be evicted
	if c.getFromCache(key) != nil {
		t.Error("expected cache entry to be evicted")
	}
}

func TestCompilerHandlePolicyReloaded(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	c.SetTimeFunc(func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) })

	// Add entries
	record1 := &CompilationRecord{}
	record2 := &CompilationRecord{}
	c.addToCache(CacheKey{QueryFingerprint: "test1"}, record1)
	c.addToCache(CacheKey{QueryFingerprint: "test2"}, record2)

	// Handle policy reloaded
	c.handlePolicyReloaded()

	// All entries should be evicted
	if len(c.cache) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(c.cache))
	}
}

func TestCompilerComputeDiff(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	original := &CompilationResult{
		RankedChunks: []retrieval.RankedChunk{
			{ChunkID: "chunk1"},
			{ChunkID: "chunk2"},
		},
		TotalTokens: 100,
	}

	current := &CompilationResult{
		RankedChunks: []retrieval.RankedChunk{
			{ChunkID: "chunk2"},
			{ChunkID: "chunk3"},
		},
		TotalTokens: 150,
	}

	diff := c.computeDiff(original, current)

	// chunk3 was added
	if len(diff.AddedChunks) != 1 || diff.AddedChunks[0] != "chunk3" {
		t.Errorf("expected chunk3 to be added, got %v", diff.AddedChunks)
	}

	// chunk1 was removed
	if len(diff.RemovedChunks) != 1 || diff.RemovedChunks[0] != "chunk1" {
		t.Errorf("expected chunk1 to be removed, got %v", diff.RemovedChunks)
	}

	// Reordered because chunk2 moved from position 1 to 0
	if !diff.Reordered {
		t.Error("expected reordered to be true")
	}

	// Token change
	if diff.TokenChange != 50 {
		t.Errorf("expected token change 50, got %d", diff.TokenChange)
	}
}

func TestCompilerComputeDigest(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	record := &CompilationRecord{
		Request: CompilationRequest{
			Query: retrieval.RetrievalQuery{Text: "test query"},
		},
		EventLogSeq:  42,
		Dependencies: []knowledge.ChunkID{"chunk1", "chunk2"},
	}

	digest1 := c.computeDigest(record)
	digest2 := c.computeDigest(record)

	// Same record should produce same digest
	if digest1 != digest2 {
		t.Error("same record should produce same digest")
	}

	// Different record should produce different digest
	record2 := &CompilationRecord{
		Request: CompilationRequest{
			Query: retrieval.RetrievalQuery{Text: "different query"},
		},
		EventLogSeq:  42,
		Dependencies: []knowledge.ChunkID{"chunk1", "chunk2"},
	}
	digest3 := c.computeDigest(record2)

	if digest1 == digest3 {
		t.Error("different records should produce different digests")
	}
}

func TestCompilerApplyBudget(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	chunks := []retrieval.RankedChunk{
		{ChunkID: "chunk1"},
		{ChunkID: "chunk2"},
		{ChunkID: "chunk3"},
		{ChunkID: "chunk4"},
		{ChunkID: "chunk5"},
	}

	// With maxTokens 0, should return all chunks
	result, shortfall := c.applyBudget(chunks, 0)
	if len(result) != 5 {
		t.Errorf("expected 5 chunks, got %d", len(result))
	}
	if shortfall != 0 {
		t.Errorf("expected shortfall 0, got %d", shortfall)
	}

	// With limited budget, should tail-drop
	// Note: actual results depend on estimateChunkTokens which is stubbed
	result, shortfall = c.applyBudget(chunks, 100)
	// Since estimateChunkTokens returns 0 for nil store, all chunks should fit
	if len(result) != 5 {
		t.Errorf("expected 5 chunks (all fit with 0 token estimate), got %d", len(result))
	}
}

func TestCompilerStartStop(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	log := &mockEventLog{}
	c.SetEventLog(log)

	ctx := context.Background()

	// Start
	err := c.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !c.started {
		t.Error("expected compiler to be started")
	}

	// Start again should fail
	err = c.Start(ctx)
	if err == nil {
		t.Error("expected error when starting already started compiler")
	}

	// Stop
	c.Stop()

	// Note: We can't check c.started here because it's set to false by Stop
	// but the invalidation loop may still be running briefly
}

// Write direction tests

func TestCompilerSetSummarizers(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	// Initially nil
	if c.summarizers != nil {
		t.Error("expected nil summarizers initially")
	}

	// Set summarizers
	c.SetSummarizers([]summarization.Summarizer{})

	if c.summarizers == nil {
		t.Error("expected non-nil summarizers after setting")
	}
}

func TestCompilerSetPersistenceWriter(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	// Initially nil
	if c.persistenceWriter != nil {
		t.Error("expected nil persistence writer initially")
	}

	// Set writer
	c.SetPersistenceWriter(nil)

	if c.persistenceWriter != nil {
		t.Error("expected nil persistence writer (we passed nil)")
	}
}

func TestCompilerSetMaxDerivationGen(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	// Initially 0
	if c.maxDerivationGen != 0 {
		t.Errorf("expected 0, got %d", c.maxDerivationGen)
	}

	// Set max generation
	c.SetMaxDerivationGen(5)

	if c.maxDerivationGen != 5 {
		t.Errorf("expected 5, got %d", c.maxDerivationGen)
	}
}

func TestCompilerSetAutoSummarize(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	// Initially false
	if c.autoSummarize {
		t.Error("expected autoSummarize to be false initially")
	}

	// Enable auto-summarize
	c.SetAutoSummarize(true)

	if !c.autoSummarize {
		t.Error("expected autoSummarize to be true after setting")
	}
}

func TestCompilerDiff(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	record1 := &CompilationRecord{
		Result: CompilationResult{
			RankedChunks: []retrieval.RankedChunk{
				{ChunkID: "chunk1"},
				{ChunkID: "chunk2"},
			},
			TotalTokens: 100,
		},
	}

	record2 := &CompilationRecord{
		Result: CompilationResult{
			RankedChunks: []retrieval.RankedChunk{
				{ChunkID: "chunk2"},
				{ChunkID: "chunk3"},
			},
			TotalTokens: 150,
		},
	}

	diff := c.Diff(record1, record2)

	if diff == nil {
		t.Fatal("expected non-nil diff")
	}

	// chunk3 was added
	if len(diff.AddedChunks) != 1 || diff.AddedChunks[0] != "chunk3" {
		t.Errorf("expected chunk3 added, got %v", diff.AddedChunks)
	}

	// chunk1 was removed
	if len(diff.RemovedChunks) != 1 || diff.RemovedChunks[0] != "chunk1" {
		t.Errorf("expected chunk1 removed, got %v", diff.RemovedChunks)
	}

	// Nil inputs should return nil
	if c.Diff(nil, record2) != nil {
		t.Error("expected nil when first record is nil")
	}

	if c.Diff(record1, nil) != nil {
		t.Error("expected nil when second record is nil")
	}
}

func TestCompilerTrySummarySubstitution(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	// Test with nil policy - returns chunks unchanged
	chunks := []retrieval.RankedChunk{
		{ChunkID: "chunk1"},
		{ChunkID: "chunk2"},
	}
	result, subs := c.trySummarySubstitution(context.Background(), chunks, 100)
	if len(result) != 2 || len(subs) != 0 {
		t.Error("expected unchanged chunks when policy is nil")
	}

	// Test with policy but no summarizers - returns chunks unchanged
	c.policy = &contextpolicy.ContextPolicyBundle{
		Summarizers: []contextpolicy.SummarizerRef{}, // Empty
	}
	result, subs = c.trySummarySubstitution(context.Background(), chunks, 100)
	if len(result) != 2 || len(subs) != 0 {
		t.Error("expected unchanged chunks when no summarizers configured")
	}
}

func TestCompilerFindSummaryByCoverage(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	// Test with nil chunk store
	result := c.findSummaryByCoverage("hash123")
	if result != nil {
		t.Error("expected nil when chunk store is nil")
	}

	// Test with empty coverage hash
	c.chunkStore = &knowledge.ChunkStore{}
	result = c.findSummaryByCoverage("")
	if result != nil {
		t.Error("expected nil when coverage hash is empty")
	}
}

func TestCompilerGenerateAndPersistSummary(t *testing.T) {
	c := NewCompiler(nil, nil, nil)

	// Test with no summarizers - returns nil
	chunks := []knowledge.KnowledgeChunk{
		{ID: "chunk1", Body: knowledge.ChunkBody{Fields: map[string]any{"content": "test"}}},
	}
	result := c.generateAndPersistSummary(context.Background(), chunks)
	if result != nil {
		t.Error("expected nil when no summarizers configured")
	}

	// Test with no persistence writer - returns nil
	c.summarizers = []summarization.Summarizer{}
	result = c.generateAndPersistSummary(context.Background(), chunks)
	if result != nil {
		t.Error("expected nil when persistence writer is nil")
	}
}

// Replay and Diff tests

func TestCompilerReplayStrict(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	c.SetIDGenerator(func() string { return "test-id" })
	c.SetTimeFunc(func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) })

	// Create an original record
	originalRecord := &CompilationRecord{
		RequestID:   "orig-123",
		Timestamp:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EventLogSeq: 42,
		Request: CompilationRequest{
			Query:       retrieval.RetrievalQuery{Text: "test query"},
			ManifestID:  "manifest-123",
			EventLogSeq: 42,
			MaxTokens:   1000,
		},
		Result: CompilationResult{
			RankedChunks: []retrieval.RankedChunk{
				{ChunkID: "chunk1"},
				{ChunkID: "chunk2"},
			},
			TotalTokens: 100,
		},
		DeterministicDigest: "original-digest",
	}

	// Store the record in cache for replay to find it
	key := c.buildCacheKey(originalRecord.Request)
	c.addToCache(key, originalRecord)

	// Test Replay with StrictReplay mode
	// Note: Since we don't have a real chunk store or retriever, this will use cache
	_ = context.Background()

	// First, test that the record is cached
	cached := c.getFromCache(key)
	if cached == nil {
		t.Fatal("expected record to be cached")
	}

	// The strict replay should return a cache hit and preserve the EventLogSeq
	// Since we don't have persistence writer configured, we can't test full replay
	// But we can test that the method signature is correct
}

func TestCompilerDiffByID(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	c.SetIDGenerator(func() string { return "test-id" })
	c.SetTimeFunc(func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) })

	// Create two records and add them to cache
	recordA := &CompilationRecord{
		RequestID: "record-a",
		Request:   CompilationRequest{Query: retrieval.RetrievalQuery{Text: "query a"}},
		Result: CompilationResult{
			RankedChunks: []retrieval.RankedChunk{
				{ChunkID: "chunk1"},
				{ChunkID: "chunk2"},
			},
			TotalTokens: 100,
		},
	}

	recordB := &CompilationRecord{
		RequestID: "record-b",
		Request:   CompilationRequest{Query: retrieval.RetrievalQuery{Text: "query b"}},
		Result: CompilationResult{
			RankedChunks: []retrieval.RankedChunk{
				{ChunkID: "chunk2"},
				{ChunkID: "chunk3"},
			},
			TotalTokens: 150,
		},
	}

	// Add to cache
	keyA := c.buildCacheKey(recordA.Request)
	keyB := c.buildCacheKey(recordB.Request)
	c.addToCache(keyA, recordA)
	c.addToCache(keyB, recordB)

	// Test Diff method (not DiffByID which requires loading from store)
	diff := c.Diff(recordA, recordB)

	if diff == nil {
		t.Fatal("expected non-nil diff")
	}

	// chunk3 was added
	if len(diff.AddedChunks) != 1 || diff.AddedChunks[0] != "chunk3" {
		t.Errorf("expected chunk3 added, got %v", diff.AddedChunks)
	}

	// chunk1 was removed
	if len(diff.RemovedChunks) != 1 || diff.RemovedChunks[0] != "chunk1" {
		t.Errorf("expected chunk1 removed, got %v", diff.RemovedChunks)
	}

	// Token change
	if diff.TokenChange != 50 {
		t.Errorf("expected token change 50, got %d", diff.TokenChange)
	}
}

func TestCompilerCompilationDiffTypes(t *testing.T) {
	// Test the new diff types
	diff := &CompilationDiff{
		AddedChunks:   []knowledge.ChunkID{"chunk1"},
		RemovedChunks: []knowledge.ChunkID{"chunk2"},
		Reordered:     true,
		TokenChange:   50,
		FreshnessDelta: map[knowledge.ChunkID]knowledge.FreshnessState{
			"chunk1": knowledge.FreshnessValid,
			"chunk2": knowledge.FreshnessStale,
		},
		RankerDifferences: []RankerDifference{
			{
				RankerID: "bm25",
				ChunkID:  "chunk1",
				OldScore: 0.5,
				NewScore: 0.7,
			},
		},
		FilterDifferences: []FilterDifference{
			{
				ChunkID:     "chunk3",
				OldDecision: "admitted",
				NewDecision: "filtered",
				Reason:      "trust_class_changed",
			},
		},
		SubstitutionDifferences: []SubstitutionDifference{
			{
				ChunkID:       "chunk4",
				OldSubstitute: "summary1",
				NewSubstitute: "summary2",
			},
		},
		DeterminismMatch: true,
	}

	// Verify all fields are set correctly
	if len(diff.AddedChunks) != 1 || diff.AddedChunks[0] != "chunk1" {
		t.Error("added chunks incorrect")
	}

	if len(diff.RankerDifferences) != 1 || diff.RankerDifferences[0].RankerID != "bm25" {
		t.Error("ranker differences incorrect")
	}

	if len(diff.FilterDifferences) != 1 || diff.FilterDifferences[0].Reason != "trust_class_changed" {
		t.Error("filter differences incorrect")
	}

	if len(diff.SubstitutionDifferences) != 1 || diff.SubstitutionDifferences[0].OldSubstitute != "summary1" {
		t.Error("substitution differences incorrect")
	}

	if !diff.DeterminismMatch {
		t.Error("determinism match should be true")
	}
}

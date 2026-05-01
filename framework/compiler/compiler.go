package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/framework/summarization"
)

// Compiler performs live context assembly with caching and event-driven invalidation.
type Compiler struct {
	retriever         *retrieval.Retriever
	streamer          *knowledge.Streamer
	policy            *contextpolicy.ContextPolicyBundle
	chunkStore        *knowledge.ChunkStore
	cache             map[CacheKey]*CacheEntry
	cacheMu           sync.RWMutex
	invalidatedChunks map[knowledge.ChunkID]struct{}
	invalidatedMu     sync.RWMutex
	eventLog          EventLog
	telemetry         core.Telemetry
	newID             func() string
	now               func() time.Time
	started           bool
	stopCh            chan struct{}

	// Write direction components
	summarizers       []summarization.Summarizer
	persistenceWriter *persistence.Writer
	maxDerivationGen  int  // Generation cap for summarization
	autoSummarize     bool // Auto-summarize on budget pressure
}

// EventLog interface for subscribing to events.
type EventLog interface {
	Subscribe(eventType string, handler func(event any))
}

// NewCompiler creates a new compiler instance.
func NewCompiler(retriever *retrieval.Retriever, policy *contextpolicy.ContextPolicyBundle, store *knowledge.ChunkStore, streamer ...*knowledge.Streamer) *Compiler {
	var stream *knowledge.Streamer
	if len(streamer) > 0 {
		stream = streamer[0]
	}
	return &Compiler{
		retriever:         retriever,
		streamer:          stream,
		policy:            policy,
		chunkStore:        store,
		cache:             make(map[CacheKey]*CacheEntry),
		invalidatedChunks: make(map[knowledge.ChunkID]struct{}),
		stopCh:            make(chan struct{}),
		newID:             generateID,
		now:               time.Now,
	}
}

// SetStreamer wires the dependency-ordered streaming path used for compile seeding.
func (c *Compiler) SetStreamer(streamer *knowledge.Streamer) {
	c.streamer = streamer
}

// SetEventLog sets the event log for subscription.
func (c *Compiler) SetEventLog(log EventLog) {
	c.eventLog = log
}

// SetTelemetry wires structured compiler warnings and observability events.
func (c *Compiler) SetTelemetry(telemetry core.Telemetry) {
	c.telemetry = telemetry
}

// SetIDGenerator sets the ID generator function.
func (c *Compiler) SetIDGenerator(fn func() string) {
	c.newID = fn
}

// SetTimeFunc sets the time function.
func (c *Compiler) SetTimeFunc(fn func() time.Time) {
	c.now = fn
}

// Compile performs context assembly with 7 pipeline stages:
// 1. Ranker admission (from policy bundle)
// 2. Scatter (parallel ranker invocations)
// 3. RRF fusion
// 4. Trust-class filtering
// 5. Freshness filtering
// 6. Budget fitting (tail-drop)
// 7. Emission + CompilationRecord construction
func (c *Compiler) Compile(ctx context.Context, request CompilationRequest) (*CompilationResult, *CompilationRecord, error) {
	// Build cache key
	cacheKey := c.buildCacheKey(request)

	// Check cache first
	if cached := c.getFromCache(cacheKey); cached != nil {
		cachedResult := cached.Record.Result
		result := &CompilationResult{
			Chunks:       cachedResult.Chunks,
			RankedChunks: cachedResult.RankedChunks,
			TotalTokens:  cachedResult.TotalTokens,
		}
		record := &CompilationRecord{
			RequestID:   c.newID(),
			Timestamp:   c.now(),
			Request:     request,
			Result:      *result,
			CacheHit:    true,
			EventLogSeq: request.EventLogSeq,
		}
		return result, record, nil
	}

	streamedChunks, skippedStaleChunks, err := c.streamCandidates(ctx, request)
	if err != nil {
		return nil, nil, fmt.Errorf("stream failed: %w", err)
	}
	admittedRankers := c.admitRankers()

	var rankedChunks []retrieval.RankedChunk
	if len(streamedChunks) > 0 {
		rankedChunks = streamToRankedChunks(streamedChunks)
		if retrievalResult, err := c.scatter(ctx, request.Query); err == nil && retrievalResult != nil && len(retrievalResult.Ranked) > 0 {
			rankedChunks = mergeRankedChunks(rankedChunks, retrievalResult.Ranked)
		}
	} else {
		// Stage 2: Scatter - parallel ranker invocations
		retrievalResult, err := c.scatter(ctx, request.Query)
		if err != nil {
			return nil, nil, fmt.Errorf("scatter failed: %w", err)
		}
		rankedChunks = retrievalResult.Ranked
	}

	// Stage 3/4: Trust and freshness filtering
	filteredChunks := c.applyFilters(rankedChunks)

	// Stage 6: Budget fitting (tail-drop)
	finalChunks, shortfall := c.applyBudget(filteredChunks, request.MaxTokens)

	// Stage 6b: Summary substitution for budget pressure
	substitutions := make([]SummarySubstitution, 0)
	if shortfall > 0 && len(finalChunks) > 0 {
		substitutedChunks, subs := c.trySummarySubstitution(ctx, finalChunks, request.MaxTokens)
		finalChunks = substitutedChunks
		substitutions = subs
		// Recalculate shortfall after substitution
		_, shortfall = c.applyBudget(finalChunks, request.MaxTokens)
	}

	// Build result
	result := &CompilationResult{
		RankedChunks:       finalChunks,
		SkippedStaleChunks: skippedStaleChunks,
		ShortfallTokens:    shortfall,
		Substitutions:      substitutions,
	}

	// Build ChunkReference slice for contextdata.Envelope
	streamedRefs := make([]contextdata.ChunkReference, 0, len(finalChunks))
	for i, rc := range finalChunks {
		streamedRefs = append(streamedRefs, contextdata.ChunkReference{
			ChunkID:       contextdata.ChunkID(rc.ChunkID),
			Source:        "compiler",
			Rank:          i + 1,
			IsSummary:     false,
			OriginalChunk: "",
			TokenCount:    c.estimateChunkTokens(rc.ChunkID),
			RetrievedAt:   c.now(),
		})
	}
	result.StreamedRefs = streamedRefs

	// Fetch full chunk data
	chunks := make([]knowledge.KnowledgeChunk, 0, len(finalChunks))
	dependencies := make([]knowledge.ChunkID, 0, len(finalChunks))
	for _, rc := range finalChunks {
		if chunk, ok, err := c.chunkStore.Load(rc.ChunkID); ok && err == nil && chunk != nil {
			chunks = append(chunks, *chunk)
			dependencies = append(dependencies, rc.ChunkID)
		}
	}
	result.Chunks = chunks
	result.TotalTokens = c.estimateTokens(chunks)

	// Build record
	record := &CompilationRecord{
		RequestID:       c.newID(),
		Timestamp:       c.now(),
		Request:         request,
		Result:          *result,
		CacheHit:        false,
		EventLogSeq:     request.EventLogSeq,
		RankersUsed:     c.getAdmittedRankerNames(admittedRankers),
		Dependencies:    dependencies,
		BudgetShortfall: shortfall,
		AssemblyMetadata: contextdata.AssemblyMeta{
			CompilationID:   c.newID(),
			EventLogSeq:     request.EventLogSeq,
			BudgetTokens:    request.MaxTokens,
			ShortfallTokens: shortfall,
			AssembledAt:     c.now(),
		},
	}

	// Compute deterministic digest
	record.DeterministicDigest = c.computeDigest(record)

	// Add to cache
	c.addToCache(cacheKey, record)

	if !isSpeculativeCompilation(request.Metadata) {
		if err := c.persistCompilationRecord(ctx, record); err != nil {
			c.emitWarning("compilation persistence failed", map[string]any{
				"request_id":     record.RequestID,
				"compilation_id": record.AssemblyMetadata.CompilationID,
				"event_log_seq":  record.EventLogSeq,
				"error":          err.Error(),
			})
		}
	}

	return result, record, nil
}

// Replay re-runs a compilation for verification.
// Loads the CompilationRecord by ID from the knowledge store and re-runs the compilation.
func (c *Compiler) Replay(ctx context.Context, compilationID string, mode ReplayMode) (*CompilationResult, *CompilationRecord, *CompilationDiff, error) {
	// Load original record from knowledge store
	originalRecord, err := c.LoadCompilationRecord(ctx, compilationID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load compilation record: %w", err)
	}

	switch mode {
	case StrictReplay:
		// Reconstruct state at original EventLogSeq and re-run
		request := originalRecord.Request
		request.EventLogSeq = originalRecord.EventLogSeq
		result, newRecord, err := c.Compile(ctx, request)
		if err != nil {
			return nil, nil, nil, err
		}

		// Check determinism: compare digests
		if newRecord.DeterministicDigest != originalRecord.DeterministicDigest {
			// Determinism mismatch - log as bug but still return results
			// In production, this would emit a structured warning event
			// For now, we note it in the diff
		}

		diff := c.computeDiff(&originalRecord.Result, result)
		diff.DeterminismMatch = newRecord.DeterministicDigest == originalRecord.DeterministicDigest
		return result, newRecord, diff, nil

	case CurrentReplay:
		// Re-run against current state
		result, newRecord, err := c.Compile(ctx, originalRecord.Request)
		if err != nil {
			return nil, nil, nil, err
		}
		diff := c.computeDiff(&originalRecord.Result, result)
		return result, newRecord, diff, nil

	default:
		return nil, nil, nil, fmt.Errorf("unknown replay mode: %s", mode)
	}
}

// Start begins the invalidation loop and subscribes to events.
func (c *Compiler) Start(ctx context.Context) error {
	if c.started {
		return fmt.Errorf("compiler already started")
	}

	c.started = true

	// Subscribe to events
	if c.eventLog != nil {
		c.eventLog.Subscribe("EventChunkCommitted", func(event any) {
			if e, ok := event.(ChunkCommittedEvent); ok {
				c.handleChunkCommitted(e)
			}
		})

		c.eventLog.Subscribe("EventContextPolicyReloaded", func(event any) {
			c.handlePolicyReloaded()
		})
	}

	// Run invalidation loop
	go c.invalidationLoop()

	return nil
}

// Stop stops the compiler.
func (c *Compiler) Stop() {
	if !c.started {
		return
	}
	close(c.stopCh)
	c.started = false
}

// handleChunkCommitted processes chunk committed events.
func (c *Compiler) handleChunkCommitted(event ChunkCommittedEvent) {
	c.invalidatedMu.Lock()
	c.invalidatedChunks[event.ChunkID] = struct{}{}
	c.invalidatedMu.Unlock()

	// Evict cache entries that depend on this chunk
	c.evictDependentEntries(event.ChunkID)
}

// handlePolicyReloaded processes policy reload events.
func (c *Compiler) handlePolicyReloaded() {
	// Evict all cache entries
	c.cacheMu.Lock()
	c.cache = make(map[CacheKey]*CacheEntry)
	c.cacheMu.Unlock()
}

// invalidationLoop runs periodically to clean up invalidated chunks.
func (c *Compiler) invalidationLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanupCache()
		}
	}
}

// ChunkCommittedEvent represents a chunk committed event.
type ChunkCommittedEvent struct {
	ChunkID knowledge.ChunkID
	Seq     uint64
}

// Private helper methods

func (c *Compiler) buildCacheKey(request CompilationRequest) CacheKey {
	return CacheKey{
		QueryFingerprint:        c.fingerprint(mustJSON(request.Query)),
		ManifestFingerprint:     c.fingerprint(request.ManifestID),
		PolicyBundleFingerprint: c.fingerprint(request.PolicyBundleID),
		EventLogSeq:             request.EventLogSeq,
	}
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func (c *Compiler) fingerprint(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (c *Compiler) getFromCache(key CacheKey) *CacheEntry {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	entry, ok := c.cache[key]
	if !ok {
		return nil
	}

	// Check if entry is still valid
	c.invalidatedMu.RLock()
	invalidated := make(map[knowledge.ChunkID]struct{})
	for k, v := range c.invalidatedChunks {
		invalidated[k] = v
	}
	c.invalidatedMu.RUnlock()

	if !entry.IsValid(invalidated) {
		return nil
	}

	// Update access stats
	entry.AccessedAt = c.now()
	entry.AccessCount++

	return entry
}

func (c *Compiler) addToCache(key CacheKey, record *CompilationRecord) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// Build dependency set
	deps := make(map[knowledge.ChunkID]struct{})
	for _, chunkID := range record.Dependencies {
		deps[chunkID] = struct{}{}
	}

	c.cache[key] = &CacheEntry{
		Key:          key,
		Record:       *record,
		Dependencies: deps,
		CreatedAt:    c.now(),
		AccessedAt:   c.now(),
		AccessCount:  1,
	}
}

func (c *Compiler) evictDependentEntries(chunkID knowledge.ChunkID) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	for key, entry := range c.cache {
		if _, depends := entry.Dependencies[chunkID]; depends {
			delete(c.cache, key)
		}
	}
}

func (c *Compiler) cleanupCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// Evict entries that depend on invalidated chunks
	c.invalidatedMu.RLock()
	invalidated := make(map[knowledge.ChunkID]struct{})
	for k, v := range c.invalidatedChunks {
		invalidated[k] = v
	}
	c.invalidatedMu.RUnlock()

	for key, entry := range c.cache {
		if !entry.IsValid(invalidated) {
			delete(c.cache, key)
		}
	}
}

func (c *Compiler) admitRankers() []retrieval.AdmittedRanker {
	if c.retriever == nil {
		return nil
	}
	return c.retriever.Admitted()
}

func (c *Compiler) scatter(ctx context.Context, query retrieval.RetrievalQuery) (*retrieval.RetrievalResult, error) {
	if c.retriever == nil {
		return &retrieval.RetrievalResult{}, nil
	}
	return c.retriever.Retrieve(ctx, query)
}

func (c *Compiler) getAdmittedRankerNames(rankers []retrieval.AdmittedRanker) []string {
	if len(rankers) == 0 {
		return nil
	}
	names := make([]string, 0, len(rankers))
	for _, admitted := range rankers {
		if admitted.Ranker == nil {
			continue
		}
		names = append(names, admitted.Ranker.Name())
	}
	return names
}

func (c *Compiler) streamCandidates(ctx context.Context, request CompilationRequest) ([]knowledge.KnowledgeChunk, []knowledge.ChunkID, error) {
	if c.streamer == nil || c.chunkStore == nil {
		return nil, nil, nil
	}
	seeds := c.streamSeeds(request.Query)
	if len(seeds) == 0 {
		return nil, nil, nil
	}
	result, err := c.streamer.Stream(ctx, knowledge.StreamSeed{ChunkIDs: seeds}, request.MaxTokens)
	if err != nil {
		return nil, nil, err
	}
	if result == nil {
		return nil, nil, nil
	}
	return result.Chunks, append([]knowledge.ChunkID(nil), result.StaleDuringStream...), nil
}

func (c *Compiler) streamSeeds(query retrieval.RetrievalQuery) []knowledge.ChunkID {
	if c.chunkStore == nil {
		return nil
	}
	seen := make(map[knowledge.ChunkID]struct{})
	seeds := make([]knowledge.ChunkID, 0, len(query.Anchors))
	add := func(id knowledge.ChunkID) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		seeds = append(seeds, id)
	}
	for _, anchor := range query.Anchors {
		if id := knowledge.ChunkID(strings.TrimSpace(anchor.ChunkID)); id != "" {
			add(id)
			continue
		}
		anchorID := strings.TrimSpace(anchor.AnchorID)
		term := strings.TrimSpace(anchor.Term)
		if anchorID == "" && term == "" {
			continue
		}
		switch {
		case strings.HasPrefix(anchorID, "file:"):
			path := strings.TrimSpace(strings.TrimPrefix(anchorID, "file:"))
			if path == "" {
				path = term
			}
			chunks, err := c.chunkStore.FindByFilePath(path)
			if err != nil {
				continue
			}
			for _, chunk := range chunks {
				add(chunk.ID)
			}
		case strings.HasPrefix(anchorID, "pin:"):
			path := strings.TrimSpace(strings.TrimPrefix(anchorID, "pin:"))
			if path == "" {
				path = term
			}
			chunks, err := c.chunkStore.FindByFilePath(path)
			if err != nil {
				continue
			}
			for _, chunk := range chunks {
				add(chunk.ID)
			}
		}
	}
	return seeds
}

func streamToRankedChunks(chunks []knowledge.KnowledgeChunk) []retrieval.RankedChunk {
	if len(chunks) == 0 {
		return nil
	}
	out := make([]retrieval.RankedChunk, 0, len(chunks))
	for i, chunk := range chunks {
		out = append(out, retrieval.RankedChunk{
			ChunkID: chunk.ID,
			Rank:    i + 1,
			Score:   float64(len(chunks)-i) / float64(len(chunks)+1),
			Source:  "streamer",
		})
	}
	return out
}

func mergeRankedChunks(primary, secondary []retrieval.RankedChunk) []retrieval.RankedChunk {
	if len(primary) == 0 {
		return append([]retrieval.RankedChunk(nil), secondary...)
	}
	lists := [][]knowledge.ChunkID{
		rankedChunkIDs(primary),
		rankedChunkIDs(secondary),
	}
	weights := []float64{10, 1}
	return retrieval.RRF(lists, weights, 60)
}

func rankedChunkIDs(chunks []retrieval.RankedChunk) []knowledge.ChunkID {
	out := make([]knowledge.ChunkID, 0, len(chunks))
	seen := make(map[knowledge.ChunkID]struct{}, len(chunks))
	for _, chunk := range chunks {
		if chunk.ChunkID == "" {
			continue
		}
		if _, ok := seen[chunk.ChunkID]; ok {
			continue
		}
		seen[chunk.ChunkID] = struct{}{}
		out = append(out, chunk.ChunkID)
	}
	return out
}

func (c *Compiler) applyFilters(ranked []retrieval.RankedChunk) []retrieval.RankedChunk {
	if c.policy == nil || c.chunkStore == nil {
		return ranked
	}

	filtered := make([]retrieval.RankedChunk, 0, len(ranked))
	for _, rc := range ranked {
		chunk, ok, err := c.chunkStore.Load(rc.ChunkID)
		if !ok || err != nil || chunk == nil {
			continue
		}

		// Trust filter - check trust level directly
		if chunk.TrustClass == "" { // Empty trust class means untrusted
			continue
		}

		// Freshness filter
		if chunk.Freshness == knowledge.FreshnessInvalid {
			continue
		}

		filtered = append(filtered, rc)
	}

	return filtered
}

func (c *Compiler) applyBudget(ranked []retrieval.RankedChunk, maxTokens int) ([]retrieval.RankedChunk, int) {
	if maxTokens <= 0 {
		return ranked, 0
	}

	totalTokens := 0
	result := make([]retrieval.RankedChunk, 0, len(ranked))

	for _, rc := range ranked {
		chunkTokens := c.estimateChunkTokens(rc.ChunkID)
		if totalTokens+chunkTokens <= maxTokens {
			result = append(result, rc)
			totalTokens += chunkTokens
		} else {
			// Tail-drop: stop adding chunks
			break
		}
	}

	shortfall := maxTokens - totalTokens
	if shortfall < 0 {
		shortfall = 0
	}

	return result, shortfall
}

func (c *Compiler) estimateTokens(chunks []knowledge.KnowledgeChunk) int {
	total := 0
	for _, chunk := range chunks {
		total += c.estimateChunkTokens(chunk.ID)
	}
	return total
}

func (c *Compiler) estimateChunkTokens(chunkID knowledge.ChunkID) int {
	// Simple estimation: 1 token per 4 characters
	if chunk, ok, err := c.chunkStore.Load(chunkID); ok && err == nil && chunk != nil {
		content := fmt.Sprint(chunk.Body.Fields["content"])
		return len(content) / 4
	}
	return 0
}

func (c *Compiler) getRankerNames(rankers []retrieval.Ranker) []string {
	names := make([]string, 0, len(rankers))
	for _, r := range rankers {
		names = append(names, r.Name())
	}
	return names
}

func (c *Compiler) computeDigest(record *CompilationRecord) string {
	return compilationDigest(record)
}

func compilationDigest(record *CompilationRecord) string {
	h := sha256.New()
	if record == nil {
		return hex.EncodeToString(h.Sum(nil))
	}
	h.Write([]byte(record.Request.Query.Text))
	h.Write([]byte(fmt.Sprintf("%d", record.EventLogSeq)))
	for _, chunkID := range record.Dependencies {
		h.Write([]byte(chunkID))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// trySummarySubstitution attempts to substitute chunks with their summaries to meet budget.
func (c *Compiler) trySummarySubstitution(ctx context.Context, chunks []retrieval.RankedChunk, maxTokens int) ([]retrieval.RankedChunk, []SummarySubstitution) {
	substitutions := make([]SummarySubstitution, 0)
	if c.policy == nil || c.chunkStore == nil {
		return chunks, substitutions
	}

	result := make([]retrieval.RankedChunk, 0, len(chunks))

	for _, rc := range chunks {
		chunk, ok, err := c.chunkStore.Load(rc.ChunkID)
		if !ok || err != nil || chunk == nil {
			continue
		}

		// Check generation cap
		if chunk.DerivationGeneration >= c.maxDerivationGen && c.maxDerivationGen > 0 {
			// Don't summarize chunks already at generation cap
			result = append(result, rc)
			continue
		}

		// Check if summarization is permitted (any configured summarizer is permitted)
		if len(c.policy.Summarizers) == 0 {
			result = append(result, rc)
			continue
		}

		// Look up existing summary via indexed CoverageHash lookup.
		var summaryChunk *knowledge.KnowledgeChunk
		if c.chunkStore != nil && chunk.CoverageHash != "" {
			if chunksByHash, err := c.chunkStore.FindByCoverageHash(chunk.CoverageHash); err == nil {
				for i := range chunksByHash {
					if chunksByHash[i].CoverageHash == chunk.CoverageHash && chunksByHash[i].SourceOrigin == "summary_derivation" {
						summaryChunk = &chunksByHash[i]
						break
					}
				}
			}
		}
		if summaryChunk != nil {
			// Check if summary is stale
			if summaryChunk.Freshness == knowledge.FreshnessStale {
				// Try to regenerate if auto-summarize is enabled
				if c.autoSummarize && len(c.summarizers) > 0 {
					summaryChunk = c.generateAndPersistSummary(ctx, []knowledge.KnowledgeChunk{*chunk})
				} else {
					// Keep original chunk
					result = append(result, rc)
					continue
				}
			}

			// Substitute with summary
			originalTokens := c.estimateChunkTokens(rc.ChunkID)
			summaryTokens := c.estimateChunkTokens(summaryChunk.ID)
			savings := originalTokens - summaryTokens

			result = append(result, retrieval.RankedChunk{
				ChunkID: summaryChunk.ID,
				Score:   rc.Score, // Preserve original score
			})

			substitutions = append(substitutions, SummarySubstitution{
				OriginalChunkID: rc.ChunkID,
				SummaryChunkID:  summaryChunk.ID,
				Reason:          "budget_pressure",
				TokenSavings:    savings,
			})
		} else if c.autoSummarize && len(c.summarizers) > 0 {
			// No summary exists - generate on-demand
			summaryChunk = c.generateAndPersistSummary(ctx, []knowledge.KnowledgeChunk{*chunk})
			if summaryChunk != nil {
				originalTokens := c.estimateChunkTokens(rc.ChunkID)
				summaryTokens := c.estimateChunkTokens(summaryChunk.ID)
				savings := originalTokens - summaryTokens

				result = append(result, retrieval.RankedChunk{
					ChunkID: summaryChunk.ID,
					Score:   rc.Score,
				})

				substitutions = append(substitutions, SummarySubstitution{
					OriginalChunkID: rc.ChunkID,
					SummaryChunkID:  summaryChunk.ID,
					Reason:          "budget_pressure",
					TokenSavings:    savings,
				})
			} else {
				// Keep original chunk
				result = append(result, rc)
			}
		} else {
			// No summary and auto-summarize disabled
			result = append(result, rc)
		}
	}

	return result, substitutions
}

// generateAndPersistSummary generates a summary and persists it.
func (c *Compiler) generateAndPersistSummary(ctx context.Context, chunks []knowledge.KnowledgeChunk) *knowledge.KnowledgeChunk {
	if len(c.summarizers) == 0 || c.persistenceWriter == nil {
		return nil
	}

	// Route to appropriate summarizer
	result, err := summarization.Route(ctx, chunks, 0, c.summarizers, c.policy)
	if err != nil {
		return nil
	}

	// Build source chunk IDs
	sourceIDs := make([]knowledge.ChunkID, 0, len(chunks))
	for _, c := range chunks {
		sourceIDs = append(sourceIDs, c.ID)
	}

	// Calculate next generation
	maxGen := 0
	for _, chunk := range chunks {
		if chunk.DerivationGeneration > maxGen {
			maxGen = chunk.DerivationGeneration
		}
	}

	// Create summary chunk
	summaryChunk := knowledge.KnowledgeChunk{
		ID:                   knowledge.ChunkID(c.newID()),
		CoverageHash:         result.CoverageHash,
		SourceOrigin:         "summary_derivation",
		DerivedFrom:          sourceIDs,
		DerivationGeneration: maxGen + 1,
		Body: knowledge.ChunkBody{
			Fields: map[string]any{"content": result.Summary},
		},
		AcquiredAt: c.now(),
		Freshness:  knowledge.FreshnessValid,
	}

	// Persist via persistence writer
	_, err = c.persistenceWriter.Persist(ctx, persistence.PersistenceRequest{
		Content:      []byte(result.Summary),
		ContentType:  "summary",
		SourceOrigin: "summary_derivation",
		DerivedFrom:  sourceIDs,
	})
	if err != nil {
		return nil
	}

	// Save to chunk store
	saved, err := c.chunkStore.Save(summaryChunk)
	if err != nil {
		return nil
	}

	return saved
}

func (c *Compiler) computeDiff(original, current *CompilationResult) *CompilationDiff {
	diff := &CompilationDiff{
		FreshnessDelta: make(map[knowledge.ChunkID]knowledge.FreshnessState),
	}

	originalIDs := make(map[knowledge.ChunkID]struct{})
	for _, rc := range original.RankedChunks {
		originalIDs[rc.ChunkID] = struct{}{}
	}

	currentIDs := make(map[knowledge.ChunkID]struct{})
	for _, rc := range current.RankedChunks {
		currentIDs[rc.ChunkID] = struct{}{}
		if _, existed := originalIDs[rc.ChunkID]; !existed {
			diff.AddedChunks = append(diff.AddedChunks, rc.ChunkID)
		}
	}

	for _, rc := range original.RankedChunks {
		if _, stillExists := currentIDs[rc.ChunkID]; !stillExists {
			diff.RemovedChunks = append(diff.RemovedChunks, rc.ChunkID)
		}
	}

	// Check for reordering
	if len(original.RankedChunks) == len(current.RankedChunks) {
		for i := range original.RankedChunks {
			if original.RankedChunks[i].ChunkID != current.RankedChunks[i].ChunkID {
				diff.Reordered = true
				break
			}
		}
	} else {
		diff.Reordered = true
	}

	diff.TokenChange = current.TotalTokens - original.TotalTokens

	return diff
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// SetSummarizers sets the summarizers for on-demand summarization.
func (c *Compiler) SetSummarizers(summarizers []summarization.Summarizer) {
	c.summarizers = summarizers
}

// SetPersistenceWriter sets the persistence writer for saving summaries.
func (c *Compiler) SetPersistenceWriter(writer *persistence.Writer) {
	c.persistenceWriter = writer
}

// SetMaxDerivationGen sets the maximum derivation generation cap.
func (c *Compiler) SetMaxDerivationGen(maxGen int) {
	c.maxDerivationGen = maxGen
}

// SetAutoSummarize enables/disables auto-summarization on budget pressure.
func (c *Compiler) SetAutoSummarize(auto bool) {
	c.autoSummarize = auto
}

// Diff produces a structured diff between two compilation records.
func (c *Compiler) Diff(a, b *CompilationRecord) *CompilationDiff {
	if a == nil || b == nil {
		return nil
	}
	return c.computeDiff(&a.Result, &b.Result)
}

// DiffByID produces a structured diff between two compilations by their IDs.
func (c *Compiler) DiffByID(ctx context.Context, idA, idB string) (*CompilationDiff, error) {
	recordA, err := c.LoadCompilationRecord(ctx, idA)
	if err != nil {
		return nil, fmt.Errorf("load record A: %w", err)
	}
	recordB, err := c.LoadCompilationRecord(ctx, idB)
	if err != nil {
		return nil, fmt.Errorf("load record B: %w", err)
	}
	return c.Diff(recordA, recordB), nil
}

// persistCompilationRecord persists a compilation record to the knowledge store.
func (c *Compiler) persistCompilationRecord(ctx context.Context, record *CompilationRecord) error {
	if c.persistenceWriter == nil {
		return fmt.Errorf("persistence writer not configured")
	}

	// Serialize record to JSON
	content, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	req := persistence.PersistenceRequest{
		Content:      content,
		ContentType:  "compilation_record",
		SourceOrigin: "compilation_record",
		Tags:         []string{"compilation", "replayable"},
		Reason:       fmt.Sprintf("Compilation %s at seq %d", record.RequestID, record.EventLogSeq),
	}

	_, err = c.persistenceWriter.Persist(ctx, req)
	if err != nil {
		return fmt.Errorf("persist record: %w", err)
	}

	return nil
}

func isSpeculativeCompilation(metadata map[string]any) bool {
	if len(metadata) == 0 {
		return false
	}
	value, ok := metadata["speculative"]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func (c *Compiler) emitWarning(message string, metadata map[string]any) {
	if c == nil || c.telemetry == nil {
		return
	}
	c.telemetry.Emit(core.Event{
		Type:      core.EventCompilerWarning,
		Message:   message,
		Timestamp: c.now(),
		Metadata:  metadata,
	})
}

// LoadCompilationRecord loads a compilation record by ID from the knowledge store.
func (c *Compiler) LoadCompilationRecord(ctx context.Context, compilationID string) (*CompilationRecord, error) {
	if c.chunkStore == nil {
		return nil, fmt.Errorf("chunk store not configured")
	}

	// Search for chunks with compilation_record source origin and matching request ID
	chunks, err := c.chunkStore.FindAll()
	if err != nil {
		return nil, fmt.Errorf("find chunks: %w", err)
	}

	for _, chunk := range chunks {
		if chunk.SourceOrigin != "compilation_record" {
			continue
		}

		// Parse the record
		var record CompilationRecord
		content, ok := chunk.Body.Fields["content"]
		if !ok {
			// Try Raw field
			content = chunk.Body.Raw
		}

		var data []byte
		switch v := content.(type) {
		case string:
			data = []byte(v)
		case []byte:
			data = v
		default:
			continue
		}

		if err := json.Unmarshal(data, &record); err != nil {
			continue // Skip malformed records
		}

		if record.RequestID == compilationID {
			return &record, nil
		}
	}

	return nil, fmt.Errorf("compilation record not found: %s", compilationID)
}

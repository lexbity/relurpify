package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/context/persistence"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
	"codeburg.org/lexbit/relurpify/execution/prompt/summarization"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	// MaxActivePins caps how many files can be pinned simultaneously.
	MaxActivePins = 8
	// PinRefTokenBudget is the fixed per-pin token budget for a reference chunk.
	PinRefTokenBudget = 64
)

// UnlimitedBudget is the explicit "no budget" sentinel: a request with
// MaxTokens <= 0 admits ranked content in order, and the opt-in is logged at
// request assembly so it can never be silent.
const UnlimitedBudget = 0

// ErrCompilerStopped is returned by Start after the compiler has been
// stopped: a stopped compiler owns no invalidation loop and is not
// restartable (no zombie loops).
var ErrCompilerStopped = errors.New("compiler stopped")

// ErrReplayMismatch is returned by StrictReplay when recomputing the
// compilation (cache bypassed) yields a different deterministic digest than
// the recorded one — the only honest proof of non-determinism.
var ErrReplayMismatch = errors.New("replay digest mismatch")

// Compiler performs live context assembly with caching and event-driven invalidation.
type Compiler struct {
	retriever  *retrieval.Retriever
	streamer   *knowledge.Streamer
	policy     *execctx.ContextPolicyBundle
	chunkStore *knowledge.ChunkStore
	cache      compilationCache
	eventBus   *knowledge.EventBus
	repository Repository
	telemetry  telemetry.Telemetry
	newID      func() string
	now        func() time.Time

	// Lifecycle: Start is once-only, Start-after-Stop is an error, and the
	// invalidation loop exits exactly once on Stop.
	lifecycleMu sync.Mutex
	started     bool
	stopped     bool
	stopCh      chan struct{}
	loopDone    chan struct{}
	unsubscribe func()

	// Write direction components
	summarizers       []summarization.Summarizer
	persistenceWriter *persistence.Writer
	maxDerivationGen  int  // Generation cap for summarization
	autoSummarize     bool // Auto-summarize on budget pressure
}

// NewCompiler creates a new compiler instance.
func NewCompiler(retriever *retrieval.Retriever, policy *execctx.ContextPolicyBundle, store *knowledge.ChunkStore, streamer ...*knowledge.Streamer) *Compiler {
	var stream *knowledge.Streamer
	if len(streamer) > 0 {
		stream = streamer[0]
	}
	c := &Compiler{
		retriever:  retriever,
		streamer:   stream,
		policy:     policy,
		chunkStore: store,
		newID:      generateID,
		now:        time.Now,
	}
	c.cache = newCompilationCache(c.now)
	return c
}

// SetStreamer wires the dependency-ordered streaming path used for compile seeding.
func (c *Compiler) SetStreamer(streamer *knowledge.Streamer) {
	c.streamer = streamer
}

// SetEventBus wires the knowledge event bus whose chunk events drive cache
// invalidation (chunk ingested supersedes prior chunks; chunk staled removes
// them from consideration).
func (c *Compiler) SetEventBus(bus *knowledge.EventBus) {
	c.eventBus = bus
}

// SetRepository wires the O(1) compilation-record repository. Records are
// persisted and loaded exclusively through it.
func (c *Compiler) SetRepository(repo Repository) {
	c.repository = repo
}

// SetTelemetry wires structured compiler warnings and observability events.
func (c *Compiler) SetTelemetry(telemetry telemetry.Telemetry) {
	c.telemetry = telemetry
}

// SetIDGenerator sets the ID generator function.
func (c *Compiler) SetIDGenerator(fn func() string) {
	c.newID = fn
}

// SetTimeFunc sets the time function (deterministic tests). Installs the
// same clock on the compilation cache so TTL semantics follow.
func (c *Compiler) SetTimeFunc(fn func() time.Time) {
	c.now = fn
	c.cache.setClock(fn)
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
	return c.compile(ctx, request, compileOpts{})
}

// compileOpts carries internal compile switches. noCache bypasses the cache
// read AND write: replay verification must recompute, because a cache hit
// proves nothing.
type compileOpts struct {
	noCache bool
}

func (c *Compiler) compile(ctx context.Context, request CompilationRequest, opts compileOpts) (*CompilationResult, *CompilationRecord, error) {
	// A non-positive budget is an explicit unlimited opt-in; it is logged so
	// the absence of a budget can never be silent.
	if request.MaxTokens <= UnlimitedBudget {
		c.emitWarning(ctx, "unlimited_budget", map[string]any{"max_tokens": request.MaxTokens})
	}

	// Build cache key
	cacheKey := c.buildCacheKey(request)

	// Check cache first
	if !opts.noCache {
		if cached := c.cache.get(cacheKey); cached != nil {
			result := &CompilationResult{
				Chunks:       cached.record.Result.Chunks,
				RankedChunks: cached.record.Result.RankedChunks,
				TotalTokens:  cached.record.Result.TotalTokens,
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

	// Stage 5: Pin reference floor
	pinAnchors := extractPinAnchors(request.Query.Anchors)
	var pinRefs []PinReference
	var evictedPinContent []string
	var pinPaths map[string]struct{}
	if len(pinAnchors) > 0 {
		if len(pinAnchors) > MaxActivePins {
			for _, dropped := range pinAnchors[MaxActivePins:] {
				path := anchorFilePath(dropped)
				c.emitWarning(ctx, "pin_cap_exceeded", map[string]any{"path": path})
			}
			pinAnchors = pinAnchors[:MaxActivePins]
		}
		pinPaths = pinPathsFromAnchors(pinAnchors)
		for _, pa := range pinAnchors {
			path := anchorFilePath(pa)
			if path == "" {
				continue
			}
			pinRefs = append(pinRefs, c.buildPinReference(path))
		}
		// Build set of chunk IDs for pinned files and boost their rank.
		pinContentIDs := c.collectPinContentIDs(pinPaths)
		rankedChunks = boostPinContentRank(rankedChunks, pinContentIDs)
	}
	filteredChunks := c.applyFilters(rankedChunks)

	// Stage 6: Budget fitting (tail-drop) with pin reservation. Pins keep
	// their floor; content fits into max(0, maxTokens-reserved). Content and
	// pins together never exceed maxTokens unless the pins alone do, in
	// which case the pins win and the overflow is counted.
	contentBudget, pinOverflow := applyPinReservedBudget(request.MaxTokens, len(pinRefs))
	if pinOverflow > 0 {
		c.emitWarning(ctx, "pin_reserved_overflow", map[string]any{"overflow_tokens": pinOverflow})
	}
	finalChunks, contentShortfall := c.applyBudget(ctx, filteredChunks, contentBudget)

	// Track content chunks evicted by budget for pinned files.
	var pinContentIDs map[knowledge.ChunkID]struct{}
	if len(pinAnchors) > 0 {
		pinContentIDs = c.collectPinContentIDs(pinPaths)
	}
	if len(pinContentIDs) > 0 && contentShortfall > 0 {
		inFinal := make(map[knowledge.ChunkID]struct{}, len(finalChunks))
		for _, fc := range finalChunks {
			inFinal[fc.ChunkID] = struct{}{}
		}
		for cid := range pinContentIDs {
			if _, kept := inFinal[cid]; !kept {
				// We know the path from pinPaths; report eviction per pinned path.
				for path := range pinPaths {
					if evictedPinContent == nil || !containsString(evictedPinContent, path) {
						evictedPinContent = append(evictedPinContent, path)
						c.emitWarning(ctx, "pin_content_evicted", map[string]any{"path": path})
					}
				}
				break
			}
		}
	}

	// Stage 6b: Summary substitution for budget pressure
	substitutions := make([]SummarySubstitution, 0)
	if contentShortfall > 0 && len(finalChunks) > 0 {
		substitutedChunks, subs := c.trySummarySubstitution(ctx, finalChunks, contentBudget)
		finalChunks = substitutedChunks
		substitutions = subs
		_, contentShortfall = c.applyBudget(ctx, finalChunks, contentBudget)
	}

	// Build result
	result := &CompilationResult{
		RankedChunks:       finalChunks,
		SkippedStaleChunks: skippedStaleChunks,
		ShortfallTokens:    contentShortfall,
		Substitutions:      substitutions,
		PinReferences:      pinRefs,
		EvictedPinContent:  evictedPinContent,
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
		BudgetShortfall: contentShortfall,
		AssemblyMetadata: contextdata.AssemblyMeta{
			CompilationID:   c.newID(),
			EventLogSeq:     request.EventLogSeq,
			BudgetTokens:    request.MaxTokens,
			ShortfallTokens: contentShortfall,
			AssembledAt:     c.now(),
		},
	}

	// Compute deterministic digest
	record.DeterministicDigest = c.computeDigest(record)

	// Add to cache (skipped when the compile bypasses it: replay verification
	// results are not reusable answers).
	if !opts.noCache {
		c.cache.put(cacheKey, &cacheEntry{
			record:     *record,
			deps:       dependencySet(dependencies),
			insertedAt: c.now(),
		})
	}

	if !isSpeculativeCompilation(request.Metadata) {
		if err := c.persistCompilationRecord(ctx, record); err != nil {
			c.emitWarning(ctx, "compilation persistence failed", map[string]any{
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
		// Reconstruct state at original EventLogSeq and re-run with the
		// cache bypassed: only recomputation can fail, a cache hit proves
		// nothing.
		request := originalRecord.Request
		request.EventLogSeq = originalRecord.EventLogSeq
		result, newRecord, err := c.compile(ctx, request, compileOpts{noCache: true})
		if err != nil {
			return nil, nil, nil, err
		}
		if newRecord.DeterministicDigest != originalRecord.DeterministicDigest {
			return nil, nil, nil, fmt.Errorf("%w: record %s recomputed to %s (recorded %s)",
				ErrReplayMismatch, originalRecord.RequestID, newRecord.DeterministicDigest, originalRecord.DeterministicDigest)
		}

		diff := c.computeDiff(&originalRecord.Result, result)
		diff.DeterminismMatch = true
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

// Start begins the invalidation loop and subscribes to knowledge events.
// Start is once-only; Start after Stop returns ErrCompilerStopped (a stopped
// compiler owns no loop and is not restartable).
func (c *Compiler) Start(_ context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.stopped {
		return ErrCompilerStopped
	}
	if c.started {
		return fmt.Errorf("compiler already started")
	}
	c.started = true
	c.stopCh = make(chan struct{})
	c.loopDone = make(chan struct{})

	// Subscribe to chunk lifecycle events. The bus is lossy by design; the
	// invalidated-set recheck, TTL, and capacity bounds backstop dropped
	// events.
	if c.eventBus != nil {
		events, cancel := c.eventBus.Subscribe(64)
		c.unsubscribe = cancel
		go c.consumeEvents(events)
	}

	go c.invalidationLoop()

	return nil
}

// Stop halts event consumption and the invalidation loop, and waits
// (bounded) for the loop's exit. Safe to call more than once.
func (c *Compiler) Stop() {
	c.lifecycleMu.Lock()
	if !c.started || c.stopped {
		c.lifecycleMu.Unlock()
		return
	}
	c.stopped = true
	stopCh := c.stopCh
	unsubscribe := c.unsubscribe
	c.lifecycleMu.Unlock()

	if unsubscribe != nil {
		unsubscribe()
	}
	close(stopCh)
	// Deterministic join (bounded): the loop's select observes the closed
	// stop channel immediately.
	select {
	case <-c.loopDone:
	case <-time.After(time.Second):
	}
}

// consumeEvents translates knowledge events into cache invalidation until
// the subscription closes.
func (c *Compiler) consumeEvents(events <-chan knowledge.Event) {
	for event := range events {
		switch event.Kind {
		case knowledge.EventChunkIngested:
			if payload, ok := event.Payload.(knowledge.ChunkIngestedPayload); ok && payload.ChunkID != "" {
				c.handleChunkInvalidated(knowledge.ChunkID(payload.ChunkID))
			}
		case knowledge.EventChunkStaled:
			if payload, ok := event.Payload.(knowledge.ChunkStaledPayload); ok {
				for _, id := range payload.ChunkIDs {
					if id != "" {
						c.handleChunkInvalidated(knowledge.ChunkID(id))
					}
				}
			}
		}
	}
}

// handleChunkInvalidated records a superseded or staled chunk and evicts
// every cached compilation that depends on it.
func (c *Compiler) handleChunkInvalidated(chunkID knowledge.ChunkID) {
	c.cache.markInvalidated(chunkID)
	c.emitCacheEvent("compilation_cache_invalidated", map[string]any{"chunk_id": string(chunkID)})
}

// handlePolicyReloaded voids every cached compilation: verdicts computed
// under the old policy are stale wholesale.
func (c *Compiler) handlePolicyReloaded() {
	c.cache.reset()
	c.emitCacheEvent("compilation_cache_invalidated", map[string]any{"reason": "policy_reloaded"})
}

// invalidationLoop periodically sweeps TTL-expired entries. Push invalidation
// already removed live staleness; the sweep is the backstop.
func (c *Compiler) invalidationLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer close(c.loopDone)
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cache.entries.Sweep()
		}
	}
}

// emitCacheEvent surfaces cache lifecycle transitions as telemetry; each is
// counted on the compiler even without a telemetry sink (the counter is the
// contract).
func (c *Compiler) emitCacheEvent(kind string, metadata map[string]any) {
	if c.telemetry == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventType(kind),
		Message:   kind,
		Timestamp: c.now(),
		Metadata:  metadata,
	}
	c.telemetry.Emit(ev)
}

// Private helper methods

func (c *Compiler) buildCacheKey(request CompilationRequest) CacheKey {
	return CacheKey{
		QueryFingerprint:        c.fingerprint(mustJSON(request.Query)),
		ManifestFingerprint:     c.fingerprint(request.ManifestID),
		PolicyBundleFingerprint: c.fingerprint(request.PolicyBundleID),
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

func (c *Compiler) applyBudget(ctx context.Context, ranked []retrieval.RankedChunk, maxTokens int) ([]retrieval.RankedChunk, int) {
	if maxTokens <= UnlimitedBudget {
		// Explicit no-budget opt-in: admit ranked content in order.
		return ranked, 0
	}

	totalTokens := 0
	result := make([]retrieval.RankedChunk, 0, len(ranked))

	for _, rc := range ranked {
		chunkTokens := c.estimateChunkTokens(rc.ChunkID)
		if chunkTokens == 0 {
			// Empty-content chunks are excluded from the budget and the
			// context: they carry nothing and would otherwise ride along
			// uncounted.
			c.emitWarning(ctx, "content_gap", map[string]any{
				"chunk_id": string(rc.ChunkID),
				"reason":   "empty_content",
			})
			continue
		}
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

// estimateChunkTokens estimates a chunk's token cost. Content comes from
// Body.Fields["content"] when present and from Body.Raw otherwise (Raw-only
// chunks previously estimated as the literal "<nil>" — one token — silently
// voiding the budget). Empty content estimates zero: the budget path
// excludes such chunks with a gap message. Otherwise ceil(chars/4), minimum 1.
func (c *Compiler) estimateChunkTokens(chunkID knowledge.ChunkID) int {
	chunk, ok, err := c.chunkStore.Load(chunkID)
	if !ok || err != nil || chunk == nil {
		return 0
	}
	content := chunkContent(chunk)
	if len(content) == 0 {
		return 0
	}
	tokens := (len(content) + 3) / 4
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// chunkContent resolves a chunk's estimable content: the typed content
// field when it is a string, else the raw body.
func chunkContent(chunk *knowledge.KnowledgeChunk) string {
	if chunk == nil {
		return ""
	}
	if content, ok := chunk.Body.Fields["content"].(string); ok && content != "" {
		return content
	}
	return chunk.Body.Raw
}

// dependencySet builds the dependency set for a compilation's chunk IDs.
func dependencySet(dependencies []knowledge.ChunkID) map[knowledge.ChunkID]struct{} {
	deps := make(map[knowledge.ChunkID]struct{}, len(dependencies))
	for _, chunkID := range dependencies {
		deps[chunkID] = struct{}{}
	}
	return deps
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
	_, _ = fmt.Fprintf(h, "%d", record.EventLogSeq)
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
	saved, err := c.chunkStore.Save(ctx, summaryChunk)
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

// persistCompilationRecord persists a compilation record through the O(1)
// repository, keyed by its request ID.
func (c *Compiler) persistCompilationRecord(ctx context.Context, record *CompilationRecord) error {
	if c.repository == nil {
		return fmt.Errorf("repository not configured")
	}
	if err := c.repository.StoreCompilationRecord(ctx, *record); err != nil {
		return fmt.Errorf("persist record: %w", err)
	}
	return nil
}

// extractPinAnchors collects unique pin anchors from the query.
// A pin anchor has Class "session_pin".
func extractPinAnchors(anchors []retrieval.AnchorRef) []retrieval.AnchorRef {
	var pins []retrieval.AnchorRef
	seen := make(map[string]struct{})
	for _, a := range anchors {
		if a.Class != "session_pin" {
			continue
		}
		key := a.AnchorID
		if key == "" {
			key = a.Term
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		pins = append(pins, a)
	}
	return pins
}

// anchorFilePath extracts the file path from a pin anchor.
func anchorFilePath(a retrieval.AnchorRef) string {
	path := strings.TrimPrefix(a.AnchorID, "pin:")
	if path == "" {
		path = a.Term
	}
	return strings.TrimSpace(path)
}

// buildPinReference creates a bounded reference chunk for a pinned file.
// It uses the file path and any content hash found in the store to produce
// a short digest. The reference is always smaller than pinRefTokenBudget.
func (c *Compiler) buildPinReference(path string) PinReference {
	ref := PinReference{Path: path, TokenEstimate: PinRefTokenBudget}

	if c.chunkStore == nil {
		return ref
	}
	chunks, err := c.chunkStore.FindByFilePath(path)
	if err != nil {
		return ref
	}
	for _, ch := range chunks {
		if ch.ContentHash != "" {
			ref.ContentHash = ch.ContentHash
		}
		body := strings.TrimSpace(ch.Body.Raw)
		if body != "" {
			if len(body) > 120 {
				body = body[:120]
			}
			ref.ShortDigest = body
			break
		}
	}
	return ref
}

// collectPinContentIDs builds a set of chunk IDs belonging to pinned file paths.
func (c *Compiler) collectPinContentIDs(pinPaths map[string]struct{}) map[knowledge.ChunkID]struct{} {
	if c.chunkStore == nil || len(pinPaths) == 0 {
		return nil
	}
	ids := make(map[knowledge.ChunkID]struct{})
	for path := range pinPaths {
		chunks, err := c.chunkStore.FindByFilePath(path)
		if err != nil {
			continue
		}
		for _, ch := range chunks {
			ids[ch.ID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// boostPinContentRank gives a score premium to chunks belonging to pinned files
// so they rank higher at equal budget.
func boostPinContentRank(ranked []retrieval.RankedChunk, pinContentIDs map[knowledge.ChunkID]struct{}) []retrieval.RankedChunk {
	if len(pinContentIDs) == 0 {
		return ranked
	}
	out := make([]retrieval.RankedChunk, len(ranked))
	copy(out, ranked)
	for i, rc := range out {
		if _, pinned := pinContentIDs[rc.ChunkID]; pinned {
			out[i].Score += 1000.0
		}
	}
	return out
}

// pinPathsFromAnchors extracts pin file paths from anchors.
func pinPathsFromAnchors(pins []retrieval.AnchorRef) map[string]struct{} {
	paths := make(map[string]struct{}, len(pins))
	for _, p := range pins {
		path := anchorFilePath(p)
		if path != "" {
			paths[path] = struct{}{}
		}
	}
	return paths
}

// applyPinReservedBudget subtracts the pin-reference floor from maxTokens
// and returns the content budget plus the overflow when the pins alone
// exceed the budget (pins win; the excess is the caller's to count).
func applyPinReservedBudget(maxTokens int, pinCount int) (contentBudget int, overflow int) {
	reserved := pinCount * PinRefTokenBudget
	if reserved <= maxTokens {
		return maxTokens - reserved, 0
	}
	return 0, reserved - maxTokens
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
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

func (c *Compiler) emitWarning(ctx context.Context, message string, metadata map[string]any) {
	if c == nil || c.telemetry == nil {
		return
	}
	ev := telemetry.Event{
		Type:      telemetry.EventCompilerWarning,
		Message:   message,
		Timestamp: c.now(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	c.telemetry.Emit(ev)
}

// ListCompilationRecords returns the persisted compilation records.
func (c *Compiler) ListCompilationRecords(ctx context.Context) ([]CompilationRecord, error) {
	if c.repository == nil {
		return nil, fmt.Errorf("repository not configured")
	}
	return c.repository.ListCompilationRecords(ctx, 0)
}

// LoadCompilationRecord loads a compilation record by ID through the O(1)
// repository path. Non-ID locators are a compile-time error by construction:
// callers hold request IDs.
func (c *Compiler) LoadCompilationRecord(ctx context.Context, compilationID string) (*CompilationRecord, error) {
	if c.repository == nil {
		return nil, fmt.Errorf("repository not configured")
	}
	record, err := c.repository.GetCompilationRecord(ctx, compilationID)
	if err != nil {
		return nil, fmt.Errorf("load compilation record: %w", err)
	}
	return record, nil
}

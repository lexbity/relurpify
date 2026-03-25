package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/perfstats"
)

// RetrieverService is the runtime-facing retrieval interface.
type RetrieverService interface {
	Retrieve(ctx context.Context, q RetrievalQuery) ([]core.ContentBlock, RetrievalEvent, error)
}

// Service composes retrieval, packing, audit logging, and optional telemetry.
type Service struct {
	db        *sql.DB
	retriever *Retriever
	packer    *ContextPacker
	telemetry core.Telemetry
	cache     *exactCache
	hotWindow time.Duration
	hotLimit  int
	now       func() time.Time
	schemaErr error
}

// NewService constructs a runtime-facing retrieval service.
func NewService(db *sql.DB, embedder Embedder, telemetry core.Telemetry) *Service {
	return NewServiceWithOptions(db, embedder, telemetry, ServiceOptions{})
}

// NewServiceWithOptions constructs a runtime-facing retrieval service with tier controls.
func NewServiceWithOptions(db *sql.DB, embedder Embedder, telemetry core.Telemetry, opts ServiceOptions) *Service {
	hotWindow := opts.HotWindow
	if hotWindow <= 0 {
		hotWindow = defaultHotWindow
	}
	hotLimit := opts.HotLimit
	if hotLimit <= 0 {
		hotLimit = defaultHotChunkCap
	}
	return &Service{
		db:        db,
		retriever: NewRetriever(db, embedder),
		packer:    NewContextPacker(db),
		telemetry: telemetry,
		cache:     newExactCache(opts.Cache),
		hotWindow: hotWindow,
		hotLimit:  hotLimit,
		now:       func() time.Time { return time.Now().UTC() },
		schemaErr: ensureRuntimeSchema(context.Background(), db),
	}
}

// Retrieve executes the retrieval pipeline and returns packed content blocks.
func (s *Service) Retrieve(ctx context.Context, q RetrievalQuery) ([]core.ContentBlock, RetrievalEvent, error) {
	if s == nil || s.db == nil || s.retriever == nil || s.packer == nil {
		return nil, RetrievalEvent{}, errors.New("retrieval service not configured")
	}
	if s.schemaErr != nil {
		return nil, RetrievalEvent{}, s.schemaErr
	}
	now := s.now()
	corpusStamp, err := s.corpusStamp(ctx)
	if err != nil {
		return nil, RetrievalEvent{}, err
	}
	cacheKey := exactCacheKey(q, corpusStamp)
	if blocks, cachedEvent, ok := s.cache.get(cacheKey, now); ok {
		cachedEvent.QueryID = newQueryID(q, now)
		cachedEvent.Timestamp = now
		cachedEvent.CacheTier = "l1_exact"
		emitRetrievalTelemetry(s.telemetry, cachedEvent, nil)
		return blocks, cachedEvent, nil
	}

	queryID := newQueryID(q, now)
	retrievalResult, cacheTier, err := s.retrieveCandidates(ctx, q, now)
	if err != nil {
		return nil, RetrievalEvent{}, err
	}
	packing, err := s.packer.Pack(ctx, retrievalResult.Fused, PackingOptions{
		MaxTokens: q.MaxTokens,
		MaxChunks: q.Limit,
	})
	if err != nil {
		return nil, RetrievalEvent{}, err
	}

	retrievalEvent := RetrievalEvent{
		QueryID:          queryID,
		Query:            q.Text,
		FilterSummary:    retrievalResult.Prefilter.FilterSummary,
		SparseCandidates: len(retrievalResult.Sparse),
		DenseCandidates:  len(retrievalResult.Dense),
		FusedCandidates:  len(retrievalResult.Fused),
		ExcludedReasons:  mergeExcludedReasons(retrievalResult.ExcludedReasons, packing.DroppedChunks),
		CacheTier:        cacheTier,
		Timestamp:        now,
	}
	packingEvent := PackingEvent{
		QueryID:        queryID,
		InjectedChunks: append([]string{}, packing.InjectedChunks...),
		DroppedChunks:  cloneMapString(packing.DroppedChunks),
		TokenBudget:    packing.TokenBudget,
		TokensUsed:     packing.TokensUsed,
		Timestamp:      now,
	}
	if err := persistRetrievalEvent(ctx, s.db, retrievalEvent); err != nil {
		return nil, RetrievalEvent{}, err
	}
	if err := persistPackingEvent(ctx, s.db, packingEvent); err != nil {
		return nil, RetrievalEvent{}, err
	}
	emitRetrievalTelemetry(s.telemetry, retrievalEvent, &packingEvent)
	s.cache.set(cacheKey, packing.Blocks, retrievalEvent, now)
	return packing.Blocks, retrievalEvent, nil
}

func (s *Service) retrieveCandidates(ctx context.Context, q RetrievalQuery, now time.Time) (*RetrievalResult, string, error) {
	fullResult, err := s.retriever.RetrieveCandidates(ctx, q)
	if err != nil {
		return nil, "", err
	}
	if len(q.AllowChunkIDs) > 0 {
		return fullResult, "l3_main", nil
	}
	hotIDs, err := recentHotChunkIDs(ctx, s.db, s.hotWindow, s.hotLimit, now)
	if err != nil {
		return nil, "", err
	}
	if len(hotIDs) == 0 {
		return fullResult, "l3_main", nil
	}

	hotQuery := q
	hotQuery.AllowChunkIDs = hotIDs
	hotResult, err := s.retriever.RetrieveCandidates(ctx, hotQuery)
	if err != nil {
		return nil, "", err
	}
	if q.Limit <= 0 || len(hotResult.Fused) < q.Limit {
		return fullResult, "l3_main", nil
	}
	if sameLeadingCandidates(hotResult.Fused, fullResult.Fused, q.Limit) {
		return hotResult, "l2_hot", nil
	}
	return fullResult, "l3_main", nil
}

func sameLeadingCandidates(a, b []RankedCandidate, n int) bool {
	if n <= 0 {
		return false
	}
	if len(a) < n || len(b) < n {
		return false
	}
	for i := 0; i < n; i++ {
		if a[i].ChunkID != b[i].ChunkID || a[i].VersionID != b[i].VersionID {
			return false
		}
	}
	return true
}

func cloneMapString(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func mergeExcludedReasons(retrievalExcluded, packingDropped map[string]string) map[string]string {
	merged := cloneMapString(retrievalExcluded)
	if merged == nil && len(packingDropped) == 0 {
		return nil
	}
	if merged == nil {
		merged = make(map[string]string, len(packingDropped))
	}
	for chunkID, reason := range packingDropped {
		appendExcludedReason(merged, chunkID, "packing:"+reason)
	}
	return merged
}

func (s *Service) corpusStamp(ctx context.Context) (string, error) {
	perfstats.IncRetrievalCorpusStamp()
	return currentCorpusRevision(ctx, s.db)
}

func exactCacheKey(q RetrievalQuery, corpusStamp string) string {
	parts := []string{
		q.Text,
		normalizeScope(q.Scope),
		corpusStamp,
		strings.Join(sortedStrings(q.SourceTypes), ","),
		strings.Join(sortedStrings(q.PolicyTags), ","),
		strings.Join(sortedStrings(q.AllowChunkIDs), ","),
		strconv.Itoa(q.MaxTokens),
		strconv.Itoa(q.Limit),
	}
	if q.UpdatedAfter != nil {
		parts = append(parts, q.UpdatedAfter.UTC().Format(time.RFC3339Nano))
	}
	return "rc:" + shortStableHash(strings.Join(parts, "\n"))
}

func sortedStrings(values []string) []string {
	out := cloneStrings(values)
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

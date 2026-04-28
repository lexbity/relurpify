package retrieval

import (
	"context"
	"fmt"
	"sort"

	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/search"
)

// BM25Ranker wraps framework/search.SearchEngine for BM25 ranking.
type BM25Ranker struct {
	engine *search.SearchEngine
}

// NewBM25Ranker creates a new BM25 ranker.
func NewBM25Ranker(engine *search.SearchEngine) *BM25Ranker {
	return &BM25Ranker{engine: engine}
}

// Name returns the ranker name.
func (r *BM25Ranker) Name() string {
	return "bm25"
}

// Rank performs BM25 ranking using the search engine.
func (r *BM25Ranker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	if r.engine == nil {
		return nil, nil
	}

	// Use keyword search mode for BM25-style ranking
	searchQuery := search.SearchQuery{
		Text:       query.Text,
		Mode:       search.SearchKeyword,
		MaxResults: query.Limit,
	}

	results, err := r.engine.Search(searchQuery)
	if err != nil {
		return nil, fmt.Errorf("bm25 search failed: %w", err)
	}

	// Convert SearchResult ChunkIDs to knowledge.ChunkID
	chunkIDs := make([]knowledge.ChunkID, 0, len(results))
	for _, result := range results {
		if result.ChunkID != "" {
			chunkIDs = append(chunkIDs, knowledge.ChunkID(result.ChunkID))
		}
	}

	return chunkIDs, nil
}

// RecencyRanker sorts chunks by timestamp descending (most recent first).
type RecencyRanker struct{}

// NewRecencyRanker creates a new recency ranker.
func NewRecencyRanker() *RecencyRanker {
	return &RecencyRanker{}
}

// Name returns the ranker name.
func (r *RecencyRanker) Name() string {
	return "recency"
}

// Rank sorts chunks by timestamp (most recent first).
func (r *RecencyRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	if store == nil {
		return nil, nil
	}

	// Get all chunks from store
	chunks, err := store.FindAll()
	if err != nil {
		return nil, fmt.Errorf("failed to list chunks: %w", err)
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].AcquiredAt.After(chunks[j].AcquiredAt)
	})

	// Extract chunk IDs
	chunkIDs := make([]knowledge.ChunkID, 0, len(chunks))
	for _, chunk := range chunks {
		chunkIDs = append(chunkIDs, chunk.ID)
	}

	return chunkIDs, nil
}

// TrustRanker filters chunks by trust level.
type TrustRanker struct {
	minTrust float64
}

// NewTrustRanker creates a new trust ranker.
func NewTrustRanker(minTrust float64) *TrustRanker {
	return &TrustRanker{minTrust: minTrust}
}

// Name returns the ranker name.
func (r *TrustRanker) Name() string {
	return "trust"
}

// Rank filters by trust and sorts by suspicion score (lower is better).
func (r *TrustRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	if store == nil {
		return nil, nil
	}

	// Get all chunks and filter by trust
	allChunks, err := store.FindAll()
	if err != nil {
		return nil, fmt.Errorf("failed to list chunks: %w", err)
	}

	// Filter by minimum trust (using suspicion score threshold)
	filtered := make([]knowledge.KnowledgeChunk, 0)
	for _, chunk := range allChunks {
		if chunk.SuspicionScore <= r.minTrust {
			filtered = append(filtered, chunk)
		}
	}

	// Sort by suspicion score ascending (most trusted first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].SuspicionScore < filtered[j].SuspicionScore
	})

	// Extract chunk IDs
	chunkIDs := make([]knowledge.ChunkID, 0, len(filtered))
	for _, chunk := range filtered {
		chunkIDs = append(chunkIDs, chunk.ID)
	}

	return chunkIDs, nil
}

// FreshnessRanker filters and ranks chunks by freshness state.
type FreshnessRanker struct {
	allowStale bool
}

// NewFreshnessRanker creates a new freshness ranker.
func NewFreshnessRanker(allowStale bool) *FreshnessRanker {
	return &FreshnessRanker{allowStale: allowStale}
}

// Name returns the ranker name.
func (r *FreshnessRanker) Name() string {
	return "freshness"
}

// Rank filters by freshness and sorts by freshness state (valid > stale > unverified).
func (r *FreshnessRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	if store == nil {
		return nil, nil
	}

	// Get all chunks
	chunks, err := store.FindAll()
	if err != nil {
		return nil, fmt.Errorf("failed to list chunks: %w", err)
	}

	// Collect chunks by freshness
	validChunks := make([]knowledge.ChunkID, 0)
	staleChunks := make([]knowledge.ChunkID, 0)
	unverifiedChunks := make([]knowledge.ChunkID, 0)

	for _, chunk := range chunks {
		switch chunk.Freshness {
		case knowledge.FreshnessValid:
			validChunks = append(validChunks, chunk.ID)
		case knowledge.FreshnessStale:
			if r.allowStale {
				staleChunks = append(staleChunks, chunk.ID)
			}
		case knowledge.FreshnessUnverified:
			unverifiedChunks = append(unverifiedChunks, chunk.ID)
		}
	}

	// Combine in order of preference: valid > stale > unverified
	result := make([]knowledge.ChunkID, 0)
	result = append(result, validChunks...)
	result = append(result, staleChunks...)
	result = append(result, unverifiedChunks...)

	return result, nil
}

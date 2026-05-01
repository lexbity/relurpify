package retrieval

import (
	"context"
	"fmt"

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

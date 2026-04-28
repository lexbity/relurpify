package retrieval

import (
	"context"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// Retriever performs scatter-gather retrieval using multiple rankers.
type Retriever struct {
	registry *RankerRegistry
	store    *knowledge.ChunkStore
	policy   *contextpolicy.ContextPolicyBundle
}

// NewRetriever creates a new retriever.
func NewRetriever(registry *RankerRegistry, store *knowledge.ChunkStore) *Retriever {
	return &Retriever{
		registry: registry,
		store:    store,
	}
}

// WithPolicy sets the context policy for ranker admission and filtering.
func (r *Retriever) WithPolicy(policy *contextpolicy.ContextPolicyBundle) *Retriever {
	r.policy = policy
	return r
}

// Retrieve performs scatter-gather retrieval.
func (r *Retriever) Retrieve(ctx context.Context, query RetrievalQuery) (*RetrievalResult, error) {
	if r.registry == nil || r.store == nil {
		return &RetrievalResult{
			Query:      query,
			Ranked:     nil,
			TotalFound: 0,
		}, nil
	}

	// Get admitted rankers
	rankers := r.registry.Admitted(r.policy)
	if len(rankers) == 0 {
		return &RetrievalResult{
			Query:      query,
			Ranked:     nil,
			TotalFound: 0,
		}, nil
	}

	// Scatter: execute rankers in parallel
	rankedLists := r.scatter(ctx, query, rankers)

	// Gather: merge results using RRF
	merged := r.gather(rankedLists)

	// Apply limit
	if query.Limit > 0 && len(merged) > query.Limit {
		merged = merged[:query.Limit]
	}

	return &RetrievalResult{
		Query:      query,
		Ranked:     merged,
		TotalFound: len(merged),
	}, nil
}

// scatter executes rankers in parallel and returns their ranked lists.
func (r *Retriever) scatter(ctx context.Context, query RetrievalQuery, rankers []Ranker) [][]knowledge.ChunkID {
	results := make([][]knowledge.ChunkID, len(rankers))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, ranker := range rankers {
		wg.Add(1)
		go func(index int, rnk Ranker) {
			defer wg.Done()

			chunkIDs, err := rnk.Rank(ctx, query, r.store)
			if err != nil {
				return
			}

			mu.Lock()
			results[index] = chunkIDs
			mu.Unlock()
		}(i, ranker)
	}

	wg.Wait()
	return results
}

// gather merges ranked lists using RRF fusion.
func (r *Retriever) gather(rankedLists [][]knowledge.ChunkID) []RankedChunk {
	// Filter out nil/empty lists
	validLists := make([][]knowledge.ChunkID, 0, len(rankedLists))
	for _, list := range rankedLists {
		if len(list) > 0 {
			validLists = append(validLists, list)
		}
	}

	if len(validLists) == 0 {
		return nil
	}

	// Use equal weights for all rankers
	weights := make([]float64, len(validLists))
	for i := range weights {
		weights[i] = 1.0
	}

	return RRF(validLists, weights, 60.0)
}

// RetrieveBatch performs retrieval for multiple queries in parallel.
func (r *Retriever) RetrieveBatch(ctx context.Context, queries []RetrievalQuery) ([]*RetrievalResult, error) {
	results := make([]*RetrievalResult, len(queries))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, query := range queries {
		wg.Add(1)
		go func(index int, q RetrievalQuery) {
			defer wg.Done()

			result, err := r.Retrieve(ctx, q)
			if err != nil {
				mu.Lock()
				results[index] = &RetrievalResult{
					Query:  q,
					Ranked: nil,
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			results[index] = result
			mu.Unlock()
		}(i, query)
	}

	wg.Wait()
	return results, nil
}

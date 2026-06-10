package retrieval

import (
	"context"
	"sync"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

// Retriever performs scatter-gather retrieval using multiple rankers.
type Retriever struct {
	registry *RankerRegistry
	store    *knowledge.ChunkStore
	policy   *contextports.PolicyBundle
}

// NewRetriever creates a new retriever.
func NewRetriever(registry *RankerRegistry, store *knowledge.ChunkStore) *Retriever {
	return &Retriever{
		registry: registry,
		store:    store,
	}
}

// WithPolicy sets the context policy for ranker admission and filtering.
func (r *Retriever) WithPolicy(policy *contextports.PolicyBundle) *Retriever {
	r.policy = policy
	return r
}

// Retrieve performs scatter-gather retrieval.
func (r *Retriever) Retrieve(ctx context.Context, query RetrievalQuery) (*RetrievalResult, error) {
	if r.registry == nil || r.store == nil {
		traversal := r.traversalCandidates(ctx, query)
		if len(traversal) == 0 {
			return &RetrievalResult{
				Query:      query,
				Ranked:     nil,
				TotalFound: 0,
			}, nil
		}
		return &RetrievalResult{
			Query:      query,
			Ranked:     rankedChunksFromIDs(traversal, "traversal"),
			TotalFound: len(traversal),
		}, nil
	}

	traversal := r.traversalCandidates(ctx, query)
	admitted := r.Admitted()
	if len(admitted) == 0 && len(traversal) == 0 {
		return &RetrievalResult{
			Query:      query,
			Ranked:     nil,
			TotalFound: 0,
		}, nil
	}

	rankedLists, weights := make([][]knowledge.ChunkID, 0, len(admitted)+1), make([]float64, 0, len(admitted)+1)

	// Scatter: execute rankers in parallel
	if len(admitted) > 0 {
		scattered, scatteredWeights := r.scatter(ctx, query, admitted)
		rankedLists = append(rankedLists, scattered...)
		weights = append(weights, scatteredWeights...)
	}

	if len(traversal) > 0 {
		rankedLists = append(rankedLists, traversal)
		traversalWeight := 0.5
		if query.Traversal != nil && query.Traversal.PreferLatest {
			traversalWeight = 0.75
		}
		weights = append(weights, traversalWeight)
	}

	// Gather: merge results using RRF
	merged := r.gather(rankedLists, weights)

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

const (
	traversalPageSize    = 512
	defaultMaxCandidates = 500
)

func (r *Retriever) traversalCandidates(ctx context.Context, query RetrievalQuery) []knowledge.ChunkID {
	spec := query.Traversal
	if r == nil || r.store == nil || r.store.Graph == nil || spec == nil {
		return nil
	}
	anchorIDs := make([]string, 0, len(spec.AnchorIDs)+len(query.Anchors))
	for _, id := range spec.AnchorIDs {
		if id != "" {
			anchorIDs = append(anchorIDs, id)
		}
	}
	if len(anchorIDs) == 0 {
		for _, anchor := range query.Anchors {
			if anchor.ChunkID != "" {
				anchorIDs = append(anchorIDs, anchor.ChunkID)
			}
		}
	}
	if len(anchorIDs) == 0 {
		return nil
	}

	direction := graphdb.DirectionBoth
	switch spec.Direction {
	case TraversalDirectionOut:
		direction = graphdb.DirectionOut
	case TraversalDirectionIn:
		direction = graphdb.DirectionIn
	}
	edgeKinds := make([]graphdb.EdgeKind, 0, len(spec.EdgeKinds))
	for _, kind := range spec.EdgeKinds {
		if kind != "" {
			edgeKinds = append(edgeKinds, graphdb.EdgeKind(kind))
		}
	}

	// Resolve candidate budget by precedence: per-query > policy > floor.
	budget := spec.MaxCandidates
	if budget <= 0 && r.policy != nil {
		budget = r.policy.MaxTraversalCandidates
	}
	if budget <= 0 {
		budget = defaultMaxCandidates
	}

	keep := newBoundedTopK(budget, spec.PreferLatest)
	var token graphdb.PageToken
	for {
		page, err := r.store.Graph.SubgraphPage(ctx, graphdb.GraphPageQuery{
			GraphQuery: graphdb.GraphQuery{
				RootIDs:   anchorIDs,
				EdgeKinds: edgeKinds,
				Direction: direction,
				MaxDepth:  spec.MaxDepth,
				Limit:     budget,
			},
			PageSize: traversalPageSize,
			After:    token,
		})
		if err != nil {
			return nil
		}
		for _, el := range page.Items {
			if el.Node.Kind == knowledge.ChunkNodeKind && el.Node.ID != "" {
				keep.offer(knowledge.ChunkID(el.Node.ID), el.Node.UpdatedAt)
			}
		}
		if page.Next == "" {
			break
		}
		if !spec.PreferLatest && keep.full() {
			break
		}
		token = page.Next
	}
	return keep.ids()
}

func rankedChunksFromIDs(ids []knowledge.ChunkID, source string) []RankedChunk {
	if len(ids) == 0 {
		return nil
	}
	out := make([]RankedChunk, 0, len(ids))
	for i, id := range ids {
		out = append(out, RankedChunk{
			ChunkID: id,
			Rank:    i + 1,
			Score:   float64(len(ids)-i) / float64(len(ids)),
			Source:  source,
		})
	}
	return out
}

// Admitted returns the rankers admitted by the current policy.
func (r *Retriever) Admitted() []AdmittedRanker {
	if r == nil || r.registry == nil {
		return nil
	}
	return r.registry.Admitted(r.policy)
}

// scatter executes rankers in parallel and returns their ranked lists.
func (r *Retriever) scatter(ctx context.Context, query RetrievalQuery, rankers []AdmittedRanker) ([][]knowledge.ChunkID, []float64) {
	results := make([][]knowledge.ChunkID, len(rankers))
	weights := make([]float64, len(rankers))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, admitted := range rankers {
		wg.Add(1)
		weights[i] = admitted.Weight
		go func(index int, rnk Ranker) {
			defer wg.Done()

			chunkIDs, err := rnk.Rank(ctx, query, r.store)
			if err != nil {
				return
			}

			mu.Lock()
			results[index] = chunkIDs
			mu.Unlock()
		}(i, admitted.Ranker)
	}

	wg.Wait()
	return results, weights
}

// gather merges ranked lists using RRF fusion.
func (r *Retriever) gather(rankedLists [][]knowledge.ChunkID, weights []float64) []RankedChunk {
	// Filter out nil/empty lists
	validLists := make([][]knowledge.ChunkID, 0, len(rankedLists))
	validWeights := make([]float64, 0, len(rankedLists))
	for _, list := range rankedLists {
		if len(list) > 0 {
			validLists = append(validLists, list)
		}
	}
	for i, list := range rankedLists {
		if len(list) > 0 {
			if i < len(weights) {
				validWeights = append(validWeights, weights[i])
			} else {
				validWeights = append(validWeights, 1.0)
			}
		}
	}

	if len(validLists) == 0 {
		return nil
	}
	return RRF(validLists, validWeights, 60.0)
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

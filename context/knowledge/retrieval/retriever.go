package retrieval

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// Retriever performs scatter-gather retrieval using multiple rankers over
// one corpus snapshot per generation: a single store decode shared by every
// ranker, refreshed by chunk lifecycle events, a 30 s TTL, and explicit
// Invalidate calls. Snapshot failures degrade to serve-stale (≤2×TTL) with
// telemetry before becoming a hard error.
type Retriever struct {
	registry  *RankerRegistry
	store     *knowledge.ChunkStore
	policy    *contextports.PolicyBundle
	snapshots *snapshotState
	telemetry fwtelemetry.Telemetry
	nowFn     func() time.Time

	invalidateMu sync.Mutex
	subCancel    func()
}

// NewRetriever creates a new retriever.
func NewRetriever(registry *RankerRegistry, store *knowledge.ChunkStore) *Retriever {
	nowFn := time.Now
	r := &Retriever{
		registry:  registry,
		store:     store,
		nowFn:     nowFn,
		snapshots: newSnapshotState(nowFn),
	}
	return r
}

// SetTelemetry wires retrieval observability (ranker failures, snapshot
// lifecycle). Counter emissions are the contract even without a sink.
func (r *Retriever) SetTelemetry(tel fwtelemetry.Telemetry) {
	r.telemetry = tel
}

// SetClock installs the retriever's time source (deterministic tests for the
// snapshot TTL). Must be called before the first Retrieve.
func (r *Retriever) SetClock(now func() time.Time) {
	r.nowFn = now
	r.snapshots.now = now
}

// SetEventBus subscribes the retriever to chunk lifecycle events
// (ingested supersedes cached snapshots; staled removes chunks from
// consideration). The returned cancel function unsubscribes and is owned by
// the composition root.
func (r *Retriever) SetEventBus(bus *knowledge.EventBus) func() {
	if bus == nil {
		return func() {}
	}
	events, cancel := bus.Subscribe(64)
	r.subCancel = cancel
	go r.consumeEvents(events)
	return cancel
}

// consumeEvents invalidates the snapshot generation on chunk events until
// the subscription closes.
func (r *Retriever) consumeEvents(events <-chan knowledge.Event) {
	for event := range events {
		switch event.Kind {
		case knowledge.EventChunkIngested:
			if payload, ok := event.Payload.(knowledge.ChunkIngestedPayload); ok && payload.ChunkID != "" {
				r.Invalidate()
			}
		case knowledge.EventChunkStaled:
			if payload, ok := event.Payload.(knowledge.ChunkStaledPayload); ok && len(payload.ChunkIDs) > 0 {
				r.Invalidate()
			}
		}
	}
}

// Unsubscribe detaches the event subscription (idempotent).
func (r *Retriever) Unsubscribe() {
	r.invalidateMu.Lock()
	cancel := r.subCancel
	r.subCancel = nil
	r.invalidateMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Invalidate drops the current snapshot generation. Called by event
// consumption and available to the ingester for explicit post-commit
// invalidation.
func (r *Retriever) Invalidate() {
	r.snapshots.invalidate()
	r.emitCounter("retrieval_snapshot_invalidated", nil)
}

// emitCounter surfaces a retrieval lifecycle transition as telemetry.
func (r *Retriever) emitCounter(kind string, metadata map[string]any) {
	if r.telemetry == nil {
		return
	}
	ev := fwtelemetry.Event{
		Type:      fwtelemetry.EventType(kind),
		Message:   kind,
		Timestamp: r.nowFn(),
		Metadata:  metadata,
	}
	r.telemetry.Emit(ev)
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

	// One snapshot per retrieval generation, shared by all rankers. Fresh
	// snapshots are silent; a rebuild and a serve-stale fallback are each
	// observable, never silent.
	snap, outcome, err := r.snapshots.get(r.store)
	if err != nil {
		return nil, fmt.Errorf("build corpus snapshot: %w", err)
	}
	switch outcome {
	case outcomeBuilt:
		r.emitCounter("retrieval_snapshot_built", map[string]any{
			"generation": snap.Generation,
			"chunks":     len(snap.Chunks),
		})
	case outcomeServedStale:
		r.emitCounter("retrieval_snapshot_served_stale", map[string]any{
			"generation": snap.Generation,
		})
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
		scattered, scatteredWeights := r.scatter(ctx, query, admitted, snap)
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

// scatter executes rankers in parallel over the shared snapshot. A ranker
// failure degrades to the remaining rankers with a retrieval_ranker_failed
// event — a failed ranker never silently empties its list.
func (r *Retriever) scatter(ctx context.Context, query RetrievalQuery, rankers []AdmittedRanker, snap *CorpusSnapshot) ([][]knowledge.ChunkID, []float64) {
	results := make([][]knowledge.ChunkID, len(rankers))
	weights := make([]float64, len(rankers))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, admitted := range rankers {
		wg.Add(1)
		weights[i] = admitted.Weight
		go func(index int, rnk Ranker) {
			defer wg.Done()

			chunkIDs, err := rnk.Rank(ctx, query, snap)
			if err != nil {
				r.emitCounter("retrieval_ranker_failed", map[string]any{
					"ranker": rnk.Name(),
					"error":  err.Error(),
				})
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

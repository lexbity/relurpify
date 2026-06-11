package retrieval

import (
	"context"
	"math"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

// mockRanker is a test ranker that returns predefined results.
type mockRanker struct {
	name    string
	results []knowledge.ChunkID
}

func (m *mockRanker) Name() string {
	return m.name
}

func (m *mockRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	return m.results, nil
}

func TestRRF(t *testing.T) {
	tests := []struct {
		name        string
		lists       [][]knowledge.ChunkID
		weights     []float64
		expectedLen int
	}{
		{
			name: "two lists with overlap",
			lists: [][]knowledge.ChunkID{
				{"a", "b", "c"},
				{"b", "c", "d"},
			},
			weights:     []float64{1.0, 1.0},
			expectedLen: 4,
		},
		{
			name: "unequal weights",
			lists: [][]knowledge.ChunkID{
				{"a", "b"},
				{"b", "a"},
			},
			weights:     []float64{2.0, 1.0},
			expectedLen: 2,
		},
		{
			name: "empty list",
			lists: [][]knowledge.ChunkID{
				{},
				{"a", "b"},
			},
			weights:     []float64{1.0, 1.0},
			expectedLen: 2,
		},
		{
			name:        "empty input",
			lists:       [][]knowledge.ChunkID{},
			weights:     []float64{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RRF(tt.lists, tt.weights, 60.0)
			if len(result) != tt.expectedLen {
				t.Errorf("expected %d results, got %d", tt.expectedLen, len(result))
			}

			// Verify ranks are sequential starting from 1
			for i, r := range result {
				if r.Rank != i+1 {
					t.Errorf("expected rank %d, got %d", i+1, r.Rank)
				}
				if r.Score <= 0 {
					t.Errorf("expected positive score, got %f", r.Score)
				}
			}
		})
	}
}

func TestSimpleRRF(t *testing.T) {
	lists := [][]knowledge.ChunkID{
		{"a", "b", "c"},
		{"b", "c", "d"},
	}

	result := SimpleRRF(lists)
	if len(result) != 4 {
		t.Errorf("expected 4 results, got %d", len(result))
	}

	// "b" and "c" appear in both lists, should rank higher
	if result[0].ChunkID != "b" && result[0].ChunkID != "c" {
		t.Errorf("expected b or c at position 0, got %s", result[0].ChunkID)
	}
}

func TestWeightedRRF(t *testing.T) {
	lists := [][]knowledge.ChunkID{
		{"a", "b", "c"},
		{"c", "b", "a"},
	}
	weights := []float64{2.0, 1.0}

	result := WeightedRRF(lists, weights)
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}

	// With higher weight on first list, "a" should be first
	if result[0].ChunkID != "a" {
		t.Errorf("expected a at position 0, got %s", result[0].ChunkID)
	}
}

func TestRankerRegistry(t *testing.T) {
	registry := NewRankerRegistry()

	// Test empty registry
	admitted := registry.Admitted(nil)
	if len(admitted) != 0 {
		t.Errorf("expected 0 admitted rankers, got %d", len(admitted))
	}

	// Register rankers
	r1 := &mockRanker{name: "r1", results: []knowledge.ChunkID{"a", "b"}}
	r2 := &mockRanker{name: "r2", results: []knowledge.ChunkID{"b", "c"}}

	registry.Register(r1)
	registry.Register(r2)

	admitted = registry.Admitted(nil)
	if len(admitted) != 2 {
		t.Errorf("expected 2 admitted rankers, got %d", len(admitted))
	}
	if admitted[0].Ranker.Name() != "r1" || admitted[1].Ranker.Name() != "r2" {
		t.Fatalf("expected registration order, got %s then %s", admitted[0].Ranker.Name(), admitted[1].Ranker.Name())
	}
}

func TestRankerRegistry_PolicyAdmission(t *testing.T) {
	registry := NewRankerRegistry()
	registry.Register(&mockRanker{name: "keyword", results: []knowledge.ChunkID{"a"}})
	registry.Register(&mockRanker{name: "recency", results: []knowledge.ChunkID{"b"}})
	registry.Register(&mockRanker{name: "ast_proximity", results: []knowledge.ChunkID{"c"}})
	registry.Register(&mockRanker{name: "trust", results: []knowledge.ChunkID{"d"}})

	policy := &contextports.PolicyBundle{}

	admitted := registry.Admitted(policy)
	if len(admitted) != 4 {
		t.Fatalf("expected 4 admitted rankers, got %d", len(admitted))
	}
	if admitted[0].Ranker.Name() != "keyword" || admitted[1].Ranker.Name() != "recency" {
		t.Fatalf("unexpected admitted order: %s, %s", admitted[0].Ranker.Name(), admitted[1].Ranker.Name())
	}
}

func TestRetriever(t *testing.T) {
	// Create registry with mock rankers
	registry := NewRankerRegistry()
	r1 := &mockRanker{name: "r1", results: []knowledge.ChunkID{"chunk1", "chunk2", "chunk3"}}
	registry.Register(r1)

	// Create retriever with nil store (for now)
	retriever := NewRetriever(registry, nil)

	// Test retrieval
	query := RetrievalQuery{
		Text:  "test query",
		Scope: "test",
		Limit: 10,
	}

	ctx := context.Background()
	result, err := retriever.Retrieve(ctx, query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With nil store, should return empty results
	if result.TotalFound != 0 {
		t.Errorf("expected 0 results with nil store, got %d", result.TotalFound)
	}
}

func TestRetrieverWithNilStore(t *testing.T) {
	registry := NewRankerRegistry()
	retriever := NewRetriever(registry, nil)

	query := RetrievalQuery{Text: "test"}
	result, err := retriever.Retrieve(context.Background(), query)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalFound != 0 {
		t.Errorf("expected 0 results with nil store, got %d", result.TotalFound)
	}
}

func TestRetrieverWithNilRegistry(t *testing.T) {
	// Create a dummy store - just nil for now
	retriever := NewRetriever(nil, nil)

	query := RetrievalQuery{Text: "test"}
	result, err := retriever.Retrieve(context.Background(), query)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalFound != 0 {
		t.Errorf("expected 0 results with nil registry, got %d", result.TotalFound)
	}
}

func TestRetrieverBatch(t *testing.T) {
	registry := NewRankerRegistry()
	registry.Register(&mockRanker{name: "r1", results: []knowledge.ChunkID{"chunk1"}})

	retriever := NewRetriever(registry, nil)

	queries := []RetrievalQuery{
		{Text: "query1"},
		{Text: "query2"},
	}

	results, err := retriever.RetrieveBatch(context.Background(), queries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestRetriever_ParallelScatter(t *testing.T) {
	store := newRetrievalTestStore(t)
	registry := NewRankerRegistry()
	registry.Register(&sleepRanker{name: "r1", delay: 10 * time.Millisecond, results: []knowledge.ChunkID{"a"}})
	registry.Register(&sleepRanker{name: "r2", delay: 10 * time.Millisecond, results: []knowledge.ChunkID{"b"}})
	registry.Register(&sleepRanker{name: "r3", delay: 10 * time.Millisecond, results: []knowledge.ChunkID{"c"}})
	registry.Register(&sleepRanker{name: "r4", delay: 10 * time.Millisecond, results: []knowledge.ChunkID{"d"}})

	retriever := NewRetriever(registry, store)
	start := time.Now()
	result, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "scatter"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 25*time.Millisecond {
		t.Fatalf("expected parallel scatter to finish quickly, took %s", elapsed)
	}
	if len(result.Ranked) != 4 {
		t.Fatalf("expected 4 fused results, got %d", len(result.Ranked))
	}
}

func TestRetrieverTraversalCandidates(t *testing.T) {
	store := newRetrievalTestStore(t)
	for _, chunk := range []knowledge.KnowledgeChunk{
		{ID: knowledge.ChunkID("chunk:root"), WorkspaceID: "ws", Provenance: knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: knowledge.ChunkBody{Raw: "root"}},
		{ID: knowledge.ChunkID("chunk:child1"), WorkspaceID: "ws", Provenance: knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: knowledge.ChunkBody{Raw: "child1"}},
		{ID: knowledge.ChunkID("chunk:child2"), WorkspaceID: "ws", Provenance: knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: knowledge.ChunkBody{Raw: "child2"}},
	} {
		_, err := store.Save(context.Background(), chunk)
		if err != nil {
			t.Fatalf("save chunk: %v", err)
		}
	}
	if _, err := store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: knowledge.ChunkID("chunk:root"), ToChunk: knowledge.ChunkID("chunk:child1"), Kind: knowledge.EdgeKindRequiresContext, Weight: 1}); err != nil {
		t.Fatalf("save edge 1: %v", err)
	}
	if _, err := store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: knowledge.ChunkID("chunk:child1"), ToChunk: knowledge.ChunkID("chunk:child2"), Kind: knowledge.EdgeKindRequiresContext, Weight: 1}); err != nil {
		t.Fatalf("save edge 2: %v", err)
	}

	retriever := NewRetriever(nil, store)
	result, err := retriever.Retrieve(context.Background(), RetrievalQuery{
		Traversal: &TraversalSpec{
			AnchorIDs: []string{"chunk:root"},
			EdgeKinds: []string{string(knowledge.EdgeKindRequiresContext)},
			Direction: TraversalDirectionOut,
			MaxDepth:  2,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Ranked) != 3 {
		t.Fatalf("expected 3 traversal results, got %d", len(result.Ranked))
	}
	if result.Ranked[0].ChunkID != knowledge.ChunkID("chunk:root") {
		t.Fatalf("expected root first, got %s", result.Ranked[0].ChunkID)
	}
}

func TestRetrieverTraversalDoesNotOverrideTextRanker(t *testing.T) {
	store := newRetrievalTestStore(t)
	for _, chunk := range []knowledge.KnowledgeChunk{
		{ID: knowledge.ChunkID("chunk:root"), WorkspaceID: "ws", Provenance: knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: knowledge.ChunkBody{Raw: "root text"}},
		{ID: knowledge.ChunkID("chunk:text"), WorkspaceID: "ws", Provenance: knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: knowledge.ChunkBody{Raw: "detailed text match"}},
	} {
		if _, err := store.Save(context.Background(), chunk); err != nil {
			t.Fatalf("save chunk: %v", err)
		}
	}
	if _, err := store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: knowledge.ChunkID("chunk:root"), ToChunk: knowledge.ChunkID("chunk:text"), Kind: knowledge.EdgeKindRequiresContext, Weight: 1}); err != nil {
		t.Fatalf("save edge: %v", err)
	}

	registry := NewRankerRegistry()
	registry.Register(&mockRanker{name: "keyword", results: []knowledge.ChunkID{"chunk:text"}})

	retriever := NewRetriever(registry, store)
	result, err := retriever.Retrieve(context.Background(), RetrievalQuery{
		Text: "detailed text match",
		Traversal: &TraversalSpec{
			AnchorIDs: []string{"chunk:root"},
			EdgeKinds: []string{string(knowledge.EdgeKindRequiresContext)},
			Direction: TraversalDirectionOut,
			MaxDepth:  1,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Ranked) < 2 {
		t.Fatalf("expected fused results, got %#v", result.Ranked)
	}
	if result.Ranked[0].ChunkID != knowledge.ChunkID("chunk:text") {
		t.Fatalf("expected text ranker to win over traversal, got %s", result.Ranked[0].ChunkID)
	}
}

func TestRRF_WeightedFusion(t *testing.T) {
	lists := [][]knowledge.ChunkID{
		{"a", "b"},
		{"b", "a"},
	}
	weights := []float64{10, 1}

	result := RRF(lists, weights, 60.0)
	if len(result) != 2 {
		t.Fatalf("expected 2 fused results, got %d", len(result))
	}
	if result[0].ChunkID != "a" {
		t.Fatalf("expected higher-weighted list to dominate, got %s", result[0].ChunkID)
	}
}

func TestComputeRRFScore(t *testing.T) {
	ranks := []int{1, 2, 0} // 0 means not present
	weights := []float64{1.0, 1.0, 1.0}

	score := ComputeRRFScore(ranks, weights, 60.0)

	// Should be 1/61 + 1/62 + 0 = ~0.0328
	expected := 1.0/61.0 + 1.0/62.0
	// Use tolerance for floating point comparison
	const epsilon = 1e-9
	if math.Abs(score-expected) > epsilon {
		t.Errorf("expected %f, got %f", expected, score)
	}
}

type sleepRanker struct {
	name    string
	delay   time.Duration
	results []knowledge.ChunkID
}

func (s *sleepRanker) Name() string { return s.name }

func (s *sleepRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	_ = ctx
	_ = query
	_ = store
	time.Sleep(s.delay)
	return append([]knowledge.ChunkID(nil), s.results...), nil
}

func TestTraversalCandidates_BoundUnderLRU(t *testing.T) {
	// Under LRU with tight capacity and a wide subgraph, the bounded
	// traversal must page correctly and stay within budget.
	opts := graphdb.DefaultOptions(t.TempDir())
	opts.LRUCapacity = 3
	engine, err := graphdb.Open(context.Background(), opts)
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	defer engine.Close(context.Background())
	store := &knowledge.ChunkStore{Graph: engine}

	// Ingest enough chunks that paging is required.
	for i := 0; i < 20; i++ {
		_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
			ID: knowledge.ChunkID("lru-chunk-" + string(rune('a'+i))),
		})
		if err != nil {
			t.Fatalf("save chunk %d: %v", i, err)
		}
		if i == 0 {
			continue
		}
		if _, err := store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: "lru-chunk-a", ToChunk: knowledge.ChunkID("lru-chunk-" + string(rune('a'+i))), Kind: knowledge.EdgeKindRequiresContext, Weight: 1}); err != nil {
			t.Fatalf("save edge %d: %v", i, err)
		}
	}

	retriever := NewRetriever(nil, store).WithPolicy(&contextports.PolicyBundle{MaxTraversalCandidates: 5})
	ids := retriever.traversalCandidates(context.Background(), RetrievalQuery{
		Traversal: &TraversalSpec{
			AnchorIDs: []string{"lru-chunk-a"},
			EdgeKinds: []string{string(knowledge.EdgeKindRequiresContext)},
			Direction: TraversalDirectionOut,
			MaxDepth:  1,
		},
	})
	if len(ids) > 5 {
		t.Errorf("expected ≤ 5 candidates under policy budget, got %d", len(ids))
	}
	if len(ids) == 0 {
		t.Error("expected at least some candidates (paging should have worked)")
	}
}

func TestTraversalCandidates_Precedence(t *testing.T) {
	// Per-query MaxCandidates overrides policy MaxTraversalCandidates.
	store := newRetrievalTestStore(t)
	_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{ID: "prec-a"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{ID: knowledge.ChunkID("prec-child-" + string(rune('0'+i)))})
		if err != nil {
			t.Fatal(err)
		}
		store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: "prec-a", ToChunk: knowledge.ChunkID("prec-child-" + string(rune('0'+i))), Kind: knowledge.EdgeKindRequiresContext, Weight: 1})
	}

	retriever := NewRetriever(nil, store).WithPolicy(&contextports.PolicyBundle{MaxTraversalCandidates: 100})

	// Query with explicit MaxCandidates=3 should return ≤ 3, not 100.
	ids := retriever.traversalCandidates(context.Background(), RetrievalQuery{
		Traversal: &TraversalSpec{
			AnchorIDs:     []string{"prec-a"},
			EdgeKinds:     []string{string(knowledge.EdgeKindRequiresContext)},
			Direction:     TraversalDirectionOut,
			MaxDepth:      1,
			MaxCandidates: 3,
		},
	})
	if len(ids) > 3 {
		t.Errorf("expected ≤ 3 with MaxCandidates=3, got %d", len(ids))
	}
	if len(ids) == 0 {
		t.Error("expected at least 1 candidate")
	}

	// Query without explicit MaxCandidates falls back to policy=100.
	ids2 := retriever.traversalCandidates(context.Background(), RetrievalQuery{
		Traversal: &TraversalSpec{
			AnchorIDs: []string{"prec-a"},
			EdgeKinds: []string{string(knowledge.EdgeKindRequiresContext)},
			Direction: TraversalDirectionOut,
			MaxDepth:  1,
		},
	})
	if len(ids2) > 100 {
		t.Errorf("expected ≤ 100 with policy, got %d", len(ids2))
	}
}

func TestTraversalCandidates_Parity(t *testing.T) {
	// For a small in-budget subgraph, returned IDs match pre-change
	// behavior (same set in discovery order).
	store := newRetrievalTestStore(t)
	chunks := []string{"a", "b", "c"}
	for _, id := range chunks {
		store.Save(context.Background(), knowledge.KnowledgeChunk{ID: knowledge.ChunkID(id)})
	}
	store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: "a", ToChunk: "b", Kind: knowledge.EdgeKindRequiresContext, Weight: 1})
	store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: "b", ToChunk: "c", Kind: knowledge.EdgeKindRequiresContext, Weight: 1})

	retriever := NewRetriever(nil, store)
	ids := retriever.traversalCandidates(context.Background(), RetrievalQuery{
		Traversal: &TraversalSpec{
			AnchorIDs: []string{"a"},
			EdgeKinds: []string{string(knowledge.EdgeKindRequiresContext)},
			Direction: TraversalDirectionOut,
			MaxDepth:  2,
		},
	})

	// Expect: a, b, c (discovery order from BFS).
	if len(ids) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %v", len(ids), ids)
	}
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Errorf("expected [a b c], got %v", ids)
	}
}

func TestTraversalCandidates_PreferLatestRegime(t *testing.T) {
	// With PreferLatest, the K kept must be the K most recent, not the
	// K shallowest.  Insert chunks with varied freshness across depths.
	store := newRetrievalTestStore(t)
	store.Save(context.Background(), knowledge.KnowledgeChunk{ID: "latest-old", Body: knowledge.ChunkBody{Raw: "old"}})
	store.Save(context.Background(), knowledge.KnowledgeChunk{ID: "latest-mid", Body: knowledge.ChunkBody{Raw: "mid"}})
	store.Save(context.Background(), knowledge.KnowledgeChunk{ID: "latest-new", Body: knowledge.ChunkBody{Raw: "new"}})
	// Link them so BFS finds all three.
	store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: "latest-old", ToChunk: "latest-mid", Kind: knowledge.EdgeKindRequiresContext, Weight: 1})
	store.SaveEdge(context.Background(), knowledge.ChunkEdge{FromChunk: "latest-mid", ToChunk: "latest-new", Kind: knowledge.EdgeKindRequiresContext, Weight: 1})

	// Manually set UpdatedAt timestamps by re-saving with different times.
	// PreferLatest sorts by UpdatedAt descending.
	store.Save(context.Background(), knowledge.KnowledgeChunk{ID: "latest-old", Body: knowledge.ChunkBody{Raw: "old"}, Provenance: knowledge.ChunkProvenance{Timestamp: time.Now().Add(-2 * time.Hour)}})
	store.Save(context.Background(), knowledge.KnowledgeChunk{ID: "latest-mid", Body: knowledge.ChunkBody{Raw: "mid"}, Provenance: knowledge.ChunkProvenance{Timestamp: time.Now().Add(-1 * time.Hour)}})
	store.Save(context.Background(), knowledge.KnowledgeChunk{ID: "latest-new", Body: knowledge.ChunkBody{Raw: "new"}, Provenance: knowledge.ChunkProvenance{Timestamp: time.Now()}})

	retriever := NewRetriever(nil, store)
	budget := 2
	_ = budget
	// With PreferLatest and budget=2, we should get [latest-new, latest-mid].
	ids := retriever.traversalCandidates(context.Background(), RetrievalQuery{
		Traversal: &TraversalSpec{
			AnchorIDs:     []string{"latest-old"},
			EdgeKinds:     []string{string(knowledge.EdgeKindRequiresContext)},
			Direction:     TraversalDirectionOut,
			MaxDepth:      2,
			PreferLatest:  true,
			MaxCandidates: 2,
		},
	})
	if len(ids) != 2 {
		t.Fatalf("expected 2 candidates with budget=2, got %d: %v", len(ids), ids)
	}
	// The 2 most recent should be latest-new and latest-mid.
	if ids[0] != "latest-new" && ids[0] != "latest-mid" {
		t.Errorf("expected most recent first, got %s", ids[0])
	}
}

func newRetrievalTestStore(t *testing.T) *knowledge.ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	return &knowledge.ChunkStore{Graph: engine}
}

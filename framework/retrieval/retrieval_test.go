package retrieval

import (
	"context"
	"math"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextpolicy"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
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

	policy := &contextpolicy.ContextPolicyBundle{
		Rankers: []contextpolicy.RankerRef{
			{ID: "keyword", Priority: 10},
			{ID: "recency", Priority: 5},
		},
	}

	admitted := registry.Admitted(policy)
	if len(admitted) != 2 {
		t.Fatalf("expected 2 admitted rankers, got %d", len(admitted))
	}
	if admitted[0].Ranker.Name() != "keyword" || admitted[1].Ranker.Name() != "recency" {
		t.Fatalf("unexpected admitted order: %s, %s", admitted[0].Ranker.Name(), admitted[1].Ranker.Name())
	}
	if admitted[0].Weight <= admitted[1].Weight {
		t.Fatalf("expected keyword to have higher weight than recency, got %.2f <= %.2f", admitted[0].Weight, admitted[1].Weight)
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

func newRetrievalTestStore(t *testing.T) *knowledge.ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return &knowledge.ChunkStore{Graph: engine}
}

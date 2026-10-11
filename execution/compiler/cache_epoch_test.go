package compiler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
)

// newEpochTestCompiler builds a deterministic compiler over a real store with
// a static ranker seeding the given chunk IDs.
func newEpochTestCompiler(t *testing.T, ids []knowledge.ChunkID) (*Compiler, *knowledge.ChunkStore, *staticRanker) {
	t.Helper()
	store := newCompilerTestStore(t)
	ranker := &staticRanker{name: "static-epoch", ids: ids}
	registry := retrieval.NewRankerRegistry()
	registry.Register(ranker)
	retriever := retrieval.NewRetriever(registry, store)
	c := NewCompiler(retriever, nil, store)
	now := time.Unix(1000, 0)
	c.SetTimeFunc(func() time.Time { return now })
	return c, store, ranker
}

// TestCacheHitPreservesStreamedRefs: a cache hit returns the stored result
// wholesale (ruling R-1) — refs, substitutions, stale-skips, shortfall, and
// chunk bodies all survive, so the hit is indistinguishable from the miss.
func TestCacheHitPreservesStreamedRefs(t *testing.T) {
	ids := []knowledge.ChunkID{"chunk:e1", "chunk:e2", "chunk:e3"}
	contents := map[string]string{"chunk:e1": "alpha body", "chunk:e2": "beta body", "chunk:e3": "gamma body"}
	c, store, _ := newEpochTestCompiler(t, ids)
	now := time.Unix(1000, 0)
	for _, id := range ids {
		saveTrustedChunk(t, store, string(id), contents[string(id)], now)
	}

	req := CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "epoch probe"},
		MaxTokens: 4096,
		EpochID:   7,
	}
	miss, missRecord, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.False(t, missRecord.CacheHit)
	require.Len(t, miss.Chunks, 3)

	hit, hitRecord, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.True(t, hitRecord.CacheHit)

	require.Equal(t, miss.StreamedRefs, hit.StreamedRefs, "streamed refs must survive a hit")
	require.Equal(t, miss.Substitutions, hit.Substitutions, "substitutions must survive a hit")
	require.Equal(t, miss.SkippedStaleChunks, hit.SkippedStaleChunks, "skipped-stale gaps must survive a hit")
	require.Equal(t, miss.ShortfallTokens, hit.ShortfallTokens, "shortfall must survive a hit")
	require.Equal(t, miss.TotalTokens, hit.TotalTokens)
	require.Equal(t, miss, hit, "the hit result must deep-equal the miss result")
	for i, chunk := range hit.Chunks {
		require.Equal(t, contents[string(chunk.ID)], chunk.Body.Raw, "body %d must survive the hit", i)
	}
}

// TestCacheKeyIncludesEpoch: the same request at a different epoch misses and
// reflects the new knowledge state (ruling R-1: correctness is structural).
func TestCacheKeyIncludesEpoch(t *testing.T) {
	c, store, ranker := newEpochTestCompiler(t, []knowledge.ChunkID{"chunk:old"})
	now := time.Unix(1000, 0)
	saveTrustedChunk(t, store, "chunk:old", "before state", now)

	req := CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "epoch key"},
		MaxTokens: 4096,
		EpochID:   3,
	}
	_, firstRecord, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.False(t, firstRecord.CacheHit)

	// Same request, same epoch: hit.
	_, sameRecord, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.True(t, sameRecord.CacheHit)

	// State advances (a barrier grounded a new chunk), the epoch advances:
	// the next compile must miss and see the new chunk.
	saveTrustedChunk(t, store, "chunk:new", "after state", now)
	ranker.ids = []knowledge.ChunkID{"chunk:old", "chunk:new"}
	req.EpochID = 4

	second, secondRecord, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.False(t, secondRecord.CacheHit, "a new epoch must always miss")

	var found bool
	for _, chunk := range second.Chunks {
		if chunk.ID == "chunk:new" {
			found = true
		}
	}
	require.True(t, found, "the epoch-4 compile must reflect the new state")
}

// TestCacheHitCopyIsolatesFieldReassignment: the hit path returns a struct
// copy of the stored result, so replacing the returned copy's slices never
// touches the stored record. Element mutation of the shared slices is
// forbidden by the documented read-only invariant (consumers — the trigger
// adapter, renderers — read only; enforced by the adapter equality check in
// knowledge_wiring_test.go, where any consumer mutation of the shared
// backing arrays would corrupt the hit result itself).
func TestCacheHitCopyIsolatesFieldReassignment(t *testing.T) {
	ids := []knowledge.ChunkID{"chunk:alias"}
	c, store, _ := newEpochTestCompiler(t, ids)
	now := time.Unix(1000, 0)
	saveTrustedChunk(t, store, "chunk:alias", "alias body", now)

	req := CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "alias probe"},
		MaxTokens: 4096,
		EpochID:   1,
	}
	result, _, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, result.Chunks)
	require.NotEmpty(t, result.StreamedRefs)

	// Replace the returned copy's slices: the struct copy absorbs it.
	result.Chunks = nil
	result.StreamedRefs = nil

	key := c.buildCacheKey(req)
	stored := c.cache.get(key)
	require.NotNil(t, stored)
	require.Len(t, stored.record.Result.Chunks, 1)
	require.Equal(t, "alias body", stored.record.Result.Chunks[0].Body.Raw)
	require.NotEmpty(t, stored.record.Result.StreamedRefs)

	// And a subsequent hit still serves the pristine stored slices.
	again, againRecord, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.True(t, againRecord.CacheHit)
	require.Equal(t, "alias body", again.Chunks[0].Body.Raw)
	require.NotEmpty(t, again.StreamedRefs)
}

// TestCacheHitEquivalenceProperty: over 100 seeded (graph, request, epoch)
// tuples, a hit's output deep-equals the stored miss output (NFR-1: a cache
// hit renders identically to the original compile).
func TestCacheHitEquivalenceProperty(t *testing.T) {
	store := newCompilerTestStore(t)
	now := time.Unix(1000, 0)
	ranker := &staticRanker{name: "static-prop", ids: nil}
	registry := retrieval.NewRankerRegistry()
	registry.Register(ranker)
	retriever := retrieval.NewRetriever(registry, store)
	c := NewCompiler(retriever, nil, store)
	c.SetTimeFunc(func() time.Time { return now })

	for i := 0; i < 100; i++ {
		id := knowledge.ChunkID(fmt.Sprintf("chunk:prop-%d", i))
		content := fmt.Sprintf("property body %d", i)
		saveTrustedChunk(t, store, string(id), content, now)
		ranker.ids = []knowledge.ChunkID{id}

		req := CompilationRequest{
			Query:     retrieval.RetrievalQuery{Text: fmt.Sprintf("query %d", i)},
			MaxTokens: 4096,
			EpochID:   uint64(i % 17),
		}
		miss, missRecord, err := c.Compile(context.Background(), req)
		require.NoError(t, err)
		require.False(t, missRecord.CacheHit, "iteration %d", i)

		hit, hitRecord, err := c.Compile(context.Background(), req)
		require.NoError(t, err)
		require.True(t, hitRecord.CacheHit, "iteration %d", i)
		require.Equal(t, miss, hit, "iteration %d: hit must deep-equal the miss", i)
		require.Equal(t, content, hit.Chunks[0].Body.Raw, "iteration %d", i)
	}
}

// TestCacheRunScopePreventsCrossRunHits: run-local epoch counters are not
// comparable across runs — and a resumed run restarts its counter — so an
// entry compiled under one run must never be served to another, regardless of
// epoch equality. The audit's epoch-0 finding (every run's pre-barrier
// compiles sharing one key) is this test's target.
func TestCacheRunScopePreventsCrossRunHits(t *testing.T) {
	ids := []knowledge.ChunkID{"chunk:r1"}
	contents := map[string]string{"chunk:r1": "cross-run body"}
	c, store, _ := newEpochTestCompiler(t, ids)
	now := time.Unix(1000, 0)
	for _, id := range ids {
		saveTrustedChunk(t, store, string(id), contents[string(id)], now)
	}

	reqA := CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "cross-run probe"},
		MaxTokens: 4096,
		EpochID:   1,
		RunID:     "run-A",
	}
	missA, missRecordA, err := c.Compile(context.Background(), reqA)
	require.NoError(t, err)
	require.False(t, missRecordA.CacheHit)
	require.Len(t, missA.Chunks, 1)

	hitA, hitRecordA, err := c.Compile(context.Background(), reqA)
	require.NoError(t, err)
	require.True(t, hitRecordA.CacheHit, "same run, same epoch: hit")
	require.Equal(t, missA, hitA)

	// A different run at the SAME epoch must miss — its epoch means a
	// different memory state.
	reqB := reqA
	reqB.RunID = "run-B"
	_, recordB, err := c.Compile(context.Background(), reqB)
	require.NoError(t, err)
	require.False(t, recordB.CacheHit, "a different run at the same epoch must never hit run A's entry")

	// A resumed run (fresh identity, counter restarted to the same opening
	// epoch) must also miss.
	reqResume := reqA
	reqResume.RunID = "run-A-resumed"
	_, recordResume, err := c.Compile(context.Background(), reqResume)
	require.NoError(t, err)
	require.False(t, recordResume.CacheHit, "a resumed run's opening epoch must never hit the paused run's entry")

	// Scope-less compiles (no run coordinator) keep a shared namespace by
	// construction: nothing can ground without a coordinator.
	reqScopeless := reqA
	reqScopeless.RunID = ""
	_, recordScopeless, err := c.Compile(context.Background(), reqScopeless)
	require.NoError(t, err)
	require.False(t, recordScopeless.CacheHit)
}

package compiler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
)

func newBoundedTestCompiler(t *testing.T) *Compiler {
	t.Helper()
	c := NewCompiler(nil, nil, nil)
	now := time.Unix(1000, 0)
	c.SetTimeFunc(func() time.Time { return now })
	return c
}

// TestCompilationCacheEvictsAtCap: 1,000 distinct keys leave exactly the cap
// (256) entries and 744 capacity evictions (NFR-4).
func TestCompilationCacheEvictsAtCap(t *testing.T) {
	c := newBoundedTestCompiler(t)
	for i := 0; i < 1000; i++ {
		key := CacheKey{QueryFingerprint: keyDigitString(i)}
		c.cache.put(key, &cacheEntry{record: CompilationRecord{RequestID: keyDigitString(i)}})
	}
	require.Equal(t, compilationCacheCap, c.cache.entries.Len())
	require.Equal(t, uint64(1000-compilationCacheCap), c.cache.entries.Evicted())
}

// TestCompilationCacheTTLExpiry: entries past the TTL are gone (fake clock)
// and counted, bounding staleness when an invalidation event was dropped.
func TestCompilationCacheTTLExpiry(t *testing.T) {
	c := newBoundedTestCompiler(t)
	now := time.Unix(1000, 0)
	c.SetTimeFunc(func() time.Time { return now })
	key := CacheKey{QueryFingerprint: "ttl"}
	c.cache.put(key, &cacheEntry{record: CompilationRecord{RequestID: "ttl"}})
	require.NotNil(t, c.cache.get(key))

	c.SetTimeFunc(func() time.Time { return now.Add(2 * compilationCacheTTL) })
	require.Nil(t, c.cache.get(key))
	require.Equal(t, uint64(1), c.cache.entries.Expired())
}

// TestCompilationCacheInvalidationOnCommit: a chunk ingested after a compile
// invalidates the cached entry — the next compile of the same logical request
// recomputes (cache miss), not serves stale.
func TestCompilationCacheInvalidationOnCommit(t *testing.T) {
	store := newCompilerTestStore(t)
	ids := []knowledge.ChunkID{"chunk:a"}
	registry := retrieval.NewRankerRegistry()
	ranker := &staticRanker{name: "static", ids: ids}
	registry.Register(ranker)
	retriever := retrieval.NewRetriever(registry, store)
	c := NewCompiler(retriever, nil, store)
	now := time.Unix(1000, 0)
	c.SetTimeFunc(func() time.Time { return now })
	c.SetRepository(NewCompilerRepository(newCompilerTestEngine(t)))

	saveTrustedChunk(t, store, "chunk:a", "alpha", now)
	req := CompilationRequest{Query: retrieval.RetrievalQuery{Text: "q"}, MaxTokens: 64}

	_, first, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.CacheHit)

	_, second, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.True(t, second.CacheHit)

	// A new chunk version for the same content supersedes the old one: the
	// cached entry touching it must not be served again.
	c.handleChunkInvalidated("chunk:a")
	_, third, err := c.Compile(context.Background(), req)
	require.NoError(t, err)
	require.False(t, third.CacheHit, "entry must be invalidated by chunk commit")
}

// TestCompilerStartAfterStop: Start-after-Stop is a typed error (no zombie
// invalidation loops); Stop joins the loop deterministically.
func TestCompilerStartAfterStop(t *testing.T) {
	defer goleak.VerifyNone(t)
	c := NewCompiler(nil, nil, nil)
	c.SetEventBus(&knowledge.EventBus{})
	require.NoError(t, c.Start(context.Background()))
	c.Stop()
	require.ErrorIs(t, c.Start(context.Background()), ErrCompilerStopped)
}

// TestCompilerStartOnce: a second Start while running is an error.
func TestCompilerStartOnce(t *testing.T) {
	defer goleak.VerifyNone(t)
	c := NewCompiler(nil, nil, nil)
	c.SetEventBus(&knowledge.EventBus{})
	require.NoError(t, c.Start(context.Background()))
	defer c.Stop()
	require.Error(t, c.Start(context.Background()))
}

// TestCompilerEventBusInvalidation: chunk events on the knowledge bus drive
// cache invalidation end-to-end.
func TestCompilerEventBusInvalidation(t *testing.T) {
	defer goleak.VerifyNone(t)
	bus := &knowledge.EventBus{}
	c := NewCompiler(nil, nil, nil)
	c.SetEventBus(bus)
	require.NoError(t, c.Start(context.Background()))
	defer c.Stop()

	key := CacheKey{QueryFingerprint: "bus"}
	c.cache.put(key, &cacheEntry{
		record: CompilationRecord{RequestID: "bus"},
		deps:   map[knowledge.ChunkID]struct{}{"chunk:b": {}},
	})
	require.NotNil(t, c.cache.get(key))

	bus.Publish(knowledge.Event{
		Kind: knowledge.EventChunkIngested,
		Payload: knowledge.ChunkIngestedPayload{
			ChunkID: "chunk:b",
		},
	})
	// The consumer goroutine processes asynchronously; poll briefly.
	deadline := time.Now().Add(2 * time.Second)
	for c.cache.get(key) != nil {
		if time.Now().After(deadline) {
			t.Fatal("chunk-ingested event did not invalidate the dependent entry")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCacheKeyIgnoresEventLogSeq: two compiles of the same logical request
// share one cache entry regardless of appended events.
func TestCacheKeyIgnoresEventLogSeq(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	base := CompilationRequest{Query: retrieval.RetrievalQuery{Text: "same"}}
	bumped := base
	bumped.EventLogSeq = base.EventLogSeq + 7
	require.Equal(t, c.buildCacheKey(base), c.buildCacheKey(bumped))
}

func keyDigitString(i int) string {
	if i == 0 {
		return "k0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return "k" + string(digits)
}

func saveTrustedChunk(t *testing.T, store *knowledge.ChunkStore, id, content string, now time.Time) {
	t.Helper()
	_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
		ID:         knowledge.ChunkID(id),
		TrustClass: "builtin.trusted",
		Body:       knowledge.ChunkBody{Raw: content, Fields: map[string]any{"content": content}},
		Provenance: knowledge.ChunkProvenance{Timestamp: now},
	})
	require.NoError(t, err)
}

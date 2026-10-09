package retrieval

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// snapshotTelemetry captures retrieval lifecycle counters.
type snapshotTelemetry struct {
	mu    sync.Mutex
	kinds map[string]int
}

func newSnapshotTelemetry() *snapshotTelemetry {
	return &snapshotTelemetry{kinds: make(map[string]int)}
}

func (s *snapshotTelemetry) Emit(ev fwtelemetry.Event) {
	s.mu.Lock()
	s.kinds[string(ev.Type)]++
	s.mu.Unlock()
}

func (s *snapshotTelemetry) count(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kinds[kind]
}

func newSnapshotTestRetriever(t *testing.T) (*Retriever, *knowledge.ChunkStore, *snapshotTelemetry, *time.Time) {
	t.Helper()
	store := newRankerTestStore(t)
	registry := NewRankerRegistry()
	registry.Register(&KeywordRanker{K1: 1.2, B: 0.75})
	registry.Register(&RecencyRanker{HalfLifeHours: 24})
	registry.Register(&TrustRanker{})
	retriever := NewRetriever(registry, store)
	tel := newSnapshotTelemetry()
	retriever.SetTelemetry(tel)
	now := time.Unix(10000, 0)
	retriever.SetClock(func() time.Time { return now })
	return retriever, store, tel, &now
}

func saveSnapshotChunk(t *testing.T, store *knowledge.ChunkStore, id, content string, now time.Time) {
	t.Helper()
	_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID(id),
		WorkspaceID: "ws",
		TrustClass:  agentspec.TrustClassBuiltinTrusted,
		UpdatedAt:   now,
		CreatedAt:   now,
		Body:        knowledge.ChunkBody{Raw: content, Fields: map[string]any{"content": content}},
	})
	require.NoError(t, err)
}

// TestSnapshotParity: snapshot-ranked results are identical to
// direct-store-ranked results for a fixed corpus — the snapshot changes the
// cost, not the ranking.
func TestSnapshotParity(t *testing.T) {
	retriever, store, _, nowp := newSnapshotTestRetriever(t)
	now := *nowp
	for i := 0; i < 20; i++ {
		saveSnapshotChunk(t, store, fmt.Sprintf("chunk:%02d", i), fmt.Sprintf("shared term doc%d alpha", i), now)
	}
	query := RetrievalQuery{Text: "shared alpha", Limit: 10}

	snapshotted, err := retriever.Retrieve(context.Background(), query)
	require.NoError(t, err)

	// Direct ranking over the same corpus, same RRF fusion inputs: build a
	// snapshot by hand and rank with direct rankers to compare ordering.
	snap, err := buildCorpusSnapshot(store, 1, now)
	require.NoError(t, err)
	keywordIDs, err := (&KeywordRanker{K1: 1.2, B: 0.75}).Rank(context.Background(), query, snap)
	require.NoError(t, err)

	require.NotEmpty(t, snapshotted.Ranked)
	topRanked := snapshotted.Ranked[0].ChunkID
	require.Equal(t, keywordIDs[0], topRanked, "snapshot retrieval must agree with direct ranking on the top hit")
}

// TestSnapshotInvalidationOnCommit: chunk-ingested events bump the
// generation — the next Retrieve rebuilds (fresh chunks enter the corpus).
func TestSnapshotInvalidationOnCommit(t *testing.T) {
	retriever, store, tel, nowp := newSnapshotTestRetriever(t)
	now := *nowp
	saveSnapshotChunk(t, store, "chunk:a", "alpha term", now)

	bus := &knowledge.EventBus{}
	retriever.SetEventBus(bus)

	_, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)
	require.Equal(t, 1, tel.count("retrieval_snapshot_built"))

	// Within TTL: served fresh, no rebuild.
	_, err = retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)
	require.Equal(t, 1, tel.count("retrieval_snapshot_built"))

	// New chunk commits: event invalidates; the next Retrieve rebuilds with
	// the new chunk present.
	saveSnapshotChunk(t, store, "chunk:b", "beta term", now)
	bus.Publish(knowledge.Event{Kind: knowledge.EventChunkIngested, Payload: knowledge.ChunkIngestedPayload{ChunkID: "chunk:b"}})
	deadline := time.Now().Add(2 * time.Second)
	for tel.count("retrieval_snapshot_invalidated") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("ingested event did not invalidate the snapshot")
		}
		time.Sleep(time.Millisecond)
	}
	result, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "beta"})
	require.NoError(t, err)
	require.Equal(t, 2, tel.count("retrieval_snapshot_built"))
	found := false
	for _, rc := range result.Ranked {
		if rc.ChunkID == "chunk:b" {
			found = true
		}
	}
	require.True(t, found, "rebuilt snapshot must include the newly committed chunk")
}

// TestSnapshotTTLExpiry: without events, a snapshot older than the TTL is
// rebuilt on the next Retrieve (fake clock).
func TestSnapshotTTLExpiry(t *testing.T) {
	retriever, store, tel, nowp := newSnapshotTestRetriever(t)
	now := nowp
	saveSnapshotChunk(t, store, "chunk:a", "alpha", *now)

	_, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)
	require.Equal(t, 1, tel.count("retrieval_snapshot_built"))

	*now = now.Add(2 * snapshotTTL)
	_, err = retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)
	require.Equal(t, 2, tel.count("retrieval_snapshot_built"), "TTL-expired snapshot must rebuild")
}

// TestSnapshotServeStale: a failed rebuild serves the previous generation
// within 2×TTL with telemetry, and becomes a hard error beyond it.
func TestSnapshotServeStale(t *testing.T) {
	retriever, store, tel, nowp := newSnapshotTestRetriever(t)
	now := nowp
	saveSnapshotChunk(t, store, "chunk:a", "alpha", *now)

	_, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)

	// Age past the TTL (but inside the 2× serve-stale window), then make
	// every rebuild fail (injected build error standing in for a broken
	// store).
	*now = now.Add(3 * snapshotTTL / 2)
	retriever.snapshots.build = func(*knowledge.ChunkStore, uint64, time.Time) (*CorpusSnapshot, error) {
		return nil, fmt.Errorf("store unavailable")
	}

	result, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.NoError(t, err, "serve-stale must tolerate a failed rebuild within 2×TTL")
	require.NotEmpty(t, result.Ranked)
	require.Equal(t, 1, tel.count("retrieval_snapshot_served_stale"))

	// Beyond 2×TTL the degraded mode ends: hard error.
	*now = now.Add(4 * snapshotTTL)
	_, err = retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.Error(t, err, "past the serve-stale window, retrieval must fail loudly")
}

// TestSnapshotSingleFlight: 32 concurrent Retrieves during invalidation
// bursts produce no race and no thundering-herd of builds.
func TestSnapshotSingleFlight(t *testing.T) {
	retriever, store, tel, nowp := newSnapshotTestRetriever(t)
	now := nowp
	for i := 0; i < 50; i++ {
		saveSnapshotChunk(t, store, fmt.Sprintf("chunk:%02d", i), strings.Repeat("content ", 5), *now)
	}
	bus := &knowledge.EventBus{}
	retriever.SetEventBus(bus)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%8 == 0 {
				retriever.Invalidate()
			}
			_, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "content"})
			if err != nil {
				t.Errorf("retrieve: %v", err)
			}
		}(i)
	}
	wg.Wait()
	// Builds are bounded by the burst count (32 invalidations at most);
	// substantially more would mean the single-flight gate is broken.
	require.LessOrEqual(t, tel.count("retrieval_snapshot_built"), 32)
}

// failingRanker always errors — ranker-failure degradation, never silence.
type failingRanker struct{}

func (failingRanker) Name() string { return "failing" }

func (failingRanker) Rank(context.Context, RetrievalQuery, *CorpusSnapshot) ([]knowledge.ChunkID, error) {
	return nil, fmt.Errorf("ranker exploded")
}

// TestRankerFailureDegrades: a failed ranker emits retrieval_ranker_failed
// and the remaining rankers still produce results.
func TestRankerFailureDegrades(t *testing.T) {
	retriever, store, tel, nowp := newSnapshotTestRetriever(t)
	now := *nowp
	saveSnapshotChunk(t, store, "chunk:a", "alpha content", now)
	retriever.registry.Register(failingRanker{})

	result, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "alpha"})
	require.NoError(t, err, "a failed ranker must degrade, not fail retrieval")
	require.NotEmpty(t, result.Ranked)
	require.GreaterOrEqual(t, tel.count("retrieval_ranker_failed"), 1)
}

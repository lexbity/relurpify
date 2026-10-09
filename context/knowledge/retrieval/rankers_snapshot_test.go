package retrieval

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
)

// TestRecencyComparatorUsesSnapshotIndex: the recency tiebreak must resolve
// through the snapshot's O(1) index. An O(N) per-comparison corpus scan on a
// 400-chunk corpus would explode the ranker's wall time; the budget below is
// generous for correctness but impossible for a rescan.
func TestRecencyComparatorUsesSnapshotIndex(t *testing.T) {
	store := newRankerTestStore(t)
	base := time.Unix(100000, 0)
	for i := 0; i < 400; i++ {
		id := fmt.Sprintf("chunk:%03d", i)
		saveRankerChunk(t, store, id, "filler content for scale", base.Add(time.Duration(i)*time.Second), agentspec.TrustClassBuiltinTrusted, "")
	}
	snap, err := buildCorpusSnapshot(store, 1, base)
	require.NoError(t, err)

	ranker := &RecencyRanker{HalfLifeHours: 24, Now: func() time.Time { return base }}
	start := time.Now()
	ids, err := ranker.Rank(context.Background(), RetrievalQuery{}, snap)
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Len(t, ids, 400)
	// 400 ids sorted with an O(N) chunkByID inside the comparator would do
	// roughly 160k full-corpus scans (about 64M chunk visits), far beyond
	// this budget.
	require.Less(t, elapsed, 50*time.Millisecond, "recency comparator must not rescan the corpus per comparison")

	// Ordering: strictly newest-first.
	for i := 1; i < len(ids); i++ {
		a, _ := snap.Lookup(ids[i-1])
		b, _ := snap.Lookup(ids[i])
		require.False(t, a.UpdatedAt.Before(b.UpdatedAt), "recency order violated at %d", i)
	}
}

// TestDebugSeedParityWithWorkspaceScan: DebugSeed(workspace) returns the same
// chunk IDs as scanning the workspace-scoped chunk set for matching
// provenance — the workspace lookup replaces the full-store scan without
// changing results.
func TestDebugSeedParityWithWorkspaceScan(t *testing.T) {
	store := newRankerTestStore(t)
	saveProvenanceChunk := func(id, workspace, ref string) {
		t.Helper()
		_, err := store.Save(context.Background(), knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID(id),
			WorkspaceID: workspace,
			Provenance:  knowledge.ChunkProvenance{Sources: []knowledge.ProvenanceSource{{Kind: "tension", Ref: ref}}},
			Body:        knowledge.ChunkBody{Raw: "body " + id},
		})
		require.NoError(t, err)
	}
	saveProvenanceChunk("chunk:t1", "ws-a", "tension-1")
	saveProvenanceChunk("chunk:t2", "ws-a", "tension-2")
	saveProvenanceChunk("chunk:other-ws", "ws-b", "tension-1")

	streamer := &knowledge.Streamer{Store: store}
	seed, err := streamer.DebugSeed("ws-a", nil, []string{"tension-1"})
	require.NoError(t, err)
	require.ElementsMatch(t, []knowledge.ChunkID{"chunk:t1"}, seed.ChunkIDs)

	// Empty tension refs: only the file-seeded IDs (none here).
	seed, err = streamer.DebugSeed("ws-a", nil, nil)
	require.NoError(t, err)
	require.Empty(t, seed.ChunkIDs)
}

// Benchmarks (NFR-5): retrieval p99 targets at 10k and 100k chunks. Run with
// -benchtime and -benchmem; the snapshot amortizes the corpus decode to once
// per generation instead of once per ranker per query.
func benchmarkCorpus(b *testing.B, chunks int) *Retriever {
	store := newRankerBenchStore(b, chunks)
	registry := NewRankerRegistry()
	registry.Register(&KeywordRanker{K1: 1.2, B: 0.75})
	registry.Register(&RecencyRanker{HalfLifeHours: 24})
	registry.Register(&TrustRanker{})
	retriever := NewRetriever(registry, store)
	// Warm the snapshot generation so the benchmark measures the steady
	// per-query path, not the one-time build.
	if _, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "warm"}); err != nil {
		b.Fatal(err)
	}
	return retriever
}

func BenchmarkRetrieve10k(b *testing.B) {
	retriever := benchmarkCorpus(b, 10_000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "needle term"}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRetrieve100k(b *testing.B) {
	if testing.Short() {
		b.Skip("100k corpus benchmark is a long-running measurement")
	}
	retriever := benchmarkCorpus(b, 100_000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := retriever.Retrieve(context.Background(), RetrievalQuery{Text: "needle term"}); err != nil {
			b.Fatal(err)
		}
	}
}

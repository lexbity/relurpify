package compiler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
)

// mutableRanker ranks a mutable ID list so a test can inject
// non-determinism between the original compile and the replay.
type mutableRanker struct {
	name string
	ids  []knowledge.ChunkID
}

func (r *mutableRanker) Name() string { return r.name }

func (r *mutableRanker) Rank(context.Context, retrieval.RetrievalQuery, *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	return append([]knowledge.ChunkID(nil), r.ids...), nil
}

// replayHarness builds a compiler whose record path runs through a real
// graph repository and whose ranking is controllable.
type replayHarness struct {
	compiler *Compiler
	ranker   *mutableRanker
	store    *knowledge.ChunkStore
}

func newReplayHarness(t *testing.T) *replayHarness {
	t.Helper()
	store := newCompilerTestStore(t)
	ranker := &mutableRanker{name: "replay", ids: []knowledge.ChunkID{"chunk:r1"}}
	registry := retrieval.NewRankerRegistry()
	registry.Register(ranker)
	c := NewCompiler(retrieval.NewRetriever(registry, store), nil, store)
	now := time.Unix(1000, 0)
	c.SetTimeFunc(func() time.Time { return now })
	c.SetIDGenerator(func() string { return "replay-req" })
	c.SetRepository(NewCompilerRepository(newCompilerTestEngine(t)))
	return &replayHarness{compiler: c, ranker: ranker, store: store}
}

// TestStrictReplayGoldenRecordPasses: replaying a deterministic compilation
// recomputes (cache bypassed) and matches the recorded digest.
func TestStrictReplayGoldenRecordPasses(t *testing.T) {
	h := newReplayHarness(t)
	saveTrustedChunk(t, h.store, "chunk:r1", "stable content", time.Unix(1000, 0))

	req := CompilationRequest{
		Query:       retrieval.RetrievalQuery{Text: "replay"},
		MaxTokens:   64,
		EventLogSeq: 7,
	}
	_, original, err := h.compiler.Compile(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, original.DeterministicDigest)

	result, newRecord, diff, err := h.compiler.Replay(context.Background(), original.RequestID, StrictReplay)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, newRecord.CacheHit, "strict replay must bypass the cache")
	require.True(t, diff.DeterminismMatch)
}

// TestStrictReplayDetectsNondeterminism: when the recomputation yields a
// different digest, replay fails with the typed ErrReplayMismatch error —
// a cache hit can never mask it.
func TestStrictReplayDetectsNondeterminism(t *testing.T) {
	h := newReplayHarness(t)
	saveTrustedChunk(t, h.store, "chunk:r1", "first content", time.Unix(1000, 0))

	req := CompilationRequest{Query: retrieval.RetrievalQuery{Text: "replay"}, MaxTokens: 64}
	_, original, err := h.compiler.Compile(context.Background(), req)
	require.NoError(t, err)

	// Inject non-determinism: ranking now returns a different chunk set.
	saveTrustedChunk(t, h.store, "chunk:r2", "second content", time.Unix(1000, 0))
	h.ranker.ids = []knowledge.ChunkID{"chunk:r2"}

	_, _, _, err = h.compiler.Replay(context.Background(), original.RequestID, StrictReplay)
	require.ErrorIs(t, err, ErrReplayMismatch)
}

// countingRepository fails its test if any list/scan-style operation runs
// during record lookups — replay and diff must be pure Get-by-ID paths.
type countingRepository struct {
	Repository
	listCalls int
	failer    *testing.T
}

func (r *countingRepository) ListCompilationRecords(ctx context.Context, seq uint64) ([]CompilationRecord, error) {
	r.listCalls++
	r.failer.Fatal("ListCompilationRecords called during record lookup — the O(1) Get path was bypassed")
	return nil, nil
}

// TestRecordLookupsNeverScan: Replay and DiffByID go through Get-by-ID only.
func TestRecordLookupsNeverScan(t *testing.T) {
	h := newReplayHarness(t)
	saveTrustedChunk(t, h.store, "chunk:r1", "content", time.Unix(1000, 0))

	reqA := CompilationRequest{Query: retrieval.RetrievalQuery{Text: "a"}, MaxTokens: 64}
	reqB := CompilationRequest{Query: retrieval.RetrievalQuery{Text: "b"}, MaxTokens: 64}
	h.compiler.SetIDGenerator(seqID("reqA"))
	_, recordA, err := h.compiler.Compile(context.Background(), reqA)
	require.NoError(t, err)
	h.compiler.SetIDGenerator(seqID("reqB"))
	_, recordB, err := h.compiler.Compile(context.Background(), reqB)
	require.NoError(t, err)

	inner := h.compiler.repository
	spy := &countingRepository{Repository: inner, failer: t}
	h.compiler.repository = spy

	_, _, _, err = h.compiler.Replay(context.Background(), recordA.RequestID, StrictReplay)
	require.NoError(t, err)
	_, err = h.compiler.DiffByID(context.Background(), recordA.RequestID, recordB.RequestID)
	require.NoError(t, err)
	require.Equal(t, 0, spy.listCalls)
}

// TestLoadCompilationRecordTypedMiss: loading an unknown ID surfaces the
// repository's typed not-found error.
func TestLoadCompilationRecordTypedMiss(t *testing.T) {
	c := NewCompiler(nil, nil, nil)
	c.SetRepository(NewCompilerRepository(newCompilerTestEngine(t)))
	_, err := c.LoadCompilationRecord(context.Background(), "no-such-record")
	require.ErrorIs(t, err, ErrCompilationRecordNotFound)
}

func seqID(id string) func() string {
	n := 0
	return func() string {
		n++
		if n == 1 {
			return id
		}
		return id + "-" + string(rune('0'+n))
	}
}

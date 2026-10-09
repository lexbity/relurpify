package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// filePathField is the chunk body field carrying the source file path.
const filePathField = "file_path"

// saveChunk is a small fixture writer shared by the store tests below.
func saveChunk(t *testing.T, store *ChunkStore, id ChunkID, filePath, coverageHash string) KnowledgeChunk {
	t.Helper()
	saved, err := store.Save(context.Background(), KnowledgeChunk{
		ID:           id,
		WorkspaceID:  "ws",
		CoverageHash: coverageHash,
		Provenance: ChunkProvenance{
			CodeStateRef: "cs-1",
			CompiledBy:   CompilerDeterministic,
			Timestamp:    time.Now().UTC(),
		},
		Body: ChunkBody{Raw: string(id), Fields: map[string]any{filePathField: filePath}},
	})
	require.NoError(t, err)
	return *saved
}

// TestFindFreshByFilePathFiltersStale proves the freshness projection drops
// stale chunks while the raw file-path lookup still returns them.
func TestFindFreshByFilePathFiltersStale(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	saveChunk(t, store, "chunk:fresh", "src/a.go", "cov-1")
	saveChunk(t, store, "chunk:stale", "src/a.go", "cov-1")

	require.NoError(t, store.MarkStale(ctx, []ChunkID{"chunk:stale"}, "test stale"))

	all, err := store.FindByFilePath("src/a.go")
	require.NoError(t, err)
	require.Len(t, all, 2)

	fresh, err := store.FindFreshByFilePath("src/a.go")
	require.NoError(t, err)
	require.Len(t, fresh, 1)
	require.Equal(t, ChunkID("chunk:fresh"), fresh[0].ID)

	// A path with no chunks returns an empty result and no error.
	empty, err := store.FindFreshByFilePath("src/missing.go")
	require.NoError(t, err)
	require.Empty(t, empty)
}

// TestMarkStaleByCoverageHashFlipsFreshness proves the coverage-hash sweep marks
// exactly the matching chunks stale, records the reason, and leaves others valid.
func TestMarkStaleByCoverageHashFlipsFreshness(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	saveChunk(t, store, "chunk:c1", "src/a.go", "cov-x")
	saveChunk(t, store, "chunk:c2", "src/b.go", "cov-x")
	saveChunk(t, store, "chunk:c3", "src/c.go", "cov-y")

	require.NoError(t, store.MarkStaleByCoverageHash(ctx, "cov-x"))

	for _, id := range []ChunkID{"chunk:c1", "chunk:c2"} {
		chunk, ok, err := store.Load(id)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, FreshnessStale, chunk.Freshness)
		require.Equal(t, "coverage_hash_changed", chunk.Body.Fields["stale_reason"])
	}
	c3, ok, err := store.Load("chunk:c3")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessValid, c3.Freshness)

	// An empty coverage hash is a no-op.
	require.NoError(t, store.MarkStaleByCoverageHash(ctx, ""))
}

// TestStoreLoadManyDeleteAndFind covers the batch read, delete, and matcher
// read paths that the save/load happy path does not reach.
func TestStoreLoadManyDeleteAndFind(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	saveChunk(t, store, "chunk:m1", "src/a.go", "cov-1")
	saveChunk(t, store, "chunk:m2", "src/b.go", "cov-2")

	many, err := store.LoadMany([]ChunkID{"chunk:m1", "chunk:m2", "chunk:absent"})
	require.NoError(t, err)
	require.Len(t, many, 2)

	byState, err := store.FindByCodeStateRef("cs-1")
	require.NoError(t, err)
	require.Len(t, byState, 2)

	byWS, err := store.FindByWorkspace("ws")
	require.NoError(t, err)
	require.Len(t, byWS, 2)

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 2)

	require.NoError(t, store.Delete(ctx, "chunk:m1"))
	_, ok, err := store.Load("chunk:m1")
	require.NoError(t, err)
	require.False(t, ok)

	// An empty id is a no-op.
	require.NoError(t, store.Delete(ctx, ""))
}

// TestStoreLoadEdge covers the point edge lookup (found and absent).
func TestStoreLoadEdge(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	from := saveChunk(t, store, "chunk:e1", "src/a.go", "cov-1")
	to := saveChunk(t, store, "chunk:e2", "src/b.go", "cov-1")

	_, err := store.SaveEdge(ctx, ChunkEdge{FromChunk: from.ID, ToChunk: to.ID, Kind: EdgeKindRequiresContext, Weight: 0.5})
	require.NoError(t, err)

	edge, ok, err := store.LoadEdge(from.ID, to.ID, EdgeKindRequiresContext)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, from.ID, edge.FromChunk)

	_, ok, err = store.LoadEdge(from.ID, ChunkID("chunk:nope"), EdgeKindRequiresContext)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestKnowledgeHelpers covers the small pure helpers used across the package.
func TestKnowledgeHelpers(t *testing.T) {
	require.Equal(t, 0, estimateTokens(""))
	require.GreaterOrEqual(t, estimateTokens("abcdefgh"), 1)
	require.Equal(t, "x", firstNonEmpty("", "  ", "x"))
	require.Empty(t, firstNonEmpty("", "  "))
	require.Nil(t, cloneMap(nil))
	require.Nil(t, cloneMap(map[string]any{}))
	src := map[string]any{"k": 1}
	cp := cloneMap(src)
	require.Equal(t, src, cp)
	cp["k"] = 2
	require.Equal(t, 1, src["k"], "cloneMap must copy, not alias")
	require.Equal(t, []string{"a", "b"}, chunkIDsToStrings([]ChunkID{"a", "", "b"}))
}

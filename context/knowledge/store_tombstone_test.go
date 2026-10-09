package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func saveTombstoneFixture(t *testing.T, store *ChunkStore) {
	t.Helper()
	ctx := context.Background()
	prov := ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}

	_, err := store.Save(ctx, KnowledgeChunk{
		ID:           "chunk:capture:live",
		WorkspaceID:  "ws",
		ContentHash:  "hash-live",
		CoverageHash: "cov-live",
		Provenance:   prov,
		Body:         ChunkBody{Raw: "live", Fields: map[string]any{"file_path": "src/live.go"}},
	})
	require.NoError(t, err)

	_, err = store.Save(ctx, KnowledgeChunk{
		ID:           "chunk:capture:dead",
		WorkspaceID:  "ws",
		ContentHash:  "hash-dead",
		CoverageHash: "cov-dead",
		Provenance:   prov,
		Body:         ChunkBody{Raw: "dead", Fields: map[string]any{"file_path": "src/dead.go"}},
	})
	require.NoError(t, err)

	require.NoError(t, store.Tombstone(ctx, "chunk:capture:dead", "chunk:capture:live"))
}

// TestChunkStoreFindMethodsExcludeTombstoned proves every list path honors
// tombstone filtering: a tombstoned chunk is absent from all Find* results but
// remains reachable through the explicit include-tombstones primitive.
func TestChunkStoreFindMethodsExcludeTombstoned(t *testing.T) {
	store := newTestStore(t)
	saveTombstoneFixture(t, store)

	assertLiveOnly := func(t *testing.T, chunks []KnowledgeChunk) {
		t.Helper()
		ids := make(map[ChunkID]bool, len(chunks))
		for _, chunk := range chunks {
			require.False(t, chunk.Tombstoned, "list query returned a tombstoned chunk %s", chunk.ID)
			ids[chunk.ID] = true
		}
		require.True(t, ids["chunk:capture:live"], "live chunk must be present")
		require.False(t, ids["chunk:capture:dead"], "tombstoned chunk must be absent")
	}

	byWorkspace, err := store.FindByWorkspace("ws")
	require.NoError(t, err)
	assertLiveOnly(t, byWorkspace)

	byCoverage, err := store.FindByCoverageHash("cov-dead")
	require.NoError(t, err)
	require.Empty(t, byCoverage, "tombstoned coverage hash must resolve to nothing")

	byCoverageLive, err := store.FindByCoverageHash("cov-live")
	require.NoError(t, err)
	assertLiveOnly(t, byCoverageLive)

	byHash, err := store.FindByContentHash("hash-dead")
	require.NoError(t, err)
	require.Empty(t, byHash, "tombstoned content hash must resolve to nothing")

	byFilePath, err := store.FindByFilePath("src/dead.go")
	require.NoError(t, err)
	require.Empty(t, byFilePath, "tombstoned file path must resolve to nothing")

	byPrefix, err := store.FindByFilePathPrefix("src")
	require.NoError(t, err)
	assertLiveOnly(t, byPrefix)

	fresh, err := store.FindFreshByFilePath("src/live.go")
	require.NoError(t, err)
	assertLiveOnly(t, fresh)

	all, err := store.FindAll()
	require.NoError(t, err)
	assertLiveOnly(t, all)

	raw, ok, err := store.LoadIncludingTombstoned("chunk:capture:dead")
	require.NoError(t, err)
	require.True(t, ok, "raw load must reach the tombstoned chunk")
	require.True(t, raw.Tombstoned)
}

// TestChunkStoreSavePreservesTombstone proves equal-or-older derivation cannot
// clear a tombstone, and that Save returns the untouched stored record.
func TestChunkStoreSavePreservesTombstone(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	prov := ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}

	_, err := store.Save(ctx, KnowledgeChunk{
		ID:                   "chunk:capture:retracted",
		WorkspaceID:          "ws",
		DerivationGeneration: 3,
		Provenance:           prov,
		Body:                 ChunkBody{Raw: "retracted"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Tombstone(ctx, "chunk:capture:retracted", ""))

	prior, ok, err := store.LoadIncludingTombstoned("chunk:capture:retracted")
	require.NoError(t, err)
	require.True(t, ok)
	priorVersion := prior.Version

	for _, generation := range []int{0, 3} {
		_, err := store.Save(ctx, KnowledgeChunk{
			ID:                   "chunk:capture:retracted",
			WorkspaceID:          "ws",
			DerivationGeneration: generation,
			Provenance:           prov,
			Body:                 ChunkBody{Raw: "retracted"},
		})
		require.NoError(t, err)
	}

	stored, ok, err := store.LoadIncludingTombstoned("chunk:capture:retracted")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, stored.Tombstoned, "tombstone must survive equal-or-older derivation")
	require.Equal(t, priorVersion, stored.Version, "preserved tombstone must not bump version")

	visible, err := store.FindByWorkspace("ws")
	require.NoError(t, err)
	require.Empty(t, visible)
}

// TestChunkStoreSaveResurrectsOnNewerDerivation proves only a strictly greater
// derivation generation may clear a tombstone, and that the resurrected chunk
// becomes visible to list queries again.
func TestChunkStoreSaveResurrectsOnNewerDerivation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	prov := ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}

	_, err := store.Save(ctx, KnowledgeChunk{
		ID:                   "chunk:capture:reborn",
		WorkspaceID:          "ws",
		DerivationGeneration: 1,
		Provenance:           prov,
		Body:                 ChunkBody{Raw: "reborn", Fields: map[string]any{"file_path": "src/reborn.go"}},
	})
	require.NoError(t, err)
	require.NoError(t, store.Tombstone(ctx, "chunk:capture:reborn", "chunk:capture:other"))

	prior, ok, err := store.LoadIncludingTombstoned("chunk:capture:reborn")
	require.NoError(t, err)
	require.True(t, ok)

	saved, err := store.Save(ctx, KnowledgeChunk{
		ID:                   "chunk:capture:reborn",
		WorkspaceID:          "ws",
		DerivationGeneration: 2,
		Provenance:           prov,
		Body:                 ChunkBody{Raw: "reborn", Fields: map[string]any{"file_path": "src/reborn.go"}},
	})
	require.NoError(t, err)
	require.False(t, saved.Tombstoned)
	require.Equal(t, FreshnessValid, saved.Freshness)
	require.Empty(t, saved.SupersededBy)
	require.Greater(t, saved.Version, prior.Version)

	fresh, err := store.FindFreshByFilePath("src/reborn.go")
	require.NoError(t, err)
	require.Len(t, fresh, 1)
	require.False(t, fresh[0].Tombstoned)
}

// TestChunkStoreSavePreservesFreshnessWhenUnset proves a caller that omits
// freshness cannot accidentally refresh a stale chunk.
func TestChunkStoreSavePreservesFreshnessWhenUnset(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	prov := ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}

	_, err := store.Save(ctx, KnowledgeChunk{
		ID:          "chunk:capture:stale",
		WorkspaceID: "ws",
		Freshness:   FreshnessStale,
		Provenance:  prov,
		Body:        ChunkBody{Raw: "stale"},
	})
	require.NoError(t, err)

	saved, err := store.Save(ctx, KnowledgeChunk{
		ID:          "chunk:capture:stale",
		WorkspaceID: "ws",
		Provenance:  prov,
		Body:        ChunkBody{Raw: "stale"},
	})
	require.NoError(t, err)
	require.Equal(t, FreshnessStale, saved.Freshness)
}

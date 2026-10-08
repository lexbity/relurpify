package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChunkStoreLoadHidesTombstoned(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	prov := ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}

	_, err := store.Save(ctx, KnowledgeChunk{ID: "chunk:live", WorkspaceID: "ws", Provenance: prov, Body: ChunkBody{Raw: "live"}})
	require.NoError(t, err)
	_, err = store.Save(ctx, KnowledgeChunk{ID: "chunk:dead", WorkspaceID: "ws", Provenance: prov, Body: ChunkBody{Raw: "dead"}})
	require.NoError(t, err)

	require.NoError(t, store.Tombstone(ctx, "chunk:dead", "chunk:live"))

	_, ok, err := store.Load("chunk:dead")
	require.NoError(t, err)
	require.False(t, ok, "Load must hide tombstoned chunks")

	chunk, ok, err := store.LoadIncludingTombstoned("chunk:dead")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, chunk.Tombstoned)
	require.Equal(t, ChunkID("chunk:live"), chunk.SupersededBy)

	live, ok, err := store.Load("chunk:live")
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, live.Tombstoned)
}

package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// TestGroundingFailsWithoutPartialStateOnStoreFault proves a store failure
// surfaces the typed ErrGroundingFailed and leaves no partial batch behind.
func TestGroundingFailsWithoutPartialStateOnStoreFault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(dir))
	require.NoError(t, err)
	store := &ChunkStore{Graph: engine}

	sentinel, err := store.Save(ctx, KnowledgeChunk{
		ID: "chunk:capture:sentinel", WorkspaceID: "ws-1",
		Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: nowUTC()},
		Body:       ChunkBody{Raw: "sentinel"}, Freshness: FreshnessValid,
	})
	require.NoError(t, err)
	require.NoError(t, engine.Close(ctx))

	service := NewGroundingService(store, nil, nil, nil)
	_, err = service.Ground(ctx, []GroundingItem{
		groundItem(map[string]any{"text": "partial-a"}, EpistemicClaimed, contextdata.OriginLLM),
		groundItem(map[string]any{"text": "partial-b"}, EpistemicClaimed, contextdata.OriginLLM),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrGroundingFailed), "error must satisfy errors.Is(ErrGroundingFailed)")

	reopened, err := graphdb.Open(ctx, graphdb.DefaultOptions(dir))
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close(ctx)) }()
	reopenedStore := &ChunkStore{Graph: reopened}

	all, err := reopenedStore.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 1, "only the sentinel chunk may exist after a failed batch")
	require.Equal(t, sentinel.ID, all[0].ID)

	edges, err := reopenedStore.LoadEdgesFrom(sentinel.ID, EdgeKindGrounds, EdgeKindDerivesFrom)
	require.NoError(t, err)
	require.Empty(t, edges, "no partial edges may survive a failed batch")
}

// TestGroundingHonorsContextCancellation proves an aborted batch writes nothing.
func TestGroundingHonorsContextCancellation(t *testing.T) {
	store := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := NewGroundingService(store, nil, nil, nil)
	_, err := service.Ground(ctx, []GroundingItem{
		groundItem(map[string]any{"text": "cancelled"}, EpistemicClaimed, contextdata.OriginLLM),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrGroundingFailed))

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Empty(t, all)
}

func nowUTC() (t time.Time) { return time.Now().UTC() }

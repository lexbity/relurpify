package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// TestGroundingIdempotentDouble proves a repeated Ground of identical items
// converges on the same chunks with no edge growth.
func TestGroundingIdempotentDouble(t *testing.T) {
	store := newTestStore(t)
	service := newGroundingService(t, store, nil)
	ctx := context.Background()
	items := []GroundingItem{
		groundItem(map[string]any{"text": "identical"}, EpistemicClaimed, contextdata.OriginLLM),
		groundItem(map[string]any{"text": "second"}, EpistemicGiven, contextdata.OriginUser),
	}

	first, err := service.Ground(ctx, items)
	require.NoError(t, err)
	second, err := service.Ground(ctx, items)
	require.NoError(t, err)

	for i := range items {
		require.Equal(t, first.Grounded[i].ChunkID, second.Grounded[i].ChunkID)
		require.True(t, second.Grounded[i].AlreadyExisted, "second ground must report the existing identity")
	}

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 2, "double ground must not duplicate chunks")

	for _, entry := range first.Grounded {
		edges, err := store.LoadEdgesFrom(entry.ChunkID, EdgeKindGrounds, EdgeKindDerivesFrom)
		require.NoError(t, err)
		require.Empty(t, edges, "edge upserts must stay idempotent for unsourced captures")
	}
}

// TestGroundingConcurrentIdentical proves eight concurrent Grounds of the same
// item converge on one chunk without error.
func TestGroundingConcurrentIdentical(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	_, err := store.Save(ctx, KnowledgeChunk{
		ID: "chunk:capture:src-a", WorkspaceID: "ws-1",
		Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: now},
		Body:       ChunkBody{Raw: "source"}, Freshness: FreshnessValid,
	})
	require.NoError(t, err)
	item := groundItem(map[string]any{"text": "concurrent"}, EpistemicClaimed, contextdata.OriginLLM)
	item.SourceChunkIDs = []ChunkID{"chunk:capture:src-a"}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service := newGroundingService(t, store, nil)
			_, err := service.Ground(ctx, []GroundingItem{item})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 2, "one source plus one converged capture")

	grounded := all[0]
	if grounded.ID == "chunk:capture:src-a" {
		grounded = all[1]
	}
	edges, err := store.LoadEdgesFrom(grounded.ID, EdgeKindGrounds)
	require.NoError(t, err)
	require.Len(t, edges, 1, "edge set must stay bounded by the source cap")
}

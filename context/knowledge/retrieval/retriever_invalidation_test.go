package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

// TestSnapshotInvalidatesOnStaledEvent proves a chunk-staled event bumps the
// snapshot generation immediately, without waiting the TTL.
func TestSnapshotInvalidatesOnStaledEvent(t *testing.T) {
	retriever, store, tel, nowp := newSnapshotTestRetriever(t)
	ctx := context.Background()
	saveSnapshotChunk(t, store, "chunk:a", "alpha term", *nowp)

	bus := &knowledge.EventBus{}
	cancel := retriever.SetEventBus(bus)
	defer cancel()

	_, err := retriever.Retrieve(ctx, RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)
	require.Equal(t, 1, tel.count("retrieval_snapshot_built"))

	bus.Publish(knowledge.Event{
		Kind:    knowledge.EventChunkStaled,
		Payload: knowledge.ChunkStaledPayload{ChunkIDs: []string{"chunk:a"}, Reason: "test"},
	})
	require.Eventually(t, func() bool {
		return tel.count("retrieval_snapshot_invalidated") >= 1
	}, time.Second, time.Millisecond, "staled event must invalidate the snapshot")

	_, err = retriever.Retrieve(ctx, RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)
	require.Equal(t, 2, tel.count("retrieval_snapshot_built"), "next retrieve must rebuild")
}

// TestSnapshotExcludesRemovedChunkAfterStaledEvent proves a removed chunk is
// absent from the rebuilt snapshot once the staled event invalidates it.
func TestSnapshotExcludesRemovedChunkAfterStaledEvent(t *testing.T) {
	retriever, store, _, nowp := newSnapshotTestRetriever(t)
	ctx := context.Background()
	saveSnapshotChunk(t, store, "chunk:a", "alpha term", *nowp)

	bus := &knowledge.EventBus{}
	cancel := retriever.SetEventBus(bus)
	defer cancel()

	result, err := retriever.Retrieve(ctx, RetrievalQuery{Text: "alpha"})
	require.NoError(t, err)
	require.True(t, rankedContains(result, "chunk:a"))

	require.NoError(t, store.Tombstone(ctx, "chunk:a", ""))
	bus.Publish(knowledge.Event{
		Kind:    knowledge.EventChunkStaled,
		Payload: knowledge.ChunkStaledPayload{ChunkIDs: []string{"chunk:a"}, Reason: "removed"},
	})

	require.Eventually(t, func() bool {
		result, err := retriever.Retrieve(ctx, RetrievalQuery{Text: "alpha"})
		return err == nil && !rankedContains(result, "chunk:a")
	}, time.Second, time.Millisecond, "removed chunk must not surface after invalidation")
}

func rankedContains(result *RetrievalResult, id knowledge.ChunkID) bool {
	if result == nil {
		return false
	}
	for _, ranked := range result.Ranked {
		if ranked.ChunkID == id {
			return true
		}
	}
	return false
}

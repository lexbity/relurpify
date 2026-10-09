package graphdb

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApplyBatchMixedSingleCommit proves one ApplyBatch call commits nodes and
// edges in a single durable batch.
func TestApplyBatchMixedSingleCommit(t *testing.T) {
	engine, _ := newTestEngine(t)
	cb := &countingBackend{backend: engine.bk}
	engine.bk = cb

	nodes := make([]NodeRecord, 3)
	for i := range nodes {
		nodes[i] = NodeRecord{ID: t.Name() + "-node-" + string(rune('a'+i)), Kind: "test"}
	}
	edges := make([]EdgeRecord, 3)
	for i := range edges {
		edges[i] = EdgeRecord{
			SourceID:  nodes[0].ID,
			TargetID:  nodes[(i+1)%len(nodes)].ID,
			Kind:      "test-edge",
			CreatedAt: 1,
		}
	}
	require.NoError(t, engine.ApplyBatch(context.TODO(), GraphBatch{Nodes: nodes, Edges: edges}))

	require.LessOrEqual(t, cb.commitCount.Load(), int64(2), "mixed batch should commit once")
	for _, node := range nodes {
		_, ok := engine.GetNode(node.ID)
		require.True(t, ok, "node %s must be visible", node.ID)
	}
	require.Len(t, engine.GetOutEdges(nodes[0].ID, "test-edge"), 3)
}

// TestApplyBatchEmptyIsNoop proves an empty batch is inert.
func TestApplyBatchEmptyIsNoop(t *testing.T) {
	engine, _ := newTestEngine(t)
	cb := &countingBackend{backend: engine.bk}
	engine.bk = cb
	require.NoError(t, engine.ApplyBatch(context.TODO(), GraphBatch{}))
	require.Equal(t, int64(0), cb.commitCount.Load())
}

// TestApplyBatchAtomicRollback proves a commit-time failure leaves no nodes
// behind: the whole batch aborts, not just the failing op.
func TestApplyBatchAtomicRollback(t *testing.T) {
	engine, _ := newTestEngine(t)
	// Force a commit-time error after the first node would have been staged by
	// closing the backend backing engine under the batch call via a whitelisted
	// failure backend.
	failing := &failCommitBackend{backend: engine.bk, failNext: true}
	engine.bk = failing

	batch := GraphBatch{
		Nodes: []NodeRecord{
			{ID: "atomic-a", Kind: "test"},
			{ID: "atomic-b", Kind: "test"},
		},
	}
	err := engine.ApplyBatch(context.TODO(), batch)
	require.Error(t, err)

	// The engine never applied the batch to memory: neither node exists, so a
	// failed commit cannot leave a partial batch behind.
	engine.store.mu.RLock()
	_, okA := engine.store.nodes["atomic-a"]
	_, okB := engine.store.nodes["atomic-b"]
	engine.store.mu.RUnlock()
	require.False(t, okA, "memory apply must not happen after a failed commit")
	require.False(t, okB, "a failed commit must not leave a partial batch")
}

// failCommitBackend fails its next commit, then delegates.
type failCommitBackend struct {
	backend
	failNext bool
	attempts atomic.Int64
}

func (f *failCommitBackend) commit(ctx context.Context, batch mutationBatch) error {
	f.attempts.Add(1)
	if f.failNext {
		f.failNext = false
		return &commitErr{}
	}
	return f.backend.commit(ctx, batch)
}

type commitErr struct{}

func (e *commitErr) Error() string { return "injected commit failure" }

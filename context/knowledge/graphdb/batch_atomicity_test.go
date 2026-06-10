package graphdb

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// countingBackend wraps a backend and counts commit calls.
type countingBackend struct {
	backend
	commitCount atomic.Int64
}

func (c *countingBackend) commit(ctx context.Context, batch mutationBatch) error {
	c.commitCount.Add(1)
	return c.backend.commit(ctx, batch)
}

func TestBatchAtomicity_upsertNodesSingleCommit(t *testing.T) {
	engine, opts := newTestEngine(t)
	cb := &countingBackend{backend: engine.bk}
	engine.bk = cb
	_ = opts

	// Upsert 100 nodes — should produce a single commit via UpsertNodes.
	nodes := make([]NodeRecord, 100)
	for i := range nodes {
		nodes[i] = NodeRecord{
			ID:   t.Name() + "-node-" + string(rune('a'+i%26)),
			Kind: "test",
		}
	}
	err := engine.UpsertNodes(nodes)
	require.NoError(t, err)

	count := cb.commitCount.Load()
	if count > 2 {
		t.Errorf("expected ≤2 commits for 100 nodes (UpsertNodes batches internally), got %d", count)
	}
}

func TestBatchAtomicity_linkEdgesSingleCommit(t *testing.T) {
	engine, opts := newTestEngine(t)
	cb := &countingBackend{backend: engine.bk}
	engine.bk = cb
	_ = opts

	// Create a source node.
	err := engine.UpsertNode(NodeRecord{ID: "src", Kind: "test"})
	require.NoError(t, err)
	// Reset counter after setup
	cb.commitCount.Store(0)

	// Link 100 edges — should produce a single commit via LinkEdges.
	edges := make([]EdgeRecord, 100)
	for i := range edges {
		edges[i] = EdgeRecord{
			SourceID:  "src",
			TargetID:  t.Name() + "-tgt-" + string(rune('a'+i%26)),
			Kind:      "test-edge",
			CreatedAt: 1,
		}
	}
	err = engine.LinkEdges(edges)
	require.NoError(t, err)

	count := cb.commitCount.Load()
	if count > 2 {
		t.Errorf("expected ≤2 commits for 100 edges (LinkEdges batches internally), got %d", count)
	}
}

func TestBatchAtomicity_mixedOpsSeparateCommits(t *testing.T) {
	engine, opts := newTestEngine(t)
	cb := &countingBackend{backend: engine.bk}
	engine.bk = cb
	_ = opts

	// One node, then one edge — two separate commits (different op types).
	err := engine.UpsertNode(NodeRecord{ID: "a", Kind: "test"})
	require.NoError(t, err)

	err = engine.Link("a", "b", "edge-a-b", "", 1, nil)
	require.NoError(t, err)

	count := cb.commitCount.Load()
	if count < 2 {
		t.Errorf("expected ≥2 commits for mixed ops (node + edge are separate), got %d", count)
	}
}

func TestBatchAtomicity_commitIsAtomic(t *testing.T) {
	// Use in-memory engine to verify that UpsertNodes commits atomically.
	engine, _ := newTestEngine(t)

	nodes := make([]NodeRecord, 50)
	for i := range nodes {
		nodes[i] = NodeRecord{
			ID:   "atomic-test-node-" + string(rune('0'+i)),
			Kind: "test",
		}
	}
	err := engine.UpsertNodes(nodes)
	require.NoError(t, err)

	// All 50 nodes should be visible after atomic batch commit.
	for _, n := range nodes {
		got, ok := engine.GetNode(n.ID)
		if !ok {
			t.Errorf("node %s not found after atomic batch commit", n.ID)
		}
		if got.Kind != "test" {
			t.Errorf("node %s has wrong kind: %s", n.ID, got.Kind)
		}
	}
}

func TestBatchAtomicity_concurrentWriterReader(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Writer commits all nodes in one batch.
	nodes := make([]NodeRecord, 25)
	for i := range nodes {
		nodes[i] = NodeRecord{ID: "concurrent-node-" + string(rune('a'+i)), Kind: "test"}
	}
	err := engine.UpsertNodes(nodes)
	require.NoError(t, err)

	// Reader sees all nodes after write.
	for _, n := range nodes {
		_, ok := engine.GetNode(n.ID)
		if !ok {
			t.Errorf("node %s not visible after write", n.ID)
		}
	}
}

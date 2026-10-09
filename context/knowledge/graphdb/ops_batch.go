package graphdb

import (
	"context"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// GraphBatch carries nodes and edges that must become durable together.
type GraphBatch struct {
	Nodes []NodeRecord
	Edges []EdgeRecord
}

const (
	// applyBatchRetries bounds retries on Badger write conflicts, which are
	// transient collisions between concurrent writers, not data errors.
	applyBatchRetries = 12
	// applyBatchBackoff is the pause between conflict retries.
	applyBatchBackoff = 4 * time.Millisecond
)

// ApplyBatch commits nodes and edges together in one durable transaction and
// applies them to memory only after the commit succeeds. It is the atomic
// write path for callers that produce both records at once.
func (e *Engine) ApplyBatch(ctx context.Context, batch GraphBatch) error {
	var err error
	for attempt := 0; ; attempt++ {
		err = e.applyBatchOnce(ctx, batch)
		if err == nil {
			return nil
		}
		if !errors.Is(err, badger.ErrConflict) || attempt >= applyBatchRetries {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(applyBatchBackoff):
		}
	}
}

func (e *Engine) applyBatchOnce(ctx context.Context, batch GraphBatch) error {
	if len(batch.Nodes) == 0 && len(batch.Edges) == 0 {
		return nil
	}
	if err := e.checkDirty(); err != nil {
		return err
	}
	now := time.Now().UnixNano()
	nodes := make([]NodeRecord, 0, len(batch.Nodes))
	for _, node := range batch.Nodes {
		if node.CreatedAt == 0 {
			node.CreatedAt = now
		}
		node.UpdatedAt = now
		nodes = append(nodes, node)
	}
	edges := make([]EdgeRecord, 0, len(batch.Edges))
	for _, edge := range batch.Edges {
		if edge.CreatedAt == 0 {
			edge.CreatedAt = now
		}
		edges = append(edges, edge)
	}
	if err := e.persist(ctx, "apply_batch", graphBatchOp{Nodes: nodes, Edges: edges}); err != nil {
		return err
	}
	if err := e.applyHook(); err != nil {
		e.markDirty(err)
		return err
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	for _, node := range nodes {
		e.applyUpsertNode(node)
	}
	for _, edge := range edges {
		e.applyLinkEdge(edge)
	}
	return nil
}

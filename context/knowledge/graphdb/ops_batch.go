package graphdb

import (
	"context"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// NodeGetter reads one node record. The validated apply supplies one that is
// lock-free and only valid inside its validate callback — the apply critical
// section already holds the store lock exclusively.
type NodeGetter func(id string) (NodeRecord, bool)

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

// ErrBatchConflict is returned by ApplyBatchValidated when the validator
// rejects the batch: the committed state the batch's decisions were made
// against changed between decision and apply. It is transient by contract —
// the caller re-decides from fresh state and retries.
var ErrBatchConflict = errors.New("graphdb: batch decision diverged from committed state")

// ApplyBatch commits nodes and edges together in one durable transaction and
// applies them to memory only after the commit succeeds. It is the atomic
// write path for callers that produce both records at once.
func (e *Engine) ApplyBatch(ctx context.Context, batch GraphBatch) error {
	return e.ApplyBatchValidated(ctx, batch, nil)
}

// ApplyBatchValidated commits nodes and edges together and applies them to
// memory only if validate — run INSIDE the apply critical section, against
// the current committed state — accepts (D4: the merge decision is made on
// committed state). Because the validator reads under the same store lock the
// apply mutates under, no writer can interleave between the re-check and the
// apply: an accepted decision is decision-correct at apply time. A
// ErrBatchConflict (or badger.ErrConflict) failure leaves both the durable
// log and the memory mirror untouched by this batch. A nil validator applies
// unconditionally (ApplyBatch semantics).
func (e *Engine) ApplyBatchValidated(ctx context.Context, batch GraphBatch, validate func(get NodeGetter) error) error {
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
	if validate != nil {
		// The getter reads the mirror WITHOUT taking the store lock: the
		// section already holds it exclusively, so the validator's reads and
		// the apply below are one atomic critical section (D4).
		get := func(id string) (NodeRecord, bool) {
			node, ok := e.store.nodes[id]
			if !ok || node == nil || node.DeletedAt != 0 {
				return NodeRecord{}, false
			}
			return cloneNode(node), true
		}
		if err := validate(get); err != nil {
			// The durable record of this batch is already committed; the
			// memory apply is rejected so the mirror never carries a state
			// the validator did not see. The caller re-decides and retries;
			// the retry's re-commit supersedes this batch's log entry.
			return ErrBatchConflict
		}
	}
	for _, node := range nodes {
		e.applyUpsertNode(node)
	}
	for _, edge := range edges {
		e.applyLinkEdge(edge)
	}
	return nil
}

// ApplyBatchChecked is the read-decide-write OCC unit (D4.1): prepare builds
// the batch from current state, validate re-checks the decision inside the
// apply critical section, and any divergence re-runs the WHOLE cycle — fresh
// reads, fresh decision, fresh commit — until it lands on uncontended state
// or the retry budget is exhausted. It is the path concurrent grounding
// merges serialize through.
func (e *Engine) ApplyBatchChecked(ctx context.Context, prepare func() (GraphBatch, error), validate func(get NodeGetter) error) error {
	var err error
	for attempt := 0; ; attempt++ {
		var batch GraphBatch
		batch, err = prepare()
		if err != nil {
			return err
		}
		err = e.ApplyBatchValidated(ctx, batch, validate)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrBatchConflict) && !errors.Is(err, badger.ErrConflict) {
			return err
		}
		if attempt >= applyBatchRetries {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(applyBatchBackoff):
		}
	}
}

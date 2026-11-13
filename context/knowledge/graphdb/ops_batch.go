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
// the current committed state — accepts. The validator runs UNDER the store
// lock BEFORE the durable commit, ensuring that validation failures leave both
// the durable log and memory mirror untouched by this batch (D-07). A
// ErrBatchConflict failure leaves no persistent record. A nil validator
// applies unconditionally (ApplyBatch semantics).
func (e *Engine) ApplyBatchValidated(ctx context.Context, batch GraphBatch, validate func(get NodeGetter) error) error {
	if len(batch.Nodes) == 0 && len(batch.Edges) == 0 {
		return nil
	}
	if err := e.checkDirty(); err != nil {
		return err
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()

	// Prepare immutable payload with timestamps
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

	// Build getter for validation - reads current committed state under the lock.
	// In LRU mode a cache miss MUST NOT mean record absence: fall back to the
	// authoritative durable backend (D-07).
	get := func(id string) (NodeRecord, bool) {
		node, ok := e.store.nodes[id]
		if ok && node != nil && node.DeletedAt == 0 {
			return cloneNode(node), true
		}
		if e.bk != nil {
			rec, err := e.bk.getNodeRecord(id)
			if err == nil && rec != nil && rec.DeletedAt == 0 {
				return *rec, true
			}
		}
		return NodeRecord{}, false
	}

	// Validate BEFORE durable commit (D-07: reject before persist)
	if validate != nil {
		if err := validate(get); err != nil {
			// Validation failed: return conflict WITHOUT persisting
			// The durable log and memory mirror remain untouched.
			return ErrBatchConflict
		}
	}

	// Validation passed - now commit durably
	if err := e.persist(ctx, "apply_batch", graphBatchOp{Nodes: nodes, Edges: edges}); err != nil {
		return err
	}

	// Check for test hook after commit
	if err := e.applyHook(); err != nil {
		e.markDirty(err)
		return err
	}

	// Apply to memory after successful durable commit
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

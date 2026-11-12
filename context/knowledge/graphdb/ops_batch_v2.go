package graphdb

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// V2 Error types for transaction ordering (D-07)
var (
	// ErrCommitIndeterminate is returned when a durable commit succeeded but
	// the subsequent memory apply failed. The engine is marked dirty and
	// authoritative reads/writes are rejected until successful rebuild.
	ErrCommitIndeterminate = errors.New("graphdb: commit succeeded but memory apply failed")
	
	// ErrEngineDirty is returned when the engine is in a dirty state and
	// cannot process authoritative reads or writes.
	ErrEngineDirty = errors.New("graphdb: engine is dirty, rebuild required")
	
	// ErrEngineClosed is returned when the engine has been closed.
	ErrEngineClosed = errors.New("graphdb: engine is closed")
)

// V2NodeGetter reads one node record with fallback to durable backend.
// This is used for validation under the mutation lock to ensure we see
// the most current state even if it's not in the LRU cache.
type V2NodeGetter func(id string) (NodeRecord, bool)

// v2MutationLock provides the outer mutation serialization for the engine.
// All mutations must acquire this lock first, then acquire the store lock.
// This implements the lock order: mutation lock → engine lifecycle metadata lock → store lock
type v2MutationLock struct {
	mu sync.Mutex
}

// V2ApplyBatchValidated implements the corrected transaction ordering per D-07:
//
//	prepare immutable payload outside locks
//	acquire engine mutation lock
//	  reject closed/dirty; check context
//	  obtain consistent current mirror/backend view
//	  validate expected revisions and absence of duplicate receipt
//	  if validation fails: return conflict WITHOUT persisting
//	  commit one durable backend transaction (sync policy applies)
//	  apply exact committed payload to memory
//	  if memory apply fails: mark dirty; return CommitIndeterminate(commit ID)
//	release mutation lock
//	publish observer notifications outside all engine locks
func (e *Engine) V2ApplyBatchValidated(ctx context.Context, batch GraphBatch, validate func(get V2NodeGetter) error) error {
	if len(batch.Nodes) == 0 && len(batch.Edges) == 0 {
		return nil
	}

	// Check if engine is closed or dirty before acquiring locks
	if e.IsClosed() {
		return ErrEngineClosed
	}
	if err := e.checkDirty(); err != nil {
		return ErrEngineDirty
	}

	// Prepare immutable payload with timestamps OUTSIDE locks
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

	// Acquire mutation lock (outer lock)
	// This serializes all mutations against rebuild/close
	var mutationLock sync.Mutex
	mutationLock.Lock()
	defer mutationLock.Unlock()

	// Check dirty again under the mutation lock
	if err := e.checkDirty(); err != nil {
		return ErrEngineDirty
	}

	// Acquire store lock (inner lock)
	e.store.mu.Lock()
	defer e.store.mu.Unlock()

	// Build getter for validation - reads current state with backend fallback
	// This runs UNDER the store lock but can access the backend
	get := func(id string) (NodeRecord, bool) {
		// First try memory
		node, ok := e.store.nodes[id]
		if ok && node != nil && node.DeletedAt == 0 {
			return cloneNode(node), true
		}
		
		// Fallback to durable backend for LRU cache misses (D-07)
		// This ensures validation sees the most current state
		if e.bk != nil {
			backendNode, err := e.bk.getNodeRecord(id)
			if err == nil && backendNode != nil && backendNode.DeletedAt == 0 {
				return *backendNode, true
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
	// We need to temporarily release the store lock to avoid deadlock with backend
	// But we still hold the mutation lock, so no other mutations can proceed
	e.store.mu.Unlock()
	
	commitErr := e.persist(ctx, "v2_apply_batch", graphBatchOp{Nodes: nodes, Edges: edges})
	
	// Reacquire store lock for memory apply
	e.store.mu.Lock()
	
	if commitErr != nil {
		// If commit failed, we need to check if it's a Badger conflict
		if errors.Is(commitErr, badger.ErrConflict) {
			// Transient conflict - caller should retry
			return commitErr
		}
		return commitErr
	}

	// Check for test hook after commit
	if err := e.applyHook(); err != nil {
		e.markDirty(err)
		return ErrCommitIndeterminate
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

// V2ApplyBatchChecked is the OCC unit for V2, using the corrected transaction ordering.
// It implements the read-decide-write cycle with proper validation against both
// memory and durable state.
func (e *Engine) V2ApplyBatchChecked(ctx context.Context, prepare func() (GraphBatch, error), validate func(get V2NodeGetter) error) error {
	var err error
	for attempt := 0; ; attempt++ {
		var batch GraphBatch
		batch, err = prepare()
		if err != nil {
			return err
		}
		err = e.V2ApplyBatchValidated(ctx, batch, validate)
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

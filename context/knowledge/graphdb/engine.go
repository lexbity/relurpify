package graphdb

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Engine is the durable embedded graph database.
type Engine struct {
	opts     Options
	store    *adjacencyStore
	bk       backend
	mu       sync.Mutex
	dirty    atomic.Int64
	dirtyErr error // non-nil when memory apply fails after commit; mu protects
	lastSave atomic.Int64
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// testHookApplyFailure, when non-nil, causes the next memory apply to
	// return this error. Used only in tests.
	testHookApplyFailure error
}

// Open initializes a Badger-backed engine from durable storage.
func Open(ctx context.Context, opts Options) (*Engine, error) {
	start := time.Now()
	engine := &Engine{
		opts:   opts,
		store:  newAdjacencyStore(),
		stopCh: make(chan struct{}),
	}
	engine.store.lruMaxCapacity = opts.LRUCapacity
	engine.emitEvent(Event{Kind: EventOpenStart})

	if opts.DataDir == "" {
		return nil, errors.New("graphdb: data dir is required")
	}

	badgerDir := opts.BadgerDir
	if badgerDir == "" {
		badgerDir = opts.DataDir
	}

	engine.lastSave.Store(time.Now().UnixNano())

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	if err != nil {
		return nil, err
	}
	if err := bb.load(ctx, engine.store); err != nil {
		bb.close()
		return nil, err
	}
	engine.bk = bb

	engine.wg.Add(1)
	go engine.background(ctx)

	engine.emitEvent(Event{
		Kind:     EventOpenComplete,
		Duration: time.Since(start),
	})
	return engine, nil
}

// Close stops maintenance and closes the durable store.
func (e *Engine) Close(ctx context.Context) error {
	var err error
	e.stopOnce.Do(func() {
		close(e.stopCh)
		e.wg.Wait()
		if e.opts.SnapshotOnClose && e.dirty.Load() > 0 {
			err = e.Snapshot(ctx)
		} else {
			err = e.Flush()
		}
		if e.bk != nil {
			if closeErr := e.bk.close(); err == nil {
				err = closeErr
			}
		}
	})
	return err
}

// Flush syncs the durable store.
func (e *Engine) Flush() error {
	if e == nil || e.bk == nil {
		return nil
	}
	return e.bk.flush()
}

// Snapshot writes a full snapshot and rewrites the incremental log.
func (e *Engine) Snapshot(ctx context.Context) error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	state := e.snapshotState()
	if err := e.bk.snapshot(ctx, state); err != nil {
		return err
	}
	e.dirty.Store(0)
	e.lastSave.Store(time.Now().UnixNano())
	return nil
}

func (e *Engine) background(ctx context.Context) {
	defer e.wg.Done()
	interval := e.opts.MaintenanceInterval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.maybeSnapshot(ctx)
		}
	}
}

func (e *Engine) maybeSnapshot(ctx context.Context) {
	if e.opts.AutoSaveThreshold <= 0 || e.opts.AutoSaveInterval <= 0 {
		return
	}
	if e.dirty.Load() < e.opts.AutoSaveThreshold {
		return
	}
	last := time.Unix(0, e.lastSave.Load())
	if time.Since(last) < e.opts.AutoSaveInterval {
		return
	}
	_ = e.Snapshot(ctx)
}

// checkDirty returns the stored dirty error when a previous memory apply
// failed after a successful backend commit. Mutation calls MUST check
// this before proceeding.
func (e *Engine) checkDirty() error {
	if e.dirtyErr != nil {
		return e.dirtyErr
	}
	return nil
}

// markDirty stores a non-nil error, preventing further mutations until
// Rebuild clears it.
func (e *Engine) markDirty(err error) {
	e.dirtyErr = err
	e.emitEvent(Event{
		Kind:       EventMemoryApplyFail,
		ErrorClass: err.Error(),
	})
}

// emitEvent sends an event to the configured observer, if any.
func (e *Engine) emitEvent(ev Event) {
	if e != nil && e.opts.Observer != nil {
		e.opts.Observer.Observe(ev)
	}
}

// applyHook returns the test-only apply failure error if set, or nil.
// It is checked after a successful backend commit but before applying
// mutations to the in-memory store.
func (e *Engine) applyHook() error {
	return e.testHookApplyFailure
}

// Rebuild reloads the in-memory adjacency store from the durable backend
// and clears any dirty error state. The engine is usable for both reads
// and writes after a successful rebuild.
func (e *Engine) Rebuild(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	newStore := newAdjacencyStore()
	// AOF replay callbacks reference e.store, so we temporarily swap.
	oldStore := e.store
	e.store = newStore
	if err := e.bk.load(ctx, newStore); err != nil {
		e.store = oldStore
		return err
	}
	e.dirty.Store(0)
	e.dirtyErr = nil
	return nil
}

func (e *Engine) snapshotState() snapshotState {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()

	state := snapshotState{
		Nodes:   make([]NodeRecord, 0, len(e.store.nodes)),
		Forward: make([]EdgeRecord, 0),
	}
	for _, node := range e.store.nodes {
		state.Nodes = append(state.Nodes, cloneNode(node))
	}
	for _, edges := range e.store.forward {
		for _, edge := range edges {
			state.Forward = append(state.Forward, cloneEdge(edge))
		}
	}
	return state
}

func (e *Engine) persist(ctx context.Context, kind string, payload any) error {
	if e == nil || e.bk == nil {
		return nil
	}
	start := time.Now()
	batch := singleOpBatch(kind, payload)
	if err := e.bk.commit(ctx, batch); err != nil {
		e.emitEvent(Event{
			Kind:       EventBackendCommitFail,
			BatchSize:  1,
			Duration:   time.Since(start),
			ErrorClass: err.Error(),
		})
		return err
	}
	e.emitEvent(Event{
		Kind:      EventBackendCommit,
		BatchSize: 1,
		Duration:  time.Since(start),
	})
	e.dirty.Add(1)
	// AOFRewriteThreshold check removed with AOF backend retirement.
	return nil
}

// RecordMutationResult stores a projection or mutation audit result durably.
func (e *Engine) RecordMutationResult(ctx context.Context, result MutationResult) error {
	if e == nil {
		return nil
	}
	if err := e.checkDirty(); err != nil {
		return err
	}
	if result.AppliedAt.IsZero() {
		result.AppliedAt = time.Now().UTC()
	}
	result.Normalize(result.TaskID, result.SessionID)
	if err := e.persist(ctx, "record_mutation_result", mutationResultOp{Result: result}); err != nil {
		return err
	}
	if err := e.applyHook(); err != nil {
		e.markDirty(err)
		return err
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	e.applyMutationResult(result)
	return nil
}

// MutationResult returns a stored mutation result by stable ID from durable
// storage. History is disk-only per FR-11.
func (e *Engine) MutationResult(stableID string) (MutationResult, bool) {
	if e == nil || e.bk == nil {
		return MutationResult{}, false
	}
	result, err := e.bk.getMutationResult(stableID)
	if err != nil || result == nil {
		return MutationResult{}, false
	}
	return *result, true
}

// MutationResults returns all stored mutation results in stable-ID order
// from durable storage.
func (e *Engine) MutationResults() []MutationResult {
	if e == nil || e.bk == nil {
		return nil
	}
	results, err := e.bk.listMutationResults()
	if err != nil || len(results) == 0 {
		return nil
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].StableID < results[j].StableID
	})
	return results
}

func (e *Engine) applyMutationResult(result MutationResult) {
	// Mutation results are persisted to Badger during commit.
	// No RAM storage needed per FR-11. The Badger commit in
	// recordMutationResult already writes the result to disk.
}

func (e *Engine) applyBinaryOp(op binaryOp) error {
	switch op.code {
	case opCodeUpsertNode:
		dec := binaryDecoderFromBytes(op.data)
		node, err := dec.readNodeRecord()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		e.applyUpsertNode(node)
	case opCodeUpsertNodes:
		dec := binaryDecoderFromBytes(op.data)
		nodes, err := dec.readNodeRecords()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		for _, node := range nodes {
			e.applyUpsertNode(node)
		}
	case opCodeDeleteNode:
		dec := binaryDecoderFromBytes(op.data)
		id, err := dec.readString()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		e.applyDeleteNode(id, 0)
	case opCodeDeleteNodes:
		dec := binaryDecoderFromBytes(op.data)
		ids, err := dec.readStrings()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		for _, id := range ids {
			e.applyDeleteNode(id, 0)
		}
	case opCodeLinkEdge:
		dec := binaryDecoderFromBytes(op.data)
		edge, err := dec.readEdgeRecord()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		e.applyLinkEdge(edge)
	case opCodeLinkEdges:
		dec := binaryDecoderFromBytes(op.data)
		edges, err := dec.readEdgeRecords()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		for _, edge := range edges {
			e.applyLinkEdge(edge)
		}
	case opCodeUnlinkEdge:
		dec := binaryDecoderFromBytes(op.data)
		sourceID, err := dec.readString()
		if err != nil {
			return err
		}
		targetID, err := dec.readString()
		if err != nil {
			return err
		}
		kind, err := dec.readString()
		if err != nil {
			return err
		}
		hard, err := dec.readBool()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		e.applyUnlink(sourceID, targetID, EdgeKind(kind), hard, 0)
	case opCodeAnnotateNode:
		dec := binaryDecoderFromBytes(op.data)
		id, err := dec.readString()
		if err != nil {
			return err
		}
		raw, err := dec.readBytes()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		var props map[string]any
		if err := json.Unmarshal(raw, &props); err != nil {
			return err
		}
		e.store.mu.Lock()
		if err := e.annotateNodeLocked(id, props, 0); err != nil {
			e.store.mu.Unlock()
			return err
		}
		e.store.mu.Unlock()
	case opCodeAnnotateEdge:
		dec := binaryDecoderFromBytes(op.data)
		sourceID, err := dec.readString()
		if err != nil {
			return err
		}
		targetID, err := dec.readString()
		if err != nil {
			return err
		}
		kind, err := dec.readString()
		if err != nil {
			return err
		}
		raw, err := dec.readBytes()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		var props map[string]any
		if err := json.Unmarshal(raw, &props); err != nil {
			return err
		}
		e.store.mu.Lock()
		if err := e.annotateEdgeLocked(sourceID, targetID, EdgeKind(kind), props); err != nil {
			e.store.mu.Unlock()
			return err
		}
		e.store.mu.Unlock()
	case opCodeRecordMutationResult:
		dec := binaryDecoderFromBytes(op.data)
		raw, err := dec.readBytes()
		if err != nil {
			return err
		}
		if err := dec.finish(); err != nil {
			return err
		}
		var result MutationResult
		if err := json.Unmarshal(raw, &result); err != nil {
			return err
		}
		e.applyMutationResult(result)
	default:
		return errors.New("graphdb: unknown binary op code")
	}
	return nil
}

func (e *Engine) applyLegacyJSONOp(payload []byte) error {
	var op struct {
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &op); err != nil {
		return err
	}
	switch op.Kind {
	case "upsert_node":
		var payload nodeOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		e.applyUpsertNode(payload.Node)
	case "upsert_nodes":
		var payload nodeBatchOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		for _, node := range payload.Nodes {
			e.applyUpsertNode(node)
		}
	case "delete_node":
		var payload deleteNodeOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		e.applyDeleteNode(payload.ID, 0)
	case "delete_nodes":
		var payload deleteNodesOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		for _, id := range payload.IDs {
			e.applyDeleteNode(id, 0)
		}
	case "link_edge":
		var payload edgeOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		e.applyLinkEdge(payload.Edge)
	case "link_edges":
		var payload edgeBatchOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		for _, edge := range payload.Edges {
			e.applyLinkEdge(edge)
		}
	case "unlink_edge":
		var payload unlinkOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		e.applyUnlink(payload.SourceID, payload.TargetID, payload.Kind, payload.Hard, 0)
	case "annotate_node":
		var payload annotateNodeOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		e.store.mu.Lock()
		if err := e.annotateNodeLocked(payload.ID, payload.Props, 0); err != nil {
			e.store.mu.Unlock()
			return err
		}
		e.store.mu.Unlock()
	case "annotate_edge":
		var payload annotateEdgeOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		e.store.mu.Lock()
		if err := e.annotateEdgeLocked(payload.SourceID, payload.TargetID, payload.Kind, payload.Props); err != nil {
			e.store.mu.Unlock()
			return err
		}
		e.store.mu.Unlock()
	case "record_mutation_result":
		var payload mutationResultOp
		if err := json.Unmarshal(op.Data, &payload); err != nil {
			return err
		}
		e.applyMutationResult(payload.Result)
	}
	return nil
}

// IsClosed returns true if the engine has been closed.
func (e *Engine) IsClosed() bool {
	if e == nil || e.stopCh == nil {
		return true
	}
	select {
	case <-e.stopCh:
		return true
	default:
		return false
	}
}

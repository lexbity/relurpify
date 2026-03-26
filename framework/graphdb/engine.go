package graphdb

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Engine is the durable embedded graph database.
type Engine struct {
	opts         Options
	store        *adjacencyStore
	aof          *aofWriter
	mu           sync.Mutex
	dirty        atomic.Int64
	lastSave     atomic.Int64
	stopOnce     sync.Once
	stopCh       chan struct{}
	wg           sync.WaitGroup
	aofPath      string
	snapshotPath string
}

// Open initializes an engine from snapshot plus AOF replay.
func Open(opts Options) (*Engine, error) {
	if opts.DataDir == "" {
		return nil, errors.New("graphdb: data dir is required")
	}
	if opts.AOFFileName == "" || opts.SnapshotFileName == "" {
		return nil, errors.New("graphdb: AOF and snapshot file names are required")
	}
	if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
		return nil, err
	}

	engine := &Engine{
		opts:         opts,
		store:        newAdjacencyStore(),
		stopCh:       make(chan struct{}),
		aofPath:      filepath.Join(opts.DataDir, opts.AOFFileName),
		snapshotPath: filepath.Join(opts.DataDir, opts.SnapshotFileName),
	}
	engine.lastSave.Store(time.Now().UnixNano())

	if err := engine.loadSnapshot(); err != nil {
		return nil, err
	}
	if err := replayAOF(engine.aofPath, engine.applyBinaryOp, engine.applyLegacyJSONOp); err != nil {
		return nil, err
	}
	aof, err := openAOF(engine.aofPath, opts)
	if err != nil {
		return nil, err
	}
	engine.aof = aof

	engine.wg.Add(1)
	go engine.background()
	return engine, nil
}

// Close stops maintenance and closes the AOF.
func (e *Engine) Close() error {
	var err error
	e.stopOnce.Do(func() {
		close(e.stopCh)
		e.wg.Wait()
		if e.opts.SnapshotOnClose && e.dirty.Load() > 0 {
			err = e.Snapshot()
		} else {
			err = e.Flush()
		}
		if e.aof != nil {
			if closeErr := e.aof.close(); err == nil {
				err = closeErr
			}
		}
	})
	return err
}

// Flush syncs the append-only file.
func (e *Engine) Flush() error {
	if e == nil || e.aof == nil {
		return nil
	}
	e.aof.mu.Lock()
	defer e.aof.mu.Unlock()
	return e.aof.syncLocked(true)
}

// Snapshot writes a full snapshot and rewrites the AOF.
func (e *Engine) Snapshot() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	state := e.snapshotState()
	if err := writeSnapshot(e.snapshotPath, state); err != nil {
		return err
	}
	if err := e.aof.truncate(); err != nil {
		return err
	}
	e.dirty.Store(0)
	e.lastSave.Store(time.Now().UnixNano())
	return nil
}

func (e *Engine) background() {
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
			e.maybeSnapshot()
		}
	}
}

func (e *Engine) maybeSnapshot() {
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
	_ = e.Snapshot()
}

func (e *Engine) loadSnapshot() error {
	state, err := readSnapshot(e.snapshotPath)
	if err != nil {
		return err
	}
	for _, node := range state.Nodes {
		n := node
		e.store.nodes[node.ID] = &n
		e.store.addNodeSourceIndex(node)
	}
	for _, edge := range state.Forward {
		e.store.forward[edge.SourceID] = append(e.store.forward[edge.SourceID], cloneEdge(edge))
		e.store.reverse[edge.TargetID] = append(e.store.reverse[edge.TargetID], cloneEdge(edge))
	}
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

func (e *Engine) persist(kind string, payload any) error {
	if e == nil || e.aof == nil {
		return nil
	}
	op, err := encodeBinaryOp(kind, payload)
	if err != nil {
		return err
	}
	if err := e.aof.appendOp(op); err != nil {
		return err
	}
	e.dirty.Add(1)
	if e.opts.AOFRewriteThresholdBytes > 0 {
		if size, err := e.aof.size(); err == nil && size >= e.opts.AOFRewriteThresholdBytes {
			_ = e.Snapshot()
		}
	}
	return nil
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
	}
	return nil
}

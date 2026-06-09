package graphdb

import (
	"context"
	"io"
	"path/filepath"
)

// aofBackend implements the backend interface using the classic AOF + snapshot
// file format. It is the transitional backend; the durable target is Badger.
type aofBackend struct {
	opts         Options
	aof          *aofWriter
	aofPath      string
	snapshotPath string

	// replayCallbacks are set by Engine before load().
	applyBinary func(binaryOp) error
	applyLegacy func([]byte) error
}

func newAOFBackend(opts Options) *aofBackend {
	return &aofBackend{
		opts:         opts,
		aofPath:      filepath.Join(opts.DataDir, opts.AOFFileName),
		snapshotPath: filepath.Join(opts.DataDir, opts.SnapshotFileName),
	}
}

func (b *aofBackend) load(_ context.Context, store *adjacencyStore) error {
	// 1. Load snapshot into store.
	state, err := readSnapshot(b.snapshotPath)
	if err != nil {
		return err
	}
	for _, node := range state.Nodes {
		n := node
		store.nodes[node.ID] = &n
		store.addNodeSourceIndex(node)
		store.addNodeLabels(node)
	}
	for key, history := range state.NodeHistory {
		store.nodeHistory[key] = cloneNodeHistory(history)
	}
	for _, edge := range state.Forward {
		store.forward[edge.SourceID] = append(store.forward[edge.SourceID], cloneEdge(edge))
		store.reverse[edge.TargetID] = append(store.reverse[edge.TargetID], cloneEdge(edge))
	}
	for key, history := range state.EdgeHistory {
		store.edgeHistory[key] = cloneEdgeHistory(history)
	}
	for key, result := range state.MutationResults {
		store.mutationResults[key] = cloneMutationResult(result)
	}

	// 2. Replay AOF over the loaded snapshot.
	if err := replayAOF(b.aofPath, b.applyBinary, b.applyLegacy); err != nil {
		return err
	}

	// 3. Open AOF for writing.
	aof, err := openAOF(b.aofPath, b.opts)
	if err != nil {
		return err
	}
	b.aof = aof
	return nil
}

func (b *aofBackend) commit(_ context.Context, batch mutationBatch) error {
	op, err := encodeBinaryOp(batch.opName, batch.op)
	if err != nil {
		return err
	}
	return b.aof.appendOp(op)
}

func (b *aofBackend) snapshot(_ context.Context, state snapshotState) error {
	if err := writeSnapshot(b.snapshotPath, state); err != nil {
		return err
	}
	return b.aof.truncate()
}

func (b *aofBackend) flush() error {
	if b == nil || b.aof == nil {
		return nil
	}
	b.aof.mu.Lock()
	defer b.aof.mu.Unlock()
	return b.aof.syncLocked(true)
}

func (b *aofBackend) close() error {
	if b.aof != nil {
		return b.aof.close()
	}
	return nil
}

func (b *aofBackend) maintenance(_ context.Context, _ MaintenanceRequest) (MaintenanceResult, error) {
	return MaintenanceResult{
		Backend: "aof",
	}, nil
}

func (b *aofBackend) backup(_ context.Context, _ io.Writer) error {
	return ErrBackupUnsupported
}

// aofSize returns the current AOF file size. Used by the engine to decide
// whether to rewrite the AOF. Returns 0 if the AOF is not open.
func (b *aofBackend) aofSize() (int64, error) {
	if b.aof == nil {
		return 0, nil
	}
	return b.aof.size()
}

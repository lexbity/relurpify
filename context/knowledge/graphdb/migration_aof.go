package graphdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// ────────────────────────────────────────────────────────────────────
// Migration state
// ────────────────────────────────────────────────────────────────────

const (
	migrationName             = "aof_to_badger"
	migrationStatusInProgress = "in_progress"
	migrationStatusCompleted  = "completed"
	migrationChunkSize        = 500
)

type migrationState struct {
	Status      string    `json:"status"`
	SourceAOF   string    `json:"source_aof,omitempty"`
	SourceSnap  string    `json:"source_snap,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

func readMigrationState(txn *badger.Txn) (*migrationState, error) {
	key := keyMigration(migrationName)
	item, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return &migrationState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var st migrationState
	if err := item.Value(func(val []byte) error {
		return json.Unmarshal(val, &st)
	}); err != nil {
		return nil, err
	}
	return &st, nil
}

// loadAOFStore reads AOF and snapshot files into a new adjacency store.
// It returns a reference to the store and the engine used for replay
// (caller must close the engine).
func loadAOFStore(aofPath, snapPath string) (*adjacencyStore, error) {
	store := newAdjacencyStore()

	// 1. Load snapshot if present.
	state, err := readSnapshot(snapPath)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
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
	eng := &Engine{store: store}
	if err := replayAOF(aofPath, eng.applyBinaryOp, eng.applyLegacyJSONOp); err != nil {
		return nil, fmt.Errorf("replay AOF: %w", err)
	}

	return store, nil
}

func writeMigrationState(txn *badger.Txn, st migrationState) error {
	val, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return txn.Set(keyMigration(migrationName), val)
}

// ────────────────────────────────────────────────────────────────────
// MigrateAOFToBadger reads all data from an AOF‑backed graph store and
// writes it into a Badger‑backed store idempotently.  Already‑completed
// migrations are skipped; interrupted migrations resume by re‑writing.
func MigrateAOFToBadger(ctx context.Context, aofDir string, badgerDir string) error {
	aofPath := filepath.Join(aofDir, "graphdb.aof")
	snapPath := filepath.Join(aofDir, "graphdb.snapshot")

	// 1. Load source data from AOF/snapshot files into an adjacency store.
	src, err := loadAOFStore(aofPath, snapPath)
	if err != nil {
		return fmt.Errorf("graphdb migration: load source: %w", err)
	}

	// 2. Open target Badger store.
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	if err != nil {
		return fmt.Errorf("graphdb migration: open target: %w", err)
	}
	defer bb.close()

	// 3. Check migration state.
	var alreadyDone bool
	if err := bb.db.Update(func(txn *badger.Txn) error {
		st, err := readMigrationState(txn)
		if err != nil {
			return err
		}
		if st.Status == migrationStatusCompleted {
			alreadyDone = true
		}
		return nil
	}); err != nil {
		return err
	}
	if alreadyDone {
		return nil
	}

	// 4. Mark in‑progress.
	if err := bb.db.Update(func(txn *badger.Txn) error {
		return writeMigrationState(txn, migrationState{
			Status:     migrationStatusInProgress,
			SourceAOF:  aofPath,
			SourceSnap: snapPath,
		})
	}); err != nil {
		return err
	}
	_ = snapPath // used via source info above

	// 5. Collect all data from the source adjacency store.
	src.mu.RLock()

	nodeList := make([]NodeRecord, 0, len(src.nodes))
	for _, node := range src.nodes {
		nodeList = append(nodeList, *node)
	}

	edgeList := make([]EdgeRecord, 0)
	for _, edges := range src.forward {
		for _, edge := range edges {
			edgeList = append(edgeList, edge)
		}
	}

	mutResults := make([]MutationResult, 0, len(src.mutationResults))
	for _, result := range src.mutationResults {
		mutResults = append(mutResults, result)
	}
	src.mu.RUnlock()

	// 6. Write nodes in chunks.
	for i := 0; i < len(nodeList); i += migrationChunkSize {
		end := i + migrationChunkSize
		if end > len(nodeList) {
			end = len(nodeList)
		}
		batch := nodeList[i:end]
		if len(batch) == 1 {
			if err := bb.commit(ctx, mutationBatch{
				opName: "upsert_node",
				op:     nodeOp{Node: batch[0]},
			}); err != nil {
				return fmt.Errorf("graphdb migration: write node: %w", err)
			}
		} else {
			if err := bb.commit(ctx, mutationBatch{
				opName: "upsert_nodes",
				op:     nodeBatchOp{Nodes: batch},
			}); err != nil {
				return fmt.Errorf("graphdb migration: write nodes: %w", err)
			}
		}
	}

	// 7. Write edges in chunks.
	for i := 0; i < len(edgeList); i += migrationChunkSize {
		end := i + migrationChunkSize
		if end > len(edgeList) {
			end = len(edgeList)
		}
		batch := edgeList[i:end]
		if len(batch) == 1 {
			if err := bb.commit(ctx, mutationBatch{
				opName: "link_edge",
				op:     edgeOp{Edge: batch[0]},
			}); err != nil {
				return fmt.Errorf("graphdb migration: write edge: %w", err)
			}
		} else {
			if err := bb.commit(ctx, mutationBatch{
				opName: "link_edges",
				op:     edgeBatchOp{Edges: batch},
			}); err != nil {
				return fmt.Errorf("graphdb migration: write edges: %w", err)
			}
		}
	}

	// 8. Write mutation results individually.
	for _, result := range mutResults {
		if err := bb.commit(ctx, mutationBatch{
			opName: "record_mutation_result",
			op:     mutationResultOp{Result: result},
		}); err != nil {
			return fmt.Errorf("graphdb migration: write mutation result: %w", err)
		}
	}

	// 9. Rebuild indexes from canonical records.
	if err := bb.rebuildIndexes(); err != nil {
		return fmt.Errorf("graphdb migration: rebuild indexes: %w", err)
	}

	// 10. Mark complete.
	if err := bb.db.Update(func(txn *badger.Txn) error {
		return writeMigrationState(txn, migrationState{
			Status:      migrationStatusCompleted,
			SourceAOF:   aofPath,
			SourceSnap:  snapPath,
			CompletedAt: time.Now().UTC(),
		})
	}); err != nil {
		return fmt.Errorf("graphdb migration: mark complete: %w", err)
	}

	return nil
}

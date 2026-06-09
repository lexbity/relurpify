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
	migrationName              = "aof_to_badger"
	migrationStatusInProgress  = "in_progress"
	migrationStatusCompleted   = "completed"
	migrationChunkSize         = 500
)

type migrationState struct {
	Status       string    `json:"status"`
	SourceAOF    string    `json:"source_aof,omitempty"`
	SourceSnap   string    `json:"source_snap,omitempty"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
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
	// 1. Load source data from AOF/snapshot files.
	aofOpts := DefaultOptions(aofDir)
	src, err := Open(aofOpts)
	if err != nil {
		return fmt.Errorf("graphdb migration: open source: %w", err)
	}
	defer src.Close()

	aofPath := filepath.Join(aofOpts.DataDir, aofOpts.AOFFileName)
	snapPath := filepath.Join(aofOpts.DataDir, aofOpts.SnapshotFileName)

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

	// 5. Collect all data from the source adjacency store.
	src.store.mu.RLock()

	nodeList := make([]NodeRecord, 0, len(src.store.nodes))
	for _, node := range src.store.nodes {
		nodeList = append(nodeList, *node)
	}

	edgeList := make([]EdgeRecord, 0)
	for _, edges := range src.store.forward {
		for _, edge := range edges {
			edgeList = append(edgeList, edge)
		}
	}

	mutResults := make([]MutationResult, 0, len(src.store.mutationResults))
	for _, result := range src.store.mutationResults {
		mutResults = append(mutResults, result)
	}
	src.store.mu.RUnlock()

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

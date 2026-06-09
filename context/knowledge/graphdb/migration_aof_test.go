package graphdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

// ────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────

func sourceWithSnapshot(t *testing.T, state snapshotState) string {
	t.Helper()
	dir := t.TempDir()
	err := writeSnapshot(filepath.Join(dir, "graphdb.snapshot"), state)
	require.NoError(t, err)
	return dir
}

func migratedNodeCount(t *testing.T, badgerDir string) int {
	t.Helper()
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	var count int
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := keyPrefix(famNode)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	}))
	return count
}

func migratedEdgeCount(t *testing.T, badgerDir string) int {
	t.Helper()
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	var count int
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := keyPrefix(famEdgeOut)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	}))
	return count
}

func isMigrationCompleted(t *testing.T, badgerDir string) bool {
	t.Helper()
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	var completed bool
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		st, err := readMigrationState(txn)
		if err != nil {
			return err
		}
		completed = st.Status == migrationStatusCompleted
		return nil
	}))
	return completed
}

// ────────────────────────────────────────────────────────────────────
// Tests
// ────────────────────────────────────────────────────────────────────

func TestMigrateAOFToBadger_EmptySource(t *testing.T) {
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), t.TempDir(), badgerDir))
	require.Zero(t, migratedNodeCount(t, badgerDir))
	require.Zero(t, migratedEdgeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))
}

func TestMigrateAOFToBadger_NodesOnly(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{
			{ID: "n1", Kind: "function", SourceID: "a.go", Labels: []string{"tag:x"}},
			{ID: "n2", Kind: "method", SourceID: "a.go", Labels: []string{"tag:y"}},
			{ID: "n3", Kind: "function", SourceID: "b.go"},
		},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 3, migratedNodeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))
	require.Contains(t, store.nodes, "n1")
	require.Contains(t, store.nodes, "n2")
	require.Contains(t, store.nodes, "n3")
	require.Equal(t, NodeKind("function"), store.nodes["n1"].Kind)
}

func TestMigrateAOFToBadger_WithEdges(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{
			{ID: "a", Kind: "function"},
			{ID: "b", Kind: "function"},
			{ID: "c", Kind: "function"},
		},
		Forward: []EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1},
			{SourceID: "b", TargetID: "c", Kind: "calls", Weight: 1},
		},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 3, migratedNodeCount(t, badgerDir))
	require.Equal(t, 2, migratedEdgeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))
	require.Len(t, store.forward["a"], 1)
	require.Len(t, store.forward["b"], 1)
	require.Len(t, store.reverse["c"], 1)
}

func TestMigrateAOFToBadger_WithMutationResults(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		MutationResults: map[string]MutationResult{
			"mig-mut-1": {
				StableID:  "mig-mut-1",
				Scope:     MutationScopeNode,
				Status:    MutationStatusCreated,
				TaskID:    "task-1",
				SessionID: "session-1",
			},
		},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))
	require.Contains(t, store.mutationResults, "mig-mut-1")
}

func TestMigrateAOFToBadger_WithLabelsAndSource(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{
			{ID: "n1", Kind: "file", SourceID: "work", Labels: []string{"tag:go", "scope:test"}},
		},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))
	require.Contains(t, store.nodes, "n1")
	require.Equal(t, []string{"tag:go", "scope:test"}, store.nodes["n1"].Labels)
	require.Contains(t, store.bySource, "work")
}

func TestMigrateAOFToBadger_SkipIfAlreadyCompleted(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{{ID: "n1", Kind: "function"}},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 1, migratedNodeCount(t, badgerDir))

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 1, migratedNodeCount(t, badgerDir))
}

func TestMigrateAOFToBadger_ResumeAfterInterruption(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{
			{ID: "n1", Kind: "function"},
			{ID: "n2", Kind: "function"},
		},
	})
	badgerDir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	require.NoError(t, bb.db.Update(func(txn *badger.Txn) error {
		return writeMigrationState(txn, migrationState{
			Status: migrationStatusInProgress,
		})
	}))
	bb.close()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 2, migratedNodeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))
}

func TestMigrateAOFToBadger_ManyNodes(t *testing.T) {
	const count = 1500
	nodes := make([]NodeRecord, count)
	for i := range count {
		nodes[i] = NodeRecord{
			ID:   fmt.Sprintf("n-%06d", i),
			Kind: "function",
		}
	}
	aofDir := sourceWithSnapshot(t, snapshotState{Nodes: nodes})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, count, migratedNodeCount(t, badgerDir))
}

func TestMigrateAOFToBadger_LoadAndQuery(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{
			{ID: "a", Kind: "function"},
			{ID: "b", Kind: "function"},
			{ID: "c", Kind: "function"},
		},
		Forward: []EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1},
			{SourceID: "b", TargetID: "c", Kind: "calls", Weight: 1},
		},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))
	require.Len(t, store.nodes, 3)
	require.Len(t, store.forward["a"], 1)
	require.Len(t, store.forward["b"], 1)
	require.Len(t, store.reverse["c"], 1)

	result := (&Engine{store: store}).ImpactSet([]string{"a"}, []EdgeKind{"calls"}, 5)
	require.ElementsMatch(t, []string{"b", "c"}, result.Affected)
}

func TestMigrateAOFToBadger_MigrationStateWritten(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{{ID: "n1", Kind: "function"}},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	var st migrationState
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		st2, err := readMigrationState(txn)
		st = *st2
		return err
	}))
	require.Equal(t, migrationStatusCompleted, st.Status)
	require.NotEmpty(t, st.CompletedAt)
	require.NotEmpty(t, st.SourceAOF)
}

func TestMigrateAOFToBadger_InvalidSourceDir(t *testing.T) {
	badgerDir := t.TempDir()
	// A missing snapshot file is fine (empty state). An invalid one should fail.
	aofDir := t.TempDir()
	err := os.WriteFile(filepath.Join(aofDir, "graphdb.snapshot"), []byte(`{invalid json}`), 0o644)
	require.NoError(t, err)
	err = MigrateAOFToBadger(context.Background(), aofDir, badgerDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read snapshot")
}

func TestMigrateAOFToBadger_NonEmptyTargetOverwrites(t *testing.T) {
	aofDir := sourceWithSnapshot(t, snapshotState{
		Nodes: []NodeRecord{{ID: "n1", Kind: "function"}},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 1, migratedNodeCount(t, badgerDir))
	_, err := os.Stat(filepath.Join(badgerDir, "graphdb.aof"))
	require.True(t, os.IsNotExist(err))
}

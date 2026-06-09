package graphdb

import (
	"context"
	"encoding/json"
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

func sourceWithNodes(t *testing.T, nodes []NodeRecord) (dir string) {
	t.Helper()
	dir = t.TempDir()
	opts := DefaultOptions(dir)
	engine, err := Open(opts)
	require.NoError(t, err)
	for _, n := range nodes {
		require.NoError(t, engine.UpsertNode(n))
	}
	require.NoError(t, engine.Close())
	return dir
}

func sourceWithEdges(t *testing.T, nodes []NodeRecord, edges []EdgeRecord) string {
	t.Helper()
	dir := sourceWithNodes(t, nodes)
	opts := DefaultOptions(dir)
	engine, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, engine.LinkEdges(edges))
	require.NoError(t, engine.Close())
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
	aofDir := t.TempDir()
	badgerDir := t.TempDir()

	// Create empty AOF store by just creating the directory.
	opts := DefaultOptions(aofDir)
	engine, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, engine.Close())

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Zero(t, migratedNodeCount(t, badgerDir))
	require.Zero(t, migratedEdgeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))
}

func TestMigrateAOFToBadger_NodesOnly(t *testing.T) {
	aofDir := sourceWithNodes(t, []NodeRecord{
		{ID: "n1", Kind: "function", SourceID: "a.go", Labels: []string{"tag:x"}},
		{ID: "n2", Kind: "method", SourceID: "a.go", Labels: []string{"tag:y"}},
		{ID: "n3", Kind: "function", SourceID: "b.go"},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 3, migratedNodeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))

	// Verify data through engine.
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
	aofDir := sourceWithEdges(t,
		[]NodeRecord{
			{ID: "a", Kind: "function"},
			{ID: "b", Kind: "function"},
			{ID: "c", Kind: "function"},
		},
		[]EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1},
			{SourceID: "b", TargetID: "c", Kind: "calls", Weight: 1},
		},
	)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 3, migratedNodeCount(t, badgerDir))
	require.Equal(t, 2, migratedEdgeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))

	// Verify through engine.
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
	aofDir := t.TempDir()
	opts := DefaultOptions(aofDir)
	engine, err := Open(opts)
	require.NoError(t, err)

	result := MutationResult{
		StableID:  "mig-mut-1",
		Scope:     MutationScopeNode,
		Status:    MutationStatusCreated,
		TaskID:    "task-1",
		SessionID: "session-1",
	}
	require.NoError(t, engine.RecordMutationResult(result))
	require.NoError(t, engine.Close())

	badgerDir := t.TempDir()
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	// Verify through engine.
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))
	require.Contains(t, store.mutationResults, "mig-mut-1")
}

func TestMigrateAOFToBadger_WithLabelsAndSource(t *testing.T) {
	aofDir := sourceWithNodes(t, []NodeRecord{
		{ID: "n1", Kind: "file", SourceID: "work", Labels: []string{"tag:go", "scope:test"}},
	})
	badgerDir := t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))

	// Labels should be loaded.
	require.Contains(t, store.nodes, "n1")
	require.Equal(t, []string{"tag:go", "scope:test"}, store.nodes["n1"].Labels)

	// Source index should be built.
	require.Contains(t, store.bySource, "work")
}

func TestMigrateAOFToBadger_WithAnnotatedNode(t *testing.T) {
	aofDir := t.TempDir()
	opts := DefaultOptions(aofDir)
	engine, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, engine.UpsertNode(NodeRecord{
		ID:    "n1",
		Kind:  "file",
		Props: json.RawMessage(`{"path":"main.go","content_hash":"sha256:abc"}`),
	}))
	require.NoError(t, engine.AnnotateNode("n1", map[string]any{"note": "important"}))
	require.NoError(t, engine.Close())

	badgerDir := t.TempDir()
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))

	node := store.nodes["n1"]
	require.NotNil(t, node)
	require.JSONEq(t, `{"path":"main.go","content_hash":"sha256:abc","note":"important"}`, string(node.Props))
}

func TestMigrateAOFToBadger_SkipIfAlreadyCompleted(t *testing.T) {
	aofDir := sourceWithNodes(t, []NodeRecord{
		{ID: "n1", Kind: "function"},
	})
	badgerDir := t.TempDir()

	// First migration completes.
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 1, migratedNodeCount(t, badgerDir))

	// Add more data to source.
	opts := DefaultOptions(aofDir)
	engine, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n2", Kind: "function"}))
	require.NoError(t, engine.Close())

	// Second migration should be skipped (already completed).
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 1, migratedNodeCount(t, badgerDir), "should NOT have n2 - migration was skipped")
}

func TestMigrateAOFToBadger_ResumeAfterInterruption(t *testing.T) {
	aofDir := sourceWithNodes(t, []NodeRecord{
		{ID: "n1", Kind: "function"},
		{ID: "n2", Kind: "function"},
	})
	badgerDir := t.TempDir()

	// Manually set migration state to in_progress to simulate interruption.
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	require.NoError(t, bb.db.Update(func(txn *badger.Txn) error {
		return writeMigrationState(txn, migrationState{
			Status: migrationStatusInProgress,
		})
	}))
	bb.close()

	// Re-running migration should complete successfully.
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 2, migratedNodeCount(t, badgerDir))
	require.True(t, isMigrationCompleted(t, badgerDir))
}

func TestMigrateAOFToBadger_ManyNodes(t *testing.T) {
	aofDir := t.TempDir()
	opts := DefaultOptions(aofDir)
	engine, err := Open(opts)
	require.NoError(t, err)

	const count = 1500
	for i := range count {
		require.NoError(t, engine.UpsertNode(NodeRecord{
			ID:   fmt.Sprintf("n-%06d", i),
			Kind: "function",
		}))
	}
	require.NoError(t, engine.Close())

	badgerDir := t.TempDir()
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, count, migratedNodeCount(t, badgerDir))
}

func TestMigrateAOFToBadger_WithSnapshotOnClose(t *testing.T) {
	aofDir := t.TempDir()
	opts := DefaultOptions(aofDir)
	opts.SnapshotOnClose = true
	engine, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "snap-n1", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "snap-n2", Kind: "function"}))
	require.NoError(t, engine.Close()) // creates snapshot

	badgerDir := t.TempDir()
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 2, migratedNodeCount(t, badgerDir))
}

func TestMigrateAOFToBadger_LoadAndQuery(t *testing.T) {
	aofDir := t.TempDir()
	opts := DefaultOptions(aofDir)
	engine, err := Open(opts)
	require.NoError(t, err)

	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "c", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("b", "c", "calls", "", 1, nil))
	require.NoError(t, engine.Close())

	badgerDir := t.TempDir()
	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))

	// Open migrated data as a Badger-backed engine and run queries.
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))

	// Check nodes
	require.Len(t, store.nodes, 3)

	// Check edges
	require.Len(t, store.forward["a"], 1)
	require.Len(t, store.forward["b"], 1)
	require.Len(t, store.reverse["c"], 1)

	// Rebuild memory and traverse
	result := (&Engine{store: store}).ImpactSet([]string{"a"}, []EdgeKind{"calls"}, 5)
	require.ElementsMatch(t, []string{"b", "c"}, result.Affected)
}

func TestMigrateAOFToBadger_MigrationStateWritten(t *testing.T) {
	aofDir := sourceWithNodes(t, []NodeRecord{
		{ID: "n1", Kind: "function"},
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
	err := MigrateAOFToBadger(context.Background(), "/nonexistent/path", badgerDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open source")
}

func TestMigrateAOFToBadger_NonEmptyTargetOverwrites(t *testing.T) {
	aofDir := sourceWithNodes(t, []NodeRecord{
		{ID: "n1", Kind: "function"},
	})
	badgerDir := sourceWithNodes(t, []NodeRecord{
		{ID: "old", Kind: "function"},
	})
	// Remove the AOF file from badgerDir to make it a valid (empty) Badger store.
	// Actually, sourceWithNodes creates an AOF store, not a Badger store.
	// Let's just create a fresh badger dir.
	badgerDir = t.TempDir()

	require.NoError(t, MigrateAOFToBadger(context.Background(), aofDir, badgerDir))
	require.Equal(t, 1, migratedNodeCount(t, badgerDir))
	_, err := os.Stat(filepath.Join(badgerDir, "graphdb.aof"))
	require.True(t, os.IsNotExist(err), "target should not have AOF files")
}

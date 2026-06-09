package graphdb

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// ────────────────────────────────────────────────────────────────────
// Maintenance
// ────────────────────────────────────────────────────────────────────

func TestBadgerBackend_MaintenanceNoOp(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	result, err := bb.maintenance(context.Background(), MaintenanceRequest{})
	require.NoError(t, err)
	require.Equal(t, "badger", result.Backend)
	require.False(t, result.Reclaimed)
	require.Zero(t, result.CheckedRecords)
}

func TestBadgerBackend_MaintenanceIntegrityCheck(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	// Write a few records.
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function"}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n2", Kind: "function"}},
	}))

	result, err := bb.maintenance(context.Background(), MaintenanceRequest{
		IntegrityCheck: true,
	})
	require.NoError(t, err)
	// Should have at least the node keys + index keys.
	require.GreaterOrEqual(t, result.CheckedRecords, 2)
}

func TestBadgerBackend_MaintenanceGCEmpty(t *testing.T) {
	dir := t.TempDir()
	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb.close()

	// GC on an empty DB is a no-op (no value log files to rewrite).
	result, err := bb.maintenance(context.Background(), MaintenanceRequest{
		ValueLogGC: true,
	})
	require.NoError(t, err)
	require.False(t, result.Reclaimed)
}

func TestAOFBackend_Maintenance(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Access the aofBackend directly.
	ab, ok := engine.bk.(*aofBackend)
	require.True(t, ok)

	result, err := ab.maintenance(context.Background(), MaintenanceRequest{ValueLogGC: true})
	require.NoError(t, err)
	require.Equal(t, "aof", result.Backend)
	require.False(t, result.Reclaimed, "AOF backend reports GC as no-op")
}

// ────────────────────────────────────────────────────────────────────
// Backup
// ────────────────────────────────────────────────────────────────────

func TestBadgerBackend_Backup(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function"}},
	}))

	var buf bytes.Buffer
	require.NoError(t, bb.backup(context.Background(), &buf))
	require.Greater(t, buf.Len(), 0, "backup should produce output")
}

func TestAOFBackend_BackupNotSupported(t *testing.T) {
	engine, _ := newTestEngine(t)
	ab, ok := engine.bk.(*aofBackend)
	require.True(t, ok)

	err := ab.backup(context.Background(), &bytes.Buffer{})
	require.ErrorIs(t, err, ErrBackupUnsupported)
}

// ────────────────────────────────────────────────────────────────────
// Event emission
// ────────────────────────────────────────────────────────────────────

type recordingObserver struct {
	mu     sync.Mutex
	events []Event
}

func (o *recordingObserver) Observe(ev Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, ev)
}

func TestEvent_OpenStartComplete(t *testing.T) {
	obs := &recordingObserver{}
	opts := DefaultOptions(t.TempDir())
	opts.Observer = obs

	engine, err := Open(opts)
	require.NoError(t, err)
	engine.Close()

	obs.mu.Lock()
	defer obs.mu.Unlock()
	require.GreaterOrEqual(t, len(obs.events), 2, "should have at least open start and complete events")

	found := map[string]bool{}
	for _, ev := range obs.events {
		found[ev.Kind] = true
	}
	require.True(t, found[EventOpenStart], "should emit open start")
	require.True(t, found[EventOpenComplete], "should emit open complete")
}

func TestEvent_Commit(t *testing.T) {
	obs := &recordingObserver{}
	opts := DefaultOptions(t.TempDir())
	opts.Observer = obs

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n1", Kind: "function"}))

	obs.mu.Lock()
	defer obs.mu.Unlock()

	var commitEvents int
	for _, ev := range obs.events {
		if ev.Kind == EventBackendCommit {
			commitEvents++
		}
	}
	require.GreaterOrEqual(t, commitEvents, 1, "should emit commit event")
}

func TestEvent_CommitFailed(t *testing.T) {
	obs := &recordingObserver{}
	opts := DefaultOptions(t.TempDir())
	opts.Observer = obs

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

	// Inject a commit failure by using an invalid payload type through
	// the engine's persist path. The engine itself validates, so we
	// send a batch with invalid content that the backend will reject.
	// We call persist directly with an invalid op that encodeBinaryOp
	// will reject.
	require.Error(t, engine.persist("upsert_node", "not a struct"))

	obs.mu.Lock()
	defer obs.mu.Unlock()

	var failEvents int
	for _, ev := range obs.events {
		if ev.Kind == EventBackendCommitFail {
			failEvents++
			require.NotEmpty(t, ev.ErrorClass)
		}
	}
	require.GreaterOrEqual(t, failEvents, 1, "should emit commit fail event")
}

func TestEvent_MemoryApplyFail(t *testing.T) {
	obs := &recordingObserver{}
	opts := DefaultOptions(t.TempDir())
	opts.Observer = obs

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

	engine.markDirty(errors.New("apply failed"))

	obs.mu.Lock()
	defer obs.mu.Unlock()

	var found bool
	for _, ev := range obs.events {
		if ev.Kind == EventMemoryApplyFail {
			found = true
			require.Contains(t, ev.ErrorClass, "apply failed")
		}
	}
	require.True(t, found, "should emit memory apply fail event")
}

func TestEvent_TraversalComplete(t *testing.T) {
	obs := &recordingObserver{}
	opts := DefaultOptions(t.TempDir())
	opts.Observer = obs

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))

	_, _ = engine.ImpactSetContext(context.Background(), []string{"a"}, nil, 2, 100)

	obs.mu.Lock()
	defer obs.mu.Unlock()

	var traversalEvents int
	for _, ev := range obs.events {
		if ev.Kind == EventTraversalComplete {
			traversalEvents++
		}
	}
	require.GreaterOrEqual(t, traversalEvents, 1, "should emit traversal complete event")
}

func TestEvent_TraversalCancelled(t *testing.T) {
	obs := &recordingObserver{}
	opts := DefaultOptions(t.TempDir())
	opts.Observer = obs

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

	// Create graph and cancel context immediately.
	buildBranchingGraph(engine, "root", 3, 3)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = engine.ImpactSetContext(ctx, []string{"root"}, nil, 10, 1000)
	require.Error(t, err)

	obs.mu.Lock()
	defer obs.mu.Unlock()

	var cancelled bool
	for _, ev := range obs.events {
		if ev.Kind == EventTraversalCancelled {
			cancelled = true
			require.NotEmpty(t, ev.ErrorClass)
		}
	}
	require.True(t, cancelled, "should emit traversal cancelled event")
}

func TestEvent_BadgerGC(t *testing.T) {
	dir := t.TempDir()
	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb.close()

	// Write enough data to create value log files.
	for i := 0; i < 100; i++ {
		id := string(rune('a' + i))
		require.NoError(t, bb.commit(context.TODO(), mutationBatch{
			opName: "upsert_node",
			op:     nodeOp{Node: NodeRecord{ID: id, Kind: "function"}},
		}))
	}

	// GC should run without error.
	result, err := bb.maintenance(context.Background(), MaintenanceRequest{ValueLogGC: true})
	require.NoError(t, err)
	require.Equal(t, "badger", result.Backend)
}

func TestEvent_NoObserverDoesNotPanic(t *testing.T) {
	// DefaultOptions has nil Observer - events should be silently dropped.
	engine, _ := newTestEngine(t)

	require.NotPanics(t, func() {
		engine.emitEvent(Event{Kind: EventOpenStart})
		engine.emitEvent(Event{Kind: EventTraversalComplete, NodeCount: 5})
	})
}

func TestRecordingObserver_ConcurrentSafe(t *testing.T) {
	obs := &recordingObserver{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obs.Observe(Event{Kind: EventOpenStart})
		}()
	}
	wg.Wait()

	obs.mu.Lock()
	defer obs.mu.Unlock()
	require.Len(t, obs.events, 10)
}

func TestEvent_BadgerGCKind(t *testing.T) {
	// Verify the constant matches expected value.
	require.Equal(t, "graphdb.badger.gc", EventBadgerGC)
}

func TestEvent_MigrationKinds(t *testing.T) {
	require.Equal(t, "graphdb.migration.start", EventMigrationStart)
	require.Equal(t, "graphdb.migration.progress", EventMigrationProgress)
	require.Equal(t, "graphdb.migration.complete", EventMigrationComplete)
}

func TestBackup_ErrBackupUnsupported(t *testing.T) {
	require.Error(t, ErrBackupUnsupported)
	require.True(t, strings.Contains(ErrBackupUnsupported.Error(), "backup not supported"))
}

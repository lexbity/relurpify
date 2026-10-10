package jobsstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/jobs"
)

// newTestStores builds one store per backed mode; every conformance case
// runs table-driven over both.
type testStore interface {
	jobs.Store
	Close() error
}

func newTestStores(t *testing.T) map[string]testStore {
	t.Helper()
	dirStores := map[string]func() (testStore, error){
		"memory": func() (testStore, error) { return openTestStore(Options{InMemory: true}) },
		"dir":    func() (testStore, error) { return openTestStore(Options{Dir: t.TempDir()}) },
	}
	out := make(map[string]testStore, len(dirStores))
	for name, open := range dirStores {
		s, err := open()
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })
		out[name] = s
	}
	return out
}

func sampleJob(id string) jobs.Job {
	return jobs.Job{
		ID:        id,
		Spec:      jobs.Spec{Kind: "knowledge.bootstrap", Payload: map[string]any{"workspace_root": "/w"}, Queue: "knowledge"},
		State:     jobs.StateQueued,
		CreatedAt: time.Now().UTC(),
	}
}

func TestStoreConformance(t *testing.T) {
	for name, s := range newTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// Create + Load round-trip.
			in := sampleJob("j1")
			require.NoError(t, s.Create(ctx, in))
			loaded, err := s.Load(ctx, "j1")
			require.NoError(t, err)
			require.Equal(t, in.ID, loaded.ID)
			require.Equal(t, in.Spec.Kind, loaded.Spec.Kind)
			require.Equal(t, jobs.StateQueued, loaded.State)

			// Duplicate ID rejected.
			require.ErrorIs(t, s.Create(ctx, in), jobs.ErrExists)

			// CorrelateID uniqueness: a second job with the same correlate
			// id is ErrExists, and the original is untouched (FR-16).
			withCorr := sampleJob("j2")
			withCorr.Spec.CorrelateID = "corr-1"
			require.NoError(t, s.Create(ctx, withCorr))
			dup := sampleJob("j3")
			dup.Spec.CorrelateID = "corr-1"
			require.ErrorIs(t, s.Create(ctx, dup), jobs.ErrExists)
			_, err = s.Load(ctx, "j3")
			require.ErrorIs(t, err, jobs.ErrNotFound)

			// Update: state transition + queue move.
			loaded.Attempt = 1
			loaded.State = jobs.StateCompleted
			loaded.CompletedAt = time.Now().UTC()
			require.NoError(t, s.Update(ctx, *loaded))
			reread, err := s.Load(ctx, "j1")
			require.NoError(t, err)
			require.Equal(t, jobs.StateCompleted, reread.State)

			// Update of a missing job is ErrNotFound.
			missing := sampleJob("nope")
			require.ErrorIs(t, s.Update(ctx, missing), jobs.ErrNotFound)

			// List by state and queue.
			queued := sampleJob("j4")
			queued.Spec.Queue = "other"
			require.NoError(t, s.Create(ctx, queued))
			completed, err := s.List(ctx, jobs.Query{State: jobs.StateCompleted})
			require.NoError(t, err)
			require.Len(t, completed, 1)
			byQueue, err := s.List(ctx, jobs.Query{Queue: "other"})
			require.NoError(t, err)
			require.Len(t, byQueue, 1)

			// Events: append and ordered read-back.
			ev := jobs.Event{ID: "e1", JobID: "j1", Type: jobs.EventCreated, Occurred: time.Now().UTC()}
			require.NoError(t, s.AppendEvent(ctx, ev))
			ev2 := jobs.Event{ID: "e2", JobID: "j1", Type: jobs.EventCompleted, Occurred: time.Now().UTC()}
			require.NoError(t, s.AppendEvent(ctx, ev2))
			events, err := s.Events(ctx, "j1")
			require.NoError(t, err)
			require.Len(t, events, 2)
			require.Equal(t, jobs.EventCreated, events[0].Type)
			require.Equal(t, jobs.EventCompleted, events[1].Type)

			// Checkpoints: save, load, replace (latest wins).
			cp := jobs.Checkpoint{ID: "c1", JobID: "j1", State: map[string]any{"n": 1}, Created: time.Now().UTC()}
			require.NoError(t, s.SaveCheckpoint(ctx, cp))
			got, err := s.LoadCheckpoint(ctx, "j1")
			require.NoError(t, err)
			require.Equal(t, "c1", got.ID)
			cp2 := jobs.Checkpoint{ID: "c2", JobID: "j1", State: map[string]any{"n": 2}, Created: time.Now().UTC()}
			require.NoError(t, s.SaveCheckpoint(ctx, cp2))
			got, err = s.LoadCheckpoint(ctx, "j1")
			require.NoError(t, err)
			require.Equal(t, "c2", got.ID)
			_, err = s.LoadCheckpoint(ctx, "j4")
			require.ErrorIs(t, err, jobs.ErrCkptNotFound)

			// Validation errors surface, not silently corrupt.
			bad := sampleJob("")
			require.Error(t, s.Create(ctx, bad))
			require.Error(t, s.AppendEvent(ctx, jobs.Event{JobID: "j1"}))
			require.Error(t, s.SaveCheckpoint(ctx, jobs.Checkpoint{ID: "cx", JobID: "j1"}))
		})
	}
}

func TestStoreClaim_OrderingAndDue(t *testing.T) {
	for name, s := range newTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Now().UTC().Add(-time.Hour)

			// Same queue: priority desc, then FIFO within priority.
			low := sampleJob("low")
			low.Spec.Priority = 1
			low.CreatedAt = base
			highOld := sampleJob("high-old")
			highOld.Spec.Priority = 9
			highOld.CreatedAt = base.Add(time.Second)
			highNew := sampleJob("high-new")
			highNew.Spec.Priority = 9
			highNew.CreatedAt = base.Add(2 * time.Second)
			for _, j := range []jobs.Job{low, highNew, highOld} {
				require.NoError(t, s.Create(ctx, j))
			}

			claimed, err := s.Claim(ctx, "w1", []string{"knowledge"}, 10)
			require.NoError(t, err)
			require.Len(t, claimed, 3)
			require.Equal(t, []string{"high-old", "high-new", "low"},
				[]string{claimed[0].ID, claimed[1].ID, claimed[2].ID})

			// Claimed jobs are running and not claimable again
			// (exactly-once claims).
			again, err := s.Claim(ctx, "w2", []string{"knowledge"}, 10)
			require.NoError(t, err)
			require.Empty(t, again)
			for _, j := range claimed {
				got, err := s.Load(ctx, j.ID)
				require.NoError(t, err)
				require.Equal(t, jobs.StateRunning, got.State)
				require.Equal(t, 1, got.Attempt)
			}

			// Queue order: queues are drained in the order given.
			q1 := sampleJob("q1job")
			q1.Spec.Queue = "qa"
			q2 := sampleJob("q2job")
			q2.Spec.Queue = "qb"
			require.NoError(t, s.Create(ctx, q1))
			require.NoError(t, s.Create(ctx, q2))
			claimed, err = s.Claim(ctx, "w3", []string{"qb", "qa"}, 1)
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			require.Equal(t, "q2job", claimed[0].ID)

			// NextAttemptAt not due: skipped; due: claimed.
			future := sampleJob("future")
			future.NextAttemptAt = time.Now().UTC().Add(time.Hour)
			require.NoError(t, s.Create(ctx, future))
			due := sampleJob("due")
			due.NextAttemptAt = time.Now().UTC().Add(-time.Minute)
			require.NoError(t, s.Create(ctx, due))
			claimed, err = s.Claim(ctx, "w4", []string{"knowledge"}, 10)
			require.NoError(t, err)
			require.Len(t, claimed, 1)
			require.Equal(t, "due", claimed[0].ID)

			// limit=0 claims nothing without error.
			none, err := s.Claim(ctx, "w5", []string{"knowledge"}, 0)
			require.NoError(t, err)
			require.Empty(t, none)
		})
	}
}

func TestStoreCancelWhileQueued(t *testing.T) {
	for name, s := range newTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			j := sampleJob("cancelme")
			require.NoError(t, s.Create(ctx, j))

			// Cancel-while-queued: Update to cancelled; Claim skips it and
			// drops its index entry (Q15).
			loaded, err := s.Load(ctx, "cancelme")
			require.NoError(t, err)
			loaded.State = jobs.StateCancelled
			require.NoError(t, s.Update(ctx, *loaded))

			claimed, err := s.Claim(ctx, "w1", []string{"knowledge"}, 10)
			require.NoError(t, err)
			require.Empty(t, claimed)

			got, err := s.Load(ctx, "cancelme")
			require.NoError(t, err)
			require.Equal(t, jobs.StateCancelled, got.State)
		})
	}
}

func TestStoreValidationErrors(t *testing.T) {
	for name, s := range newTestStores(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			require.Error(t, s.Create(ctx, jobs.Job{}))
			_, err := s.Load(ctx, "missing")
			require.ErrorIs(t, err, jobs.ErrNotFound)
			_, err = s.Claim(ctx, "", []string{"q"}, 1)
			require.Error(t, err)
		})
	}
}

// openTestStore types the concrete store into the test interface.
func openTestStore(opts Options) (testStore, error) {
	st, err := Open(opts)
	if err != nil {
		return nil, err
	}
	return st.(*store), nil
}

func TestStoreEdgeBranches(t *testing.T) {
	concrete, err := Open(Options{InMemory: true})
	require.NoError(t, err)
	s := concrete.(*store)
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	// List honors Limit.
	for i := 0; i < 5; i++ {
		require.NoError(t, s.Create(ctx, sampleJob(fmt.Sprintf("limit-%d", i))))
	}
	limited, err := s.List(ctx, jobs.Query{Limit: 2})
	require.NoError(t, err)
	require.Len(t, limited, 2)

	// Priority clamping in the ready index: out-of-range priorities are
	// clamped to [0, 999999], so extreme jobs still claim in-band.
	hi := sampleJob("pri-high")
	hi.Spec.Priority = 1 << 30
	lo := sampleJob("pri-low")
	lo.Spec.Priority = -(1 << 30)
	require.NoError(t, s.Create(ctx, hi))
	require.NoError(t, s.Create(ctx, lo))
	claimed, err := s.Claim(ctx, "clamp-worker", []string{"knowledge"}, 2)
	require.NoError(t, err)
	require.Len(t, claimed, 2)
	require.Equal(t, "pri-high", claimed[0].ID, "clamped high priority claims first")

	// Open rejects a config with neither Dir nor InMemory.
	_, err = Open(Options{})
	require.Error(t, err)

	// An empty queued-index entry whose canonical record is gone is dropped
	// at claim time instead of erroring: seed a raw index entry for a
	// nonexistent job, then claim.
	require.NoError(t, s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("q:knowledge:999999:00000000000000000001:ghost"), []byte("ghost"))
	}))
	claimed, err = s.Claim(ctx, "ghost-worker", []string{"knowledge"}, 10)
	require.NoError(t, err, "stale index entries are dropped, not fatal")
	for _, c := range claimed {
		require.NotEqual(t, "ghost", c.ID)
	}
}

func TestStoreMaxAttemptsZeroMeansOne(t *testing.T) {
	// maxAttempts: MaxAttempts 0 resolves to a budget of 1 — a job with an
	// unspecified retry budget is attempted once and fails terminally.
	require.Equal(t, 1, maxAttempts(jobs.Job{}))
	require.Equal(t, 3, maxAttempts(jobs.Job{Spec: jobs.Spec{MaxAttempts: 3}}))
	require.Equal(t, 1, maxAttempts(jobs.Job{Spec: jobs.Spec{MaxAttempts: -2}}))
}

func TestStoreRecoveryWithBackoffOverride(t *testing.T) {
	// Recovery schedules the retry with NextBackoff: an explicit Spec.Backoff
	// (here 1s) yields NextAttemptAt ≈ recovered-at + 1s, not the default 30s.
	dir := t.TempDir()
	ctx := context.Background()
	s, err := openTestStore(Options{Dir: dir})
	require.NoError(t, err)
	j := sampleJob("backoff-override")
	j.Spec.MaxAttempts = 5
	j.Spec.Backoff = time.Second
	require.NoError(t, s.Create(ctx, j))
	_, err = s.Claim(ctx, "w", []string{"knowledge"}, 1)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	reopened, err := openTestStore(Options{Dir: dir})
	require.NoError(t, err)
	defer reopened.Close()
	loaded, err := reopened.Load(ctx, "backoff-override")
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, loaded.State)
	require.Less(t, loaded.NextAttemptAt.Sub(time.Now().UTC()), 30*time.Second,
		"the explicit base backoff must win over the 30s default")
	require.Greater(t, loaded.NextAttemptAt, time.Now().UTC().Add(500*time.Millisecond))
}

func TestRetryTxn_BoundedConflictRetries(t *testing.T) {
	concrete, err := Open(Options{InMemory: true})
	require.NoError(t, err)
	s := concrete.(*store)
	t.Cleanup(func() { _ = s.Close() })

	// A persistent conflict exhausts the bounded retries and surfaces an
	// explicit error instead of spinning forever.
	calls := 0
	err = s.retryTxn(func() error {
		calls++
		return badger.ErrConflict
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "still conflicting")
	require.Equal(t, maxConflictRetries, calls, "retries are bounded at maxConflictRetries")

	// A transient conflict followed by success retries and succeeds.
	calls = 0
	err = s.retryTxn(func() error {
		calls++
		if calls == 1 {
			return badger.ErrConflict
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)

	// errIsNotFound recognizes the full contract.
	require.True(t, errIsNotFound(badger.ErrKeyNotFound))
	require.True(t, errIsNotFound(jobs.ErrNotFound))
	require.True(t, errIsNotFound(jobs.ErrCkptNotFound))
	require.False(t, errIsNotFound(jobs.ErrExists))
}

func TestStoreMarshalFailuresSurface(t *testing.T) {
	// A payload that cannot marshal (a channel) surfaces as a create/update
	// error — never as a silently corrupted canonical record.
	s, err := openTestStore(Options{InMemory: true})
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	j := sampleJob("unmarshalable")
	j.Spec.Payload = make(chan int)
	require.Error(t, s.Create(ctx, j))

	// A valid create then an unmarshalable update fails without corrupting.
	ok := sampleJob("valid")
	require.NoError(t, s.Create(ctx, ok))
	loaded, err := s.Load(ctx, "valid")
	require.NoError(t, err)
	loaded.Spec.Payload = make(chan int)
	require.Error(t, s.Update(ctx, *loaded))
	reread, err := s.Load(ctx, "valid")
	require.NoError(t, err)
	require.Contains(t, reread.Spec.Payload, "workspace_root", "failed update must not corrupt the record")

	// A checkpoint whose State cannot marshal surfaces as an error.
	cp := jobs.Checkpoint{ID: "cb", JobID: "valid", State: make(chan int), Created: time.Now().UTC()}
	require.Error(t, s.SaveCheckpoint(ctx, cp))
}

func TestRecovery_StaleRunningEntryWithoutCanonical(t *testing.T) {
	// A running-index entry whose canonical job record is gone (corrupt
	// partial write) is skipped at recovery, not fatal.
	dir := t.TempDir()
	concrete, err := Open(Options{Dir: dir})
	require.NoError(t, err)
	s := concrete.(*store)
	require.NoError(t, err)
	require.NoError(t, s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(runningIndexKey("ghost-worker", "ghost-job"), []byte("ghost-job"))
	}))
	require.NoError(t, s.Close())

	reopened, err := openTestStore(Options{Dir: dir})
	require.NoError(t, err)
	defer reopened.Close()
	// Open succeeded; the store is usable.
	require.NoError(t, reopened.Create(context.Background(), sampleJob("after-recovery")))
}

func TestOpenRejectsUnusableDir(t *testing.T) {
	// Open fails loudly when the store directory cannot be created: a file
	// occupies the path (MkdirAllSecure error), not a silent empty store.
	file := filepath.Join(t.TempDir(), "occupied")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err := Open(Options{Dir: filepath.Join(file, "nested")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create dir")
}

func TestCorruptRecordSurfacesNotPanics(t *testing.T) {
	// A corrupt canonical record surfaces as an error on read, claim, and
	// update — it never panics and never silently vanishes.
	concrete, err := Open(Options{InMemory: true})
	require.NoError(t, err)
	s := concrete.(*store)
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	require.NoError(t, s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte("j:corrupt"), []byte("{not json")); err != nil {
			return err
		}
		return txn.Set([]byte("q:knowledge:999999:00000000000000000002:corrupt"), []byte("corrupt"))
	}))

	_, err = s.Load(ctx, "corrupt")
	require.Error(t, err, "corrupt record must surface on load")

	_, err = s.Claim(ctx, "w", []string{"knowledge"}, 10)
	require.Error(t, err, "corrupt record must surface on claim")

	_, err = s.List(ctx, jobs.Query{Queue: "knowledge"})
	require.Error(t, err, "corrupt record must surface on list")
}

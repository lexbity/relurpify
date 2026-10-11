package jobsstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/jobs"
)

func newDiskStore(t *testing.T, dir string) *store {
	t.Helper()
	s, err := Open(Options{Dir: filepath.Join(dir, "jobsdb")})
	require.NoError(t, err)
	concrete := s.(*store)
	t.Cleanup(func() { _ = concrete.Close() })
	return concrete
}

func queuedJob(id, correlate string, maxAttempts int) jobs.Job {
	return jobs.Job{
		ID:        id,
		Spec:      jobs.Spec{Kind: "sweep", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: correlate, MaxAttempts: maxAttempts},
		State:     jobs.StateQueued,
		CreatedAt: time.Now().UTC(),
	}
}

// TestRecoverySetsResumeToken proves the recovery half of the D6 chain: a job
// claimed (running) when the store closed is requeued at reopen WITH a
// ResumeToken when a checkpoint exists — the handler resumes from its batch,
// it does not cold-restart.
func TestRecoverySetsResumeToken(t *testing.T) {
	dir := t.TempDir()
	store := newDiskStore(t, dir)
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, queuedJob("job-resume", "resume-1", 3)))
	claimed, err := store.Claim(ctx, "worker-0", []string{"knowledge"}, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.NoError(t, store.SaveCheckpoint(ctx, jobs.Checkpoint{
		ID: "job-resume:batch", JobID: "job-resume", Token: "job-resume",
		State:   map[string]any{"last_chunk_id": "chunk-7"},
		Created: time.Now().UTC(),
	}))

	// Crash simulation: close WITHOUT completing; the w: index survives.
	require.NoError(t, store.Close())

	reopened := newDiskStore(t, dir)
	job, err := reopened.Load(ctx, "job-resume")
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, job.State, "recovery requeued the interrupted job")
	require.Equal(t, "job-resume", job.ResumeToken, "the requeue carries the checkpoint token")

	events, err := reopened.Events(ctx, "job-resume")
	require.NoError(t, err)
	var retried *jobs.Event
	for i := range events {
		if events[i].Type == jobs.EventRetried {
			retried = &events[i]
		}
	}
	require.NotNil(t, retried, "the requeue appends a retried event")
	require.Equal(t, true, retried.Metadata["resumed"], "the retried event carries resumed=true")
}

// TestRecoverySetsCompletedAt proves a recovery-TERMINAL failure (no attempts
// left) sets CompletedAt, so the status totals see it (FR-19).
func TestRecoverySetsCompletedAt(t *testing.T) {
	dir := t.TempDir()
	store := newDiskStore(t, dir)
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, queuedJob("job-terminal", "terminal-1", 1)))
	_, err := store.Claim(ctx, "worker-0", []string{"knowledge"}, 1)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	reopened := newDiskStore(t, dir)
	job, err := reopened.Load(ctx, "job-terminal")
	require.NoError(t, err)
	require.Equal(t, jobs.StateFailed, job.State)
	require.False(t, job.CompletedAt.IsZero(), "recovery-terminal failures carry CompletedAt")
}

// TestCreateQueuedGate proves the ready index only holds queued jobs: a job
// created in a terminal state is not claimable (FR-21).
func TestCreateQueuedGate(t *testing.T) {
	store := newDiskStore(t, t.TempDir())
	ctx := context.Background()

	done := queuedJob("job-done", "done-1", 1)
	done.State = jobs.StateCompleted
	require.NoError(t, store.Create(ctx, done))

	claimed, err := store.Claim(ctx, "worker-0", []string{"knowledge"}, 5)
	require.NoError(t, err)
	require.Empty(t, claimed, "a completed create must not enter the ready index")
}

// TestWorkerParsedFromKey proves recovery identifies the worker from the
// w: KEY, not the stored value: a value/key mismatch (crash debris) deletes
// the key's entry and recovers the key's job.
func TestWorkerParsedFromKey(t *testing.T) {
	dir := t.TempDir()
	store := newDiskStore(t, dir)
	ctx := context.Background()

	require.NoError(t, store.Create(ctx, queuedJob("job-key", "key-1", 3)))
	claimed, err := store.Claim(ctx, "worker-real", []string{"knowledge"}, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	// Corrupt the value: the w: key still names worker-real/job-key, but the
	// value now claims a phantom job (crash debris between index writes).
	require.NoError(t, store.db.Update(func(txn *badger.Txn) error {
		return txn.Set(runningIndexKey("worker-real", "job-key"), []byte("job-phantom"))
	}))
	require.NoError(t, store.Close())

	reopened := newDiskStore(t, dir)
	job, err := reopened.Load(ctx, "job-key")
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, job.State, "the key's job was recovered under its key identity")

	// A second recovery pass finds nothing left running.
	reopenedCloseErr := reopened.Close()
	require.NoError(t, reopenedCloseErr)
	again := newDiskStore(t, dir)
	job2, err := again.Load(ctx, "job-key")
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, job2.State)
}

// TestRetryHonorsContext proves FR-21's ctx contract: a cancelled caller
// stops retrying OCC conflicts instead of spinning to the retry budget.
func TestRetryHonorsContext(t *testing.T) {
	store := newDiskStore(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, store.Create(ctx, queuedJob("job-ctx", "ctx-1", 1)))
	cancel()
	// The update fails with the cancellation, not a retry-budget error.
	err := store.Update(ctx, queuedJob("job-ctx", "ctx-1", 1))
	require.ErrorIs(t, err, context.Canceled)
}

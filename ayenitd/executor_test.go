package ayenitd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/jobs"
)

// fakeHandler records invocations and returns scripted results.
type fakeHandler struct {
	mu       sync.Mutex
	calls    int
	results  []error // per-call scripted error; nil = success
	cp       jobs.Checkpoint
	blockDur time.Duration
}

func (h *fakeHandler) Handle(ctx context.Context, j jobs.Job) (jobs.Checkpoint, error) {
	h.mu.Lock()
	i := h.calls
	h.calls++
	scripted := h.results
	cp := h.cp
	block := h.blockDur
	h.mu.Unlock()
	if i < len(scripted) && scripted[i] != nil {
		return jobs.Checkpoint{}, scripted[i]
	}
	if block > 0 {
		select {
		case <-ctx.Done():
			return jobs.Checkpoint{}, ctx.Err()
		case <-time.After(block):
		}
	}
	return cp, nil
}

func submitAndRun(t *testing.T, store jobs.Store, spec jobs.Spec, handlers handlerRegistry, clock func() time.Time) func() []jobs.Job {
	t.Helper()
	require.NoError(t, store.Create(context.Background(), jobs.Job{
		ID:        "job-" + spec.CorrelateID,
		Spec:      spec,
		State:     jobs.StateQueued,
		CreatedAt: time.Now().UTC(),
	}))
	e := newExecutor(store, handlers, nil, 1, []string{spec.Queue})
	e.now = clock
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, e.Start(ctx))
	// Await a terminal state (completed or failed) before stopping, so the
	// assertions observe the run rather than the queue.
	require.Eventually(t, func() bool {
		list, err := store.List(context.Background(), jobs.Query{})
		if err != nil || len(list) != 1 {
			return false
		}
		st := list[0].State
		return st == jobs.StateCompleted || st == jobs.StateFailed
	}, 10*time.Second, 20*time.Millisecond, "job must reach a terminal state")
	return func() []jobs.Job {
		_ = e.Stop()
		list, err := store.List(context.Background(), jobs.Query{})
		require.NoError(t, err)
		return list
	}
}

func noBackoff() func() time.Time {
	// A one-nanosecond base backoff makes the retry due immediately while
	// keeping the executor's clock real (a shifted clock would backdate
	// UpdatedAt below CreatedAt and fail Job.Valid).
	return time.Now
}

func TestExecutor_Success(t *testing.T) {
	store := openTestJobsStore(t, t.TempDir())
	h := &fakeHandler{results: []error{nil}}
	stop := submitAndRun(t, store, jobs.Spec{
		Kind: "ok", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: "ok-1",
	}, handlerRegistry{"ok": h}, time.Now)
	list := stop()
	require.Len(t, list, 1)
	require.Equal(t, jobs.StateCompleted, list[0].State)
	require.Equal(t, 1, h.calls)
	events, _ := store.Events(context.Background(), "job-ok-1")
	require.Contains(t, eventTypes(events), jobs.EventCompleted)
}

func eventTypes(events []jobs.Event) []jobs.EventType {
	out := make([]jobs.EventType, 0, len(events))
	for _, e := range events {
		out = append(out, e.Type)
	}
	return out
}

func TestExecutor_TransientFailThenRetry(t *testing.T) {
	store := openTestJobsStore(t, t.TempDir())
	h := &fakeHandler{results: []error{errors.New("transient"), nil}}
	stop := submitAndRun(t, store, jobs.Spec{
		Kind: "flaky", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: "flaky-1", MaxAttempts: 3, Backoff: time.Nanosecond,
	}, handlerRegistry{"flaky": h}, noBackoff())
	list := stop()
	require.Len(t, list, 1)
	require.Equal(t, jobs.StateCompleted, list[0].State, "second attempt succeeds")
	require.Equal(t, 2, h.calls)
	require.Equal(t, 2, list[0].Attempt)
	events, _ := store.Events(context.Background(), "job-flaky-1")
	require.Contains(t, eventTypes(events), jobs.EventRetried)
	require.Contains(t, eventTypes(events), jobs.EventCompleted)
}

func TestExecutor_AttemptsExhaustedTerminal(t *testing.T) {
	store := openTestJobsStore(t, t.TempDir())
	h := &fakeHandler{results: []error{errors.New("always"), errors.New("always")}}
	stop := submitAndRun(t, store, jobs.Spec{
		Kind: "doomed", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: "doomed-1", MaxAttempts: 2, Backoff: time.Nanosecond,
	}, handlerRegistry{"doomed": h}, noBackoff())
	list := stop()
	require.Len(t, list, 1)
	require.Equal(t, jobs.StateFailed, list[0].State, "attempts exhausted: terminal failure")
	require.Equal(t, 2, h.calls, "exactly MaxAttempts attempts")
	require.False(t, list[0].CompletedAt.IsZero())
	require.Equal(t, "always", list[0].LastError)
}

func TestExecutor_TimeoutRetries(t *testing.T) {
	store := openTestJobsStore(t, t.TempDir())
	// The handler blocks past the attempt timeout and returns the expired
	// context's error, as a real slow handler does — the timeout is the
	// attempt ctx's, so it retries (an unexpired attempt ctx returning
	// cancellation classifies as shutdown-interrupted instead).
	h := &fakeHandler{blockDur: 300 * time.Millisecond}
	stop := submitAndRun(t, store, jobs.Spec{
		Kind: "slow", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: "slow-1",
		MaxAttempts: 2, Timeout: 50 * time.Millisecond, Backoff: time.Nanosecond,
	}, handlerRegistry{"slow": h}, noBackoff())
	list := stop()
	require.Len(t, list, 1)
	// Both attempts time out; attempts are exhausted → terminal failed. The
	// contract under test is that the timeout produced retries, not a silent
	// hang or an immediate terminal failure.
	require.Equal(t, jobs.StateFailed, list[0].State)
	require.Equal(t, 2, list[0].Attempt)
	require.GreaterOrEqual(t, h.calls, 2)
	require.Contains(t, list[0].LastError, "context deadline exceeded")
}

func TestExecutor_CheckpointResume(t *testing.T) {
	store := openTestJobsStore(t, t.TempDir())
	cp := jobs.Checkpoint{
		ID: "job-cp-1:sweep", JobID: "job-cp-1", Token: "token-1",
		State:   map[string]any{"last_chunk_id": "chunk-9"},
		Created: time.Now().UTC(),
	}
	h := &fakeHandler{cp: cp}
	// First attempt fails AFTER the handler returned a checkpoint to save —
	// modeled by scripting failure #1 and letting the retry observe it.
	h.results = []error{errors.New("transient"), nil}
	stop := submitAndRun(t, store, jobs.Spec{
		Kind: "sweep", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: "cp-1", MaxAttempts: 3, Backoff: time.Nanosecond,
	}, handlerRegistry{"sweep": h}, noBackoff())
	list := stop()
	require.Len(t, list, 1)
	require.Equal(t, jobs.StateCompleted, list[0].State)

	// The executor persisted the handler's checkpoint (Q15). The SUCCESS
	// path no longer stamps a resume token (D6: the requeue rule owns the
	// token; a completed job needs no resume).
	saved, err := store.LoadCheckpoint(context.Background(), "job-cp-1")
	require.NoError(t, err)
	require.Equal(t, "token-1", saved.Token)
	require.Empty(t, list[0].ResumeToken, "a completed job carries no resume token")
}

func TestExecutor_UnknownKindTerminal(t *testing.T) {
	store := openTestJobsStore(t, t.TempDir())
	stop := submitAndRun(t, store, jobs.Spec{
		Kind: "nonexistent.kind", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: "unk-1",
		MaxAttempts: 5,
	}, handlerRegistry{}, time.Now)
	list := stop()
	require.Len(t, list, 1)
	require.Equal(t, jobs.StateFailed, list[0].State, "unknown kinds are terminal, never retried")
	require.Equal(t, 1, list[0].Attempt, "no retry attempt was made")
	require.Contains(t, list[0].LastError, `unknown job kind "nonexistent.kind"`)
}

func TestExecutor_ShutdownDuringAttempt(t *testing.T) {
	// A blocking handler + executor cancel: the attempt aborts, the job stays
	// running, and the next Open's recovery pass re-queues it (Q16).
	stateDir := newTestStateDir(t)
	store := openTestJobsStore(t, stateDir+"/jobs/store")
	require.NoError(t, store.Create(context.Background(), jobs.Job{
		ID: "job-shut", Spec: jobs.Spec{Kind: "block", Payload: map[string]any{}, Queue: "knowledge", CorrelateID: "shut", MaxAttempts: 2},
		State: jobs.StateQueued, CreatedAt: time.Now().UTC(),
	}))
	blocked := make(chan struct{})
	e := newExecutor(store, handlerRegistry{"block": handlerFunc(func(ctx context.Context, j jobs.Job) (jobs.Checkpoint, error) {
		close(blocked)
		<-ctx.Done()
		return jobs.Checkpoint{}, ctx.Err()
	})}, nil, 1, []string{"knowledge"})
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, e.Start(ctx))
	<-blocked
	cancel()
	_ = e.Stop()

	loaded, err := store.Load(context.Background(), "job-shut")
	require.NoError(t, err)
	require.Equal(t, jobs.StateRunning, loaded.State, "shutdown interruption leaves the job running for recovery")

	// Reopen: recovery at Open re-queues (attempts remain).
	if closer, ok := store.(interface{ Close() error }); ok {
		require.NoError(t, closer.Close())
	}
	reopened := openTestJobsStore(t, stateDir+"/jobs/store")
	got, err := reopened.Load(context.Background(), "job-shut")
	require.NoError(t, err)
	require.Equal(t, jobs.StateQueued, got.State, "recovery re-queues the interrupted attempt")
}

// handlerFunc adapts a function to jobs.Handler.
func handlerFunc(fn func(context.Context, jobs.Job) (jobs.Checkpoint, error)) jobs.Handler {
	return handlerFn(fn)
}

type handlerFn func(context.Context, jobs.Job) (jobs.Checkpoint, error)

func (f handlerFn) Handle(ctx context.Context, j jobs.Job) (jobs.Checkpoint, error) {
	return f(ctx, j)
}

func TestExecutor_ClaimErrorBacksOff(t *testing.T) {
	// A store that fails claims (closed) must back off, not spin: the
	// executor keeps the loop alive and exits cleanly on cancel.
	store := openTestJobsStore(t, t.TempDir())
	if closer, ok := store.(interface{ Close() error }); ok {
		_ = closer.Close() // every Claim now errors
	}
	e := newExecutor(store, handlerRegistry{}, nil, 1, []string{"knowledge"})
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, e.Start(ctx))
	time.Sleep(50 * time.Millisecond)
	cancel()
	require.NoError(t, e.Stop(), "the loop must exit cleanly despite claim errors")
}

func TestNewExecutorDefaults(t *testing.T) {
	// Zero-value config: workers default to 2, queues default to knowledge.
	e := newExecutor(nil, handlerRegistry{}, nil, 0, nil)
	require.Equal(t, 2, e.workers)
	require.Equal(t, []string{"knowledge"}, e.queues)
}

func TestNewStatusWriterDefaults(t *testing.T) {
	// A non-positive heartbeat defaults to 5 s (NFR-5) and the writer is
	// runner-pid stamped.
	w := newStatusWriter(t.TempDir(), "store-dir", "rid", openTestJobsStore(t, t.TempDir()), 0)
	require.Equal(t, 5*time.Second, w.every)
	require.Equal(t, os.Getpid(), w.pid)
	require.NoError(t, w.Stop(), "Stop flushes a final draining status")
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(w.stateDir, "jobs", "status.json"))
		return err == nil
	}, time.Second, 10*time.Millisecond)
}

func TestOpenSpoolDirsRejectsUnusableRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "occupied")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err := OpenSpoolDirs(file)
	require.Error(t, err, "an unwritable state root must surface at open")
}

func TestRun_ValidationAndLockPaths(t *testing.T) {
	// Missing state dir: a usage error before anything opens.
	require.Error(t, Run(context.Background(), RunnerConfig{}))

	// A store directory that cannot be created surfaces at Run as the
	// lock/open failure path (FR-17: main maps this to a nonzero exit).
	occupied := filepath.Join(t.TempDir(), "occupied")
	require.NoError(t, os.WriteFile(occupied, []byte("x"), 0o600))
	lockErr := Run(context.Background(), RunnerConfig{StateDir: occupied, RunnerID: "t"})
	require.Error(t, lockErr)
	require.Contains(t, lockErr.Error(), "open jobs store")
}

func TestRunnerConfigDefaults(t *testing.T) {
	// Run applies defaults before opening the store; verify via the store
	// path choice by pointing StoreDir at a real temp dir and cancelling
	// immediately.
	stateDir := t.TempDir()
	cfg := RunnerConfig{StateDir: stateDir, Workers: 0, Queues: nil, Grace: 0}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate drain
	err := Run(ctx, cfg)
	require.NoError(t, err, "a cancelled context drains cleanly to exit 0")
	_, err = os.Stat(stateDir + "/jobs/store")
	require.NoError(t, err, "the default store dir is <state>/jobs/store")
}

func TestRun_CleanDrainWithDefaults(t *testing.T) {
	// Run applies its defaults (store dir <state>/jobs/store, workers 2,
	// knowledge queue) and a pre-cancelled context drains to a nil error —
	// the exit-0 shutdown contract (Q16).
	stateDir := t.TempDir()
	cfg := RunnerConfig{StateDir: stateDir, Grace: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, Run(ctx, cfg))
	_, err := os.Stat(stateDir + "/jobs/store")
	require.NoError(t, err, "the default store dir is <state>/jobs/store")
	require.NoError(t, os.MkdirAll(stateDir, 0o700))
}

// resumeRecordingHandler records every attempt's job and returns scripted
// outcomes: fail with a checkpoint first, succeed on the resumed attempt.
type resumeRecordingHandler struct {
	mu    sync.Mutex
	jobs  []jobs.Job
	fails int
}

func (h *resumeRecordingHandler) Handle(_ context.Context, j jobs.Job) (jobs.Checkpoint, error) {
	h.mu.Lock()
	h.jobs = append(h.jobs, j)
	attempt := len(h.jobs)
	fails := h.fails
	h.mu.Unlock()
	if attempt == 1 {
		return jobs.Checkpoint{
			ID: j.ID + ":batch", JobID: j.ID, Token: j.ID,
			State:   map[string]any{"last_chunk_id": "chunk-3"},
			Created: time.Now().UTC(),
		}, errors.New("transient sweep failure")
	}
	_ = fails
	return jobs.Checkpoint{}, nil
}

// TestExecutor_FailPathCheckpointResumes proves the D6 chain end-to-end: an
// attempt that fails AFTER returning a checkpoint persists it, the requeued
// job carries ResumeToken == job.ID, and the resumed attempt receives the
// checkpoint via the resume metadata — no hand-set tokens, the real requeue
// path.
func TestExecutor_FailPathCheckpointResumes(t *testing.T) {
	store := openTestJobsStore(t, t.TempDir())
	h := &resumeRecordingHandler{}
	spec := jobs.Spec{
		Kind: "sweep", Payload: map[string]any{}, Queue: "knowledge",
		CorrelateID: "resume-chain", MaxAttempts: 3, Backoff: time.Nanosecond,
	}
	require.NoError(t, store.Create(context.Background(), jobs.Job{
		ID: "job-chain", Spec: spec, State: jobs.StateQueued, CreatedAt: time.Now().UTC(),
	}))
	e := newExecutor(store, handlerRegistry{"sweep": h}, nil, 1, []string{"knowledge"})
	e.now = noBackoff()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, e.Start(ctx))
	require.Eventually(t, func() bool {
		list, err := store.List(context.Background(), jobs.Query{})
		return err == nil && len(list) == 1 && list[0].State == jobs.StateCompleted
	}, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, e.Stop())

	h.mu.Lock()
	defer h.mu.Unlock()
	require.Len(t, h.jobs, 2, "the job ran a failed attempt and a resumed attempt")
	require.Empty(t, h.jobs[0].ResumeToken, "the first attempt is a cold start")
	require.Equal(t, "job-chain", h.jobs[1].ResumeToken, "the requeue set the checkpoint token")
	require.Equal(t, "job-chain:batch", h.jobs[1].Metadata["resume_checkpoint"],
		"the resumed attempt loaded its checkpoint through the token")

	saved, err := store.LoadCheckpoint(context.Background(), "job-chain")
	require.NoError(t, err)
	require.Contains(t, saved.State, "last_chunk_id", "the fail-path checkpoint persisted")
}

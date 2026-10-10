package ayenitd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/jobsstore"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	knowledgeast "codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// e2eHarness assembles a real runner against a real temp workspace: a real
// fixture file tree, a real Badger store, a real AST index, real handlers.
type e2eHarness struct {
	t         *testing.T
	workspace string
	stateDir  string
	storeDir  string
}

func newE2EHarness(t *testing.T) *e2eHarness {
	t.Helper()
	workspace := t.TempDir()
	stateDir := t.TempDir()
	// Fixture file tree for the AST index. Go files must parse.
	for _, rel := range []string{"main.go", "util/helper.go", "docs/readme.md"} {
		path := filepath.Join(workspace, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		content := "# fixture\n"
		if strings.HasSuffix(rel, ".go") {
			content = "package main\n\nfunc e2eFixture() {}\n"
		}
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return &e2eHarness{t: t, workspace: workspace, stateDir: stateDir, storeDir: filepath.Join(stateDir, "jobs", "store")}
}

func (h *e2eHarness) submit(correlateID, kind string) error {
	dirs, err := OpenSpoolDirs(h.stateDir)
	if err != nil {
		return err
	}
	return WriteSpoolFile(dirs, spoolFile{
		CorrelateID: correlateID,
		SubmittedAt: time.Now().UTC(),
		Producer:    "relurpish",
		Spec: jobs.Spec{
			Kind: kind, Payload: map[string]any{"workspace_root": h.workspace}, Queue: "knowledge",
		},
	})
}

// readLiveStatus reads status.json — the live observation channel while the
// runner holds the store lock.
func readLiveStatus(stateDir string) (RunnerStatus, bool) {
	raw, err := os.ReadFile(filepath.Join(stateDir, "jobs", "status.json"))
	if err != nil {
		return RunnerStatus{}, false
	}
	var status RunnerStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return RunnerStatus{}, false
	}
	return status, true
}

// peekStore opens the runner's store; it must only be called while no runner
// owns the directory (the Badger lock), and closes the store before returning.
func peekStore(t *testing.T, dir string, fn func(s jobs.Store) bool) bool {
	t.Helper()
	st, err := jobsstore.Open(jobsstore.Options{Dir: dir})
	if err != nil {
		return false
	}
	defer func() {
		if closer, ok := st.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	return fn(st)
}

// indexGraphDB builds the runner's AST index engine from the workspace.
func (h *e2eHarness) indexGraphDB(ctx context.Context) *graphdb.Engine {
	h.t.Helper()
	indexDir := filepath.Join(h.stateDir, "ast")
	require.NoError(h.t, os.MkdirAll(indexDir, 0o700))
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(indexDir))
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = engine.Close(context.Background()) })
	return engine
}

// TestRunnerE2E_SpoolToCompleted is the slice's proof: write a real
// knowledge.bootstrap spool file → run Runner.Run in-process → the store
// shows completed, the AST index exists under the workspace state, and
// status.json is schema-valid with the job in recent.
func TestRunnerE2E_SpoolToCompleted(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, h.submit("e2e-boot-1", "knowledge.bootstrap"))

	engine := h.indexGraphDB(ctx)
	store := knowledgeast.NewGraphIndexStore(engine)
	indexManager := knowledgeast.NewIndexManager(store, knowledgeast.IndexConfig{WorkspacePath: h.workspace, ParallelWorkers: 2})
	indexManager.GraphDB = engine

	require.NoError(t, os.MkdirAll(filepath.Join(h.stateDir, "logs"), 0o700))
	telFile, telErr := os.OpenFile(filepath.Join(h.stateDir, "logs", "runner.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(t, telErr)
	tel := &jsonlTelemetry{fh: telFile}

	cfg := RunnerConfig{
		Workspace:    h.workspace,
		StateDir:     h.stateDir,
		Queues:       []string{"knowledge"},
		Workers:      1,
		PollEvery:    20 * time.Millisecond,
		Heartbeat:    50 * time.Millisecond,
		RunnerID:     "e2e-runner",
		IndexManager: indexManager,
		Tel:          tel,
	}
	runDone := make(chan error, 1)
	go func() { runDone <- Run(ctx, cfg) }()

	// The job completes end to end: spool → store → handler → completed.
	// status.json is the live observation channel while the runner holds the
	// store lock (FR-22).
	require.Eventually(t, func() bool {
		status, ok := readLiveStatus(h.stateDir)
		if !ok {
			return false
		}
		for _, r := range status.Recent {
			if r.CorrelateID == "e2e-boot-1" && r.State == string(jobs.StateCompleted) {
				return true
			}
		}
		return false
	}, 15*time.Second, 500*time.Millisecond, "bootstrap job must run to completion via the runner")

	cancel()
	require.NoError(t, <-runDone)

	// The AST index materialized under the workspace state.
	_, err := os.Stat(filepath.Join(h.stateDir, "ast"))
	require.NoError(t, err)

	// status.json is schema-valid and carries the job in recent.
	statusRaw, err := os.ReadFile(filepath.Join(h.stateDir, "jobs", "status.json"))
	require.NoError(t, err)
	var status RunnerStatus
	require.NoError(t, json.Unmarshal(statusRaw, &status))
	require.Equal(t, "e2e-runner", status.RunnerID)
	require.False(t, status.HeartbeatAt.IsZero())
	require.NotEmpty(t, status.Recent, "completed job appears in the recent window")
	var found bool
	for _, r := range status.Recent {
		if r.Kind == "knowledge.bootstrap" {
			found = true
		}
	}
	require.True(t, found, "bootstrap job must appear in recent: %+v", status.Recent)
}

// TestRunnerE2E_RecoverAfterHardStop is the kill -9 equivalent, staged
// deterministically: a claimed (running) job at store close time is the
// crash state; the reopen runs recovery (failed + retried events), and a
// real Runner pass then completes the re-queued job — no duplicates (NFR-4).
func TestRunnerE2E_RecoverAfterHardStop(t *testing.T) {
	h := newE2EHarness(t)
	ctx := context.Background()

	// Stage 1 — the crash state: claim a refresh job, then hard-close the
	// store with the attempt still running.
	stage1Raw, err := jobsstore.Open(jobsstore.Options{Dir: h.storeDir})
	require.NoError(t, err)
	stage1 := stage1Raw.(interface {
		jobs.Store
		Close() error
	})
	job := jobs.Job{
		ID:        "job-e2e-recover-1",
		Spec:      jobs.Spec{Kind: "knowledge.refresh", Payload: map[string]any{"workspace_root": h.workspace}, Queue: "knowledge", CorrelateID: "e2e-recover-1", MaxAttempts: 3, Backoff: time.Nanosecond},
		State:     jobs.StateQueued,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, stage1.Create(ctx, job))
	claimed, err := stage1.Claim(ctx, "victim-worker", []string{"knowledge"}, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1, "the job must be running at crash time")
	require.NoError(t, stage1.Close()) // the crash

	// Stage 2 — reopen: recovery runs at Open and appends failed+retried.
	stage2Raw, err := jobsstore.Open(jobsstore.Options{Dir: h.storeDir})
	require.NoError(t, err)
	stage2 := stage2Raw.(interface {
		jobs.Store
		Close() error
	})
	events, err := stage2.Events(ctx, "job-e2e-recover-1")
	require.NoError(t, err)
	types := eventTypes(events)
	require.Contains(t, types, jobs.EventFailed, "recovery appends the interrupted-failed event")
	require.Contains(t, types, jobs.EventRetried, "recovery re-queues the attempt")
	require.NoError(t, stage2.Close())

	// Stage 3 — a real runner pass completes the re-queued job end to end.
	engine := h.indexGraphDB(ctx)
	chunkStore := &knowledge.ChunkStore{Graph: engine}
	staleness := &knowledge.StalenessManager{Store: chunkStore, Propagate: true, MaxDepth: 3}
	cfg := RunnerConfig{
		Workspace:  h.workspace,
		StateDir:   h.stateDir,
		Queues:     []string{"knowledge"},
		Workers:    1,
		PollEvery:  20 * time.Millisecond,
		Heartbeat:  50 * time.Millisecond,
		RunnerID:   "e2e-recover",
		ChunkStore: chunkStore,
		Staleness:  staleness,
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- Run(runCtx, cfg) }()

	// The re-queued job completes: status.json is the live channel.
	require.Eventually(t, func() bool {
		status, ok := readLiveStatus(h.stateDir)
		if !ok {
			return false
		}
		for _, r := range status.Recent {
			if r.CorrelateID == "e2e-recover-1" && r.State == string(jobs.StateCompleted) {
				return true
			}
		}
		return false
	}, 15*time.Second, 100*time.Millisecond, "the recovered job completes on the runner pass")

	runCancel()
	require.NoError(t, <-runDone)

	// No duplicates: exactly one job per CorrelateID, now completed.
	finalRaw, err := jobsstore.Open(jobsstore.Options{Dir: h.storeDir})
	require.NoError(t, err)
	final := finalRaw.(interface {
		jobs.Store
		Close() error
	})
	list, err := final.List(ctx, jobs.Query{})
	require.NoError(t, err)
	require.Len(t, list, 1, "exactly one materialization per CorrelateID (NFR-4)")
	require.Equal(t, jobs.StateCompleted, list[0].State)
	require.Equal(t, "e2e-recover-1", list[0].Spec.CorrelateID)
	finalEvents, err := final.Events(ctx, "job-e2e-recover-1")
	require.NoError(t, err)
	require.Equal(t, jobs.EventCompleted, finalEvents[len(finalEvents)-1].Type)
	require.NoError(t, final.Close())
}

// TestRunnerE2E_LockSingleExecutor is FR-17: a second Runner on the same
// store directory fails fast on the Badger lock while the first holds it.
func TestRunnerE2E_LockSingleExecutor(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := h.indexGraphDB(ctx)
	indexManager := knowledgeast.NewIndexManager(knowledgeast.NewGraphIndexStore(engine), knowledgeast.IndexConfig{WorkspacePath: h.workspace})
	indexManager.GraphDB = engine
	cfg := RunnerConfig{
		Workspace: h.workspace, StateDir: h.stateDir, Queues: []string{"knowledge"},
		Workers: 1, PollEvery: 50 * time.Millisecond, Heartbeat: 50 * time.Millisecond,
		RunnerID: "e2e-lock", IndexManager: indexManager,
	}
	runDone := make(chan error, 1)
	go func() { runDone <- Run(ctx, cfg) }()

	// The winner holds the lock and serves.
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(h.stateDir, "jobs", "status.json"))
		return err == nil
	}, 10*time.Second, 50*time.Millisecond, "the winner must be up and heartbeating")

	// The loser exits nonzero on the lock (attach, don't fight).
	err := Run(ctx, cfg)
	require.Error(t, err, "the second runner must fail on the store lock")

	cancel()
	require.NoError(t, <-runDone)
}

// TestRunnerE2E_TelemetryJSONL proves the observability contract: the run's
// telemetry JSONL carries submitted/claimed/completed events (NFR-9).
func TestRunnerE2E_TelemetryJSONL(t *testing.T) {
	h := newE2EHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, h.submit("e2e-tel-1", "knowledge.bootstrap"))

	engine := h.indexGraphDB(ctx)
	indexManager := knowledgeast.NewIndexManager(knowledgeast.NewGraphIndexStore(engine), knowledgeast.IndexConfig{WorkspacePath: h.workspace})
	indexManager.GraphDB = engine

	telLog := filepath.Join(h.stateDir, "logs", "runner.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(telLog), 0o700))
	telFile, telErr := os.OpenFile(telLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(t, telErr)
	tel := &jsonlTelemetry{fh: telFile}

	cfg := RunnerConfig{
		Workspace: h.workspace, StateDir: h.stateDir, Queues: []string{"knowledge"},
		Workers: 1, PollEvery: 20 * time.Millisecond, Heartbeat: 50 * time.Millisecond,
		RunnerID: "e2e-tel", IndexManager: indexManager, Tel: tel,
	}
	runDone := make(chan error, 1)
	go func() { runDone <- Run(ctx, cfg) }()

	// status.json is the live completion channel; then drain the runner.
	require.Eventually(t, func() bool {
		status, ok := readLiveStatus(h.stateDir)
		if !ok {
			return false
		}
		for _, r := range status.Recent {
			if r.CorrelateID == "e2e-tel-1" && r.State == string(jobs.StateCompleted) {
				return true
			}
		}
		return false
	}, 15*time.Second, 100*time.Millisecond, "the job completes before telemetry assertions")

	cancel()
	require.NoError(t, <-runDone)

	data, err := os.ReadFile(telLog)
	require.NoError(t, err)
	joined := string(data)
	for _, want := range []string{`"job.submitted"`, `"job.claimed"`, `"job.completed"`, `"runner.started"`, `"runner.stopped"`} {
		require.Contains(t, joined, want, "telemetry JSONL must carry %s:\n%s", want, joined)
	}
}

// jsonlTelemetry writes runner events as JSONL — the same shape as the
// main.go file telemetry, local to the tests.
type jsonlTelemetry struct {
	mu sync.Mutex
	fh *os.File
}

func (t *jsonlTelemetry) Emit(ev telemetry.Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = t.fh.Write(append(data, '\n'))
}

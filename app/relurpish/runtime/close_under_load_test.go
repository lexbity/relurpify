package runtime

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/session"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// sleepingExecutor is a workflow executor whose turn is in flight until its
// run context is cancelled: it models a turn mid-LLM-call. It honors
// cancellation, so Close's cancel step is what releases it.
type sleepingExecutor struct {
	started chan struct{}
	once    sync.Once
}

func newSleepingExecutor() *sleepingExecutor {
	return &sleepingExecutor{started: make(chan struct{})}
}

func (s *sleepingExecutor) Initialize(*execution.Config) error { return nil }
func (s *sleepingExecutor) Capabilities() []string             { return nil }

func (s *sleepingExecutor) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	return nil, nil
}

func (s *sleepingExecutor) Execute(ctx context.Context, _ *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	return &execution.Result{NodeID: "sleep", Success: true}, nil
}

// TestRuntimeCloseUnderLoadDrainsThenTearsDown runs a real in-flight turn and
// calls Close() mid-turn (AC-8): the coordinator drains/cancels the run before
// the JSONL telemetry sink and the Badger store are torn down, the telemetry
// file absorbs the shutdown events without a write error, and the store opens
// cleanly afterwards — no LOCK contention left behind.
func TestRuntimeCloseUnderLoadDrainsThenTearsDown(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()

	// Real Badger store rooted in the workspace state dir so the engine lock's
	// release is observable after Close.
	stateDir := filepath.Join(workspace, ".relurpify_state")
	require.NoError(t, os.MkdirAll(stateDir, 0o700))
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(stateDir))
	require.NoError(t, err)
	indexStore := ast.NewGraphIndexStore(engine)
	indexManager := ast.NewIndexManager(indexStore, ast.IndexConfig{WorkspacePath: workspace})
	indexManager.GraphDB = engine

	// Real JSONL telemetry sink: the shutdown accounting must land on disk
	// without a write error even though Close interrupts a live turn.
	telemetryPath := filepath.Join(workspace, "telemetry.jsonl")
	sink, err := telemetry.NewJSONFileTelemetry(telemetryPath)
	require.NoError(t, err)

	ws := &session.Workspace{
		Telemetry: sink,
		Logger:    log.New(io.Discard, "", 0),
	}
	ws.Environment.IndexManager = indexManager

	coord := newRunCoordinator(ws.Telemetry)
	coord.SetDurations(100*time.Millisecond, time.Second)
	rt := &Runtime{
		Config:      Config{Workspace: workspace},
		Workspace:   ws,
		coordinator: coord,
	}
	sleeping := newSleepingExecutor()
	rt.setAgent(sleeping)

	runResult := make(chan error, 1)
	go func() {
		_, err := rt.RunTask(ctx, &execution.Task{ID: "task-load", Instruction: "work"})
		runResult <- err
	}()

	// Wait until the turn is genuinely in flight, then close mid-turn.
	<-sleeping.started
	require.NoError(t, rt.Close(ctx))
	require.NoError(t, <-runResult, "the in-flight turn must finish cleanly after Close")

	// The JSONL sink recorded the quiesce accounting and closed without error.
	raw, err := os.ReadFile(telemetryPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), telemetry.EventShutdownDrain,
		"shutdown accounting must reach the durable telemetry trail")

	// The Badger lock is released: the same store directory opens fresh.
	reopened, err := graphdb.Open(ctx, graphdb.DefaultOptions(stateDir))
	require.NoError(t, err, "store must open cleanly after Close (no LOCK contention)")
	require.NoError(t, reopened.Close(ctx))
}

package agentgraph

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// recordingGrounder records grounding batches and can be configured to fail.
type recordingGrounder struct {
	mu      sync.Mutex
	calls   [][]knowledge.GroundingItem
	reports []knowledge.GroundingReport
	err     error
}

func (g *recordingGrounder) Ground(_ context.Context, items []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, items)
	if g.err != nil {
		return knowledge.GroundingReport{}, g.err
	}
	report := knowledge.GroundingReport{Grounded: make([]knowledge.GroundingEntry, len(items))}
	for i := range items {
		report.Grounded[i] = knowledge.GroundingEntry{Index: i, Action: knowledge.GroundingGrounded}
	}
	return report, nil
}

func (g *recordingGrounder) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.calls)
}

// epochTelemetry records telemetry events emitted by the coordinator.
type epochTelemetry struct {
	mu     sync.Mutex
	events []fwtelemetry.Event
}

func (e *epochTelemetry) Emit(ev fwtelemetry.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, ev)
}

func (e *epochTelemetry) count(kind fwtelemetry.EventType) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, ev := range e.events {
		if ev.Type == kind {
			n++
		}
	}
	return n
}

func captureItem(value string) knowledge.GroundingItem {
	return knowledge.GroundingItem{
		Value:       value,
		Epistemics:  knowledge.EpistemicClaimed,
		Origin:      "llm",
		StateKey:    "state.x",
		NodeID:      "node-a",
		TaskID:      "task-1",
		SessionID:   "session-1",
		WorkspaceID: "ws-1",
		Kind:        knowledge.ChunkKindCapture,
	}
}

func TestEpochCoordinatorCleanBarrierIsCheap(t *testing.T) {
	grounder := &recordingGrounder{}
	coord := NewEpochCoordinator(context.Background(), grounder, nil)
	start := time.Now()
	require.NoError(t, coord.CloseEpochIfPending("node-a"))
	require.Less(t, time.Since(start), 5*time.Millisecond, "clean barrier must be a no-op")
	require.Equal(t, 0, grounder.callCount())
}

func TestEpochCoordinatorFlushGroundsAndOpensEpoch(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	tel := &epochTelemetry{}
	grounder := &recordingGrounder{}
	coord := NewEpochCoordinator(contextdata.WithEnvelope(context.Background(), env), grounder, tel)
	coord.EnqueueCapture(captureItem("one"))
	coord.EnqueueCapture(captureItem("two"))

	require.NoError(t, coord.CloseEpochIfPending("node-a"))
	require.Equal(t, 1, grounder.callCount(), "one atomic flush for two captures")
	require.Equal(t, "task-1", grounder.calls[0][0].TaskID)
	require.Equal(t, uint64(1), grounder.calls[0][0].Epoch, "capture stamped with opening epoch")
	require.Equal(t, uint64(2), coord.EpochID(), "epoch opened")
	require.Equal(t, uint64(2), env.AssemblyMetadataSnapshot().EpochID, "envelope stamped with opening epoch")
	require.Equal(t, 1, tel.count(fwtelemetry.EventEpochClosed))
	captures, jobs := coord.Pending()
	require.Zero(t, captures)
	require.Zero(t, jobs)
}

func TestEpochCoordinatorGroundingFailsWithTypedError(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	grounder := &recordingGrounder{err: knowledge.ErrGroundingFailed}
	coord := NewEpochCoordinator(contextdata.WithEnvelope(context.Background(), env), grounder, nil)
	coord.EnqueueCapture(captureItem("one"))

	err := coord.CloseEpochIfPending("node-a")
	require.Error(t, err)
	require.True(t, errors.Is(err, knowledge.ErrGroundingFailed), "barrier failure must satisfy errors.Is(ErrGroundingFailed)")
	require.Equal(t, 2, grounder.callCount(), "grounding must be retried once before failing")
}

// hangingCompiler blocks until released or its context ends.
type hangingCompiler struct {
	release chan struct{}
}

func (h *hangingCompiler) Compile(ctx context.Context, _ contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	select {
	case <-h.release:
		return &contextports.CompilationResult{StreamedRefs: []string{"too-late"}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestEpochCoordinatorAwaitsStreamJob(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.AssemblyMetadata.EventLogSeq = 9
	env.AssemblyMetadata.BudgetTokens = 100
	compilerStub := &streamCompilerStub{result: &contextports.CompilationResult{
		StreamedRefs: []string{"chunk-1"},
		Record:       contextports.CompilationRecord{ID: "rec-1"},
	}}
	runCtx := contextdata.WithEnvelope(context.Background(), env)
	coord := NewEpochCoordinator(runCtx, &recordingGrounder{}, nil)
	trigger := contextstream.NewTrigger(compilerStub)
	job, err := trigger.RequestBackground(runCtx, contextstream.Request{ID: "job-1", Mode: contextstream.ModeBackground})
	require.NoError(t, err)
	coord.TrackStreamJob(job)

	require.NoError(t, coord.CloseEpochIfPending("node-a"))
	require.Equal(t, []contextdata.ChunkID{"chunk-1"}, env.StreamedChunkIDs())
	meta := env.AssemblyMetadataSnapshot()
	require.Equal(t, "rec-1", meta.CompilationID)
	require.Equal(t, uint64(9), meta.EventLogSeq, "ApplyResult must preserve EventLogSeq")
	require.Equal(t, 100, meta.BudgetTokens, "ApplyResult must preserve BudgetTokens")
}

func TestEpochCoordinatorAbandonsHangingJob(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	tel := &epochTelemetry{}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hang := &hangingCompiler{release: make(chan struct{})}
	coord := NewEpochCoordinator(contextdata.WithEnvelope(runCtx, env), &recordingGrounder{}, tel)
	coord.SetStreamJobDeadline(40 * time.Millisecond)
	trigger := contextstream.NewTrigger(hang)
	job, err := trigger.RequestBackground(runCtx, contextstream.Request{ID: "job-hang", Mode: contextstream.ModeBackground})
	require.NoError(t, err)
	coord.TrackStreamJob(job)

	require.NoError(t, coord.CloseEpochIfPending("node-a"), "an abandoned job must not fail the run")
	require.Equal(t, 1, tel.count(fwtelemetry.EventStreamAbandoned))
	require.Empty(t, env.StreamedChunkIDs(), "partial result must not be applied")

	// Release the blocked compiler so the run-lifetime goroutine exits.
	close(hang.release)
	cancel()
}

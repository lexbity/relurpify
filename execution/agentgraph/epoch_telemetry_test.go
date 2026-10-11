package agentgraph

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestBarrierPathStaleApplyDroppedIsObserved is the P2 regression proof: a
// background job whose result loses the epoch race is dropped by the guard
// and the drop is emitted on the COORDINATOR's constructor-injected sink.
// The barrier path's run context carries correlation and envelope only — no
// telemetry sink — so the old ctx-based emission could never fire here.
func TestBarrierPathStaleApplyDroppedIsObserved(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	// A newer blocking slice (epoch 5) is already stored.
	env.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID: "req-blocking", Epoch: 5, FinalTokens: 10,
		Chunks: []contextdata.StreamedChunk{{ChunkID: "chunk-new", Body: "new", TokenEstimate: 10}},
	})

	tel := &epochTelemetry{}
	compilerStub := &streamCompilerStub{result: &contextports.CompilationResult{
		StreamedRefs: []string{"chunk-stale"},
		Record:       contextports.CompilationRecord{ID: "req-stale"},
	}}
	runCtx := contextdata.WithEnvelope(context.Background(), env)
	coord := NewEpochCoordinator(runCtx, &recordingGrounder{}, tel)
	trigger := contextstream.NewTrigger(compilerStub)
	job, err := trigger.RequestBackground(runCtx, contextstream.Request{ID: "job-stale", Mode: contextstream.ModeBackground})
	require.NoError(t, err)
	coord.TrackStreamJob(job)
	require.Eventually(t, func() bool {
		select {
		case <-job.Done():
			return true
		default:
			return false
		}
	}, 2*time.Second, time.Millisecond, "the stub job must complete before the barrier")

	// The barrier applies at epoch 1 — older than the stored slice (5): the
	// guard drops it and the coordinator reports the drop on its own sink.
	require.NoError(t, coord.CloseEpochIfPending("node-a"), "a stale-dropped apply is the designed behavior, not a barrier failure")
	require.Equal(t, 1, tel.count(fwtelemetry.EventContextStreamStaleApplyDropped),
		"the barrier path must observe the drop through the coordinator's sink")
	require.Empty(t, env.StreamedChunkIDs(), "the stale result must mutate nothing")
}

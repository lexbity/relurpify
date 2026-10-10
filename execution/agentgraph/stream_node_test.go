package agentgraph

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type streamCompilerStub struct {
	request contextports.CompilationRequest
	result  *contextports.CompilationResult
}

func (s *streamCompilerStub) Compile(ctx context.Context, request contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	s.request = request
	return s.result, nil
}

// noopGrounder is a Grounder that accepts everything without storage.
type noopGrounder struct{}

func (noopGrounder) Ground(_ context.Context, _ []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	return knowledge.GroundingReport{}, nil
}

func TestContextStreamNodeBlockingAppliesRefsToEnvelope(t *testing.T) {
	compilerStub := &streamCompilerStub{
		result: &contextports.CompilationResult{
			StreamedRefs:    []string{"chunk-1"},
			ShortfallTokens: 9,
		},
	}
	node := NewContextStreamNode("stream-node", retrieval.RetrievalQuery{Text: "workspace query"}, 256)
	node.Mode = contextstream.ModeBlocking

	env := contextdata.NewEnvelope("task-1", "session-1")
	meta := env.AssemblyMetadataSnapshot()
	meta.EventLogSeq = 12
	env.SetAssemblyMetadata(meta)
	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(compilerStub))
	result, err := node.Execute(ctx, env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "stream-node", result.NodeID)
	require.Equal(t, []contextdata.ChunkID{"chunk-1"}, env.StreamedChunkIDs())
	requestID, ok := contextdata.GetTyped[string](env, "contextstream.request_id")
	require.True(t, ok)
	require.Equal(t, "stream-node.stream", requestID)
	shortfall, ok := contextdata.GetTyped[int](env, "contextstream.shortfall_tokens")
	require.True(t, ok)
	require.Equal(t, 9, shortfall)
}

func TestContextStreamNodeBackgroundLandsAtBarrier(t *testing.T) {
	compilerStub := &streamCompilerStub{
		result: &contextports.CompilationResult{
			StreamedRefs: []string{"chunk-2"},
			Record:       contextports.CompilationRecord{ID: "compilation-bg"},
		},
	}
	node := NewContextStreamNode("stream-node-bg", retrieval.RetrievalQuery{Text: "background query"}, 64)
	node.Mode = contextstream.ModeBackground

	env := contextdata.NewEnvelope("task-2", "session-2")
	env.AssemblyMetadata.EventLogSeq = 7
	env.AssemblyMetadata.BudgetTokens = 64
	coord := NewEpochCoordinator(contextdata.WithEnvelope(context.Background(), env), &noopGrounder{}, nil)
	ctx := contextstream.WithTrigger(WithEpochCoordinator(context.Background(), coord), contextstream.NewTrigger(compilerStub))
	result, err := node.Execute(ctx, env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "stream-node-bg", result.NodeID)
	requested, _ := execution.ResultField(result.Data, "contextstream_background_requested")
	require.Equal(t, true, requested)
	jobID, _ := execution.ResultField(result.Data, "contextstream_job_id")
	require.Equal(t, "stream-node-bg.stream", jobID)

	// The job is applied by the epoch barrier, not a detached goroutine.
	captures, jobs := coord.Pending()
	require.Zero(t, captures)
	require.Equal(t, 1, jobs)

	require.NoError(t, coord.CloseEpochIfPending("stream-node-bg"))
	require.Equal(t, []contextdata.ChunkID{"chunk-2"}, env.StreamedChunkIDs())
	meta := env.AssemblyMetadataSnapshot()
	require.Equal(t, "compilation-bg", meta.CompilationID)
	require.Equal(t, uint64(7), meta.EventLogSeq, "ApplyResult must merge, not replace")
	require.Equal(t, 64, meta.BudgetTokens, "ApplyResult must merge, not replace")
	require.Equal(t, uint64(2), meta.EpochID)

	captures, jobs = coord.Pending()
	require.Zero(t, captures)
	require.Zero(t, jobs, "barrier must drain tracked jobs")
}

func TestContextStreamNodeBackgroundRequiresCoordinator(t *testing.T) {
	compilerStub := &streamCompilerStub{
		result: &contextports.CompilationResult{StreamedRefs: []string{"chunk-2"}},
	}
	node := NewContextStreamNode("stream-node-bg", retrieval.RetrievalQuery{Text: "background query"}, 64)
	node.Mode = contextstream.ModeBackground

	env := contextdata.NewEnvelope("task-2", "session-2")
	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(compilerStub))
	_, err := node.Execute(ctx, env)
	require.Error(t, err)
	require.Contains(t, err.Error(), "background stream requires an epoch coordinator")
}

// TestContextStreamNodeBlockingLandsSlice extends the blocking case to the
// typed slice: chunks with bodies land on the envelope, the working-state
// entry stays a bounded summary (node contract ≤4096 bytes), and the renderer
// sees the body.
func TestContextStreamNodeBlockingLandsSlice(t *testing.T) {
	compilerStub := &streamCompilerStub{
		result: &contextports.CompilationResult{
			StreamedChunks: []contextports.StreamedChunkView{
				{ChunkID: "chunk-1", ContentHash: "h1", Body: "the grounded body", TokenEstimate: 10, TrustClass: "workspace"},
			},
			StreamedRefs: []string{"chunk-1"},
			Record:       contextports.CompilationRecord{ID: "req-1", FinalTokens: 10, OriginalBudget: 8192},
		},
	}
	node := NewContextStreamNode("stream-node", retrieval.RetrievalQuery{Text: "query"}, 256)
	env := contextdata.NewEnvelope("task-1", "session-1")
	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(compilerStub))

	_, err := node.Execute(ctx, env)
	require.NoError(t, err)

	slice := env.StreamedSliceSnapshot()
	require.NotNil(t, slice)
	require.Equal(t, "the grounded body", slice.Chunks[0].Body)

	summary, ok := contextdata.GetTyped[map[string]any](env, "contextstream.result")
	require.True(t, ok, "bounded summary must be present")
	require.Equal(t, "stream-node.stream", summary["request_id"])
	summaryBytes, err := json.Marshal(summary)
	require.NoError(t, err)
	require.LessOrEqual(t, len(summaryBytes), 4096, "node contract MaxStateEntryBytes")

	section, _, err := contextstream.RenderStreamedSection(env)
	require.NoError(t, err)
	require.Contains(t, section, "the grounded body")
}

// TestContextStreamNodeEpochRegressedBackgroundDrops proves the D-4 guard at
// the barrier: a background job whose epoch is older than the stored slice is
// dropped with contextstream.stale_apply_dropped, never rendered.
func TestContextStreamNodeEpochRegressedBackgroundDrops(t *testing.T) {
	sink := &recordingEpochTelemetry{}
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	env := contextdata.NewEnvelope("task-1", "session-1")
	// A newer blocking slice (epoch 5) is already stored.
	env.SetStreamedSlice(&contextdata.StreamedSlice{
		RequestID: "req-blocking", Epoch: 5, FinalTokens: 10,
		Chunks: []contextdata.StreamedChunk{{ChunkID: "chunk-new", Body: "new", TokenEstimate: 10}},
	})

	older := &contextstream.Result{
		Request: contextstream.Request{ID: "req-old"},
		Compilation: &contextports.CompilationResult{
			StreamedChunks: []contextports.StreamedChunkView{
				{ChunkID: "chunk-old", Body: "old", TokenEstimate: 10},
			},
			Record: contextports.CompilationRecord{ID: "req-old", FinalTokens: 10},
		},
	}
	require.NoError(t, contextstream.ApplyResult(ctx, env, older, 4))

	slice := env.StreamedSliceSnapshot()
	require.Equal(t, "req-blocking", slice.RequestID, "older background apply must not replace the slice")
	require.Equal(t, 1, sink.count(telemetry.EventContextStreamStaleApplyDropped))
}

type recordingEpochTelemetry struct {
	events []telemetry.Event
}

func (s *recordingEpochTelemetry) Emit(event telemetry.Event) {
	s.events = append(s.events, event)
}

func (s *recordingEpochTelemetry) count(eventType telemetry.EventType) int {
	n := 0
	for _, event := range s.events {
		if event.Type == eventType {
			n++
		}
	}
	return n
}

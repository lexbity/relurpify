package agentgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
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

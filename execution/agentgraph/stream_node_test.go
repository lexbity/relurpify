package agentgraph

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
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
	env.AssemblyMetadata.EventLogSeq = 12
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

func TestContextStreamNodeBackgroundAppliesEventually(t *testing.T) {
	compilerStub := &streamCompilerStub{
		result: &contextports.CompilationResult{
			StreamedRefs: []string{"chunk-2"},
		},
	}
	node := NewContextStreamNode("stream-node-bg", retrieval.RetrievalQuery{Text: "background query"}, 64)
	node.Mode = contextstream.ModeBackground

	env := contextdata.NewEnvelope("task-2", "session-2")
	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(compilerStub))
	result, err := node.Execute(ctx, env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "stream-node-bg", result.NodeID)
	mode, _ := execution.ResultField(result.Data, "mode")
	require.Equal(t, "background", mode)
	jobID, _ := execution.ResultField(result.Data, "contextstream_job_id")
	require.Equal(t, "stream-node-bg.stream", jobID)

	require.Eventually(t, func() bool {
		ids := env.StreamedChunkIDs()
		return len(ids) == 1 && ids[0] == "chunk-2"
	}, time.Second, 10*time.Millisecond)
}

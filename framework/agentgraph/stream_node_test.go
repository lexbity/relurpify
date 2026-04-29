package agentgraph

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"github.com/stretchr/testify/require"
)

type streamCompilerStub struct {
	request compiler.CompilationRequest
	result  *compiler.CompilationResult
	record  *compiler.CompilationRecord
}

func (s *streamCompilerStub) Compile(ctx context.Context, request compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error) {
	s.request = request
	return s.result, s.record, nil
}

func TestContextStreamNodeBlockingAppliesRefsToEnvelope(t *testing.T) {
	compilerStub := &streamCompilerStub{
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{
				{ChunkID: "chunk-1", Rank: 1},
			},
			ShortfallTokens: 9,
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "comp-1"},
		},
	}
	node := NewContextStreamNode("stream-node", contextstream.NewTrigger(compilerStub), "workspace query", 256)
	node.Mode = contextstream.ModeBlocking

	env := contextdata.NewEnvelope("task-1", "session-1")
	env.AssemblyMetadata.EventLogSeq = 12
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "stream-node", result.NodeID)
	require.Equal(t, []contextdata.ChunkID{"chunk-1"}, env.StreamedChunkIDs())
	requestID, ok := env.GetWorkingValue("contextstream.request_id")
	require.True(t, ok)
	require.Equal(t, "stream-node.stream", requestID)
	shortfall, ok := env.GetWorkingValue("contextstream.shortfall_tokens")
	require.True(t, ok)
	require.Equal(t, 9, shortfall)
	require.Equal(t, uint64(12), compilerStub.request.EventLogSeq)
	require.Equal(t, 256, compilerStub.request.MaxTokens)
	require.Equal(t, "workspace query", compilerStub.request.Query.Text)
}

func TestContextStreamNodeBackgroundAppliesEventually(t *testing.T) {
	compilerStub := &streamCompilerStub{
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{
				{ChunkID: "chunk-2", Rank: 1},
			},
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "comp-2"},
		},
	}
	node := NewContextStreamNode("stream-node-bg", contextstream.NewTrigger(compilerStub), "background query", 64)
	node.Mode = contextstream.ModeBackground

	env := contextdata.NewEnvelope("task-2", "session-2")
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "stream-node-bg", result.NodeID)
	require.Equal(t, "background", result.Data["mode"])
	require.Equal(t, "stream-node-bg.stream", result.Data["contextstream_job_id"])

	require.Eventually(t, func() bool {
		ids := env.StreamedChunkIDs()
		return len(ids) == 1 && ids[0] == "chunk-2"
	}, time.Second, 10*time.Millisecond)
}

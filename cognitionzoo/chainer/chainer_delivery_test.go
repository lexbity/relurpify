package chainer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// capturingInvoker is the narrow CompilerInvoker fake: it records the request
// it received and returns a scripted compilation result. onCompile, when set,
// runs while the "compile" is in flight — the seam for simulating a barrier
// closing between request-build and apply.
type capturingInvoker struct {
	captured  contextports.CompilationRequest
	result    *contextports.CompilationResult
	onCompile func()
}

func (c *capturingInvoker) Compile(_ context.Context, request contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	c.captured = request
	if c.onCompile != nil {
		c.onCompile()
	}
	return c.result, nil
}

// chainerStreamFixture builds the full seams executeStreamingTrigger reads:
// an envelope seeded by a real run coordinator, a recording trigger, and a
// chainer configured for a blocking stream.
func chainerStreamFixture(t *testing.T) (*ChainerAgent, *contextdata.Envelope, *capturingInvoker, context.Context) {
	t.Helper()

	env := contextdata.NewEnvelope("task-1", "sess-1")
	ctx := contextdata.WithEnvelope(context.Background(), env)
	coord := agentgraph.NewEpochCoordinator(ctx, nil, nil)
	ctx = agentgraph.WithEpochCoordinator(ctx, coord)

	invoker := &capturingInvoker{result: &contextports.CompilationResult{
		StreamedChunks: []contextports.StreamedChunkView{{
			ChunkID:       "chunk:chainer-1",
			ContentHash:   "hash-1",
			Body:          "CHAINER-SENTINEL-BODY",
			TokenEstimate: 10,
		}},
		StreamedRefs: []string{"chunk:chainer-1"},
		Record: contextports.CompilationRecord{
			OriginalBudget: 1024,
			FinalTokens:    10,
		},
	}}
	ctx = contextstream.WithTrigger(ctx, contextstream.NewTrigger(invoker))

	agent := &ChainerAgent{
		StreamQuery:     "sentinel query",
		StreamMode:      contextstream.ModeBlocking,
		StreamMaxTokens: 512,
	}
	return agent, env, invoker, ctx
}

// TestChainerDeliversSlice proves the chainer compile is first-class context
// delivery: the compiled slice lands on the envelope under the run's epoch,
// and the shared renderer renders its body — the same front-door intake and
// the stream node use. The pre-fix implementation dumped the raw result into
// working state, where no renderer could see it.
func TestChainerDeliversSlice(t *testing.T) {
	agent, env, invoker, ctx := chainerStreamFixture(t)

	require.NoError(t, agent.executeStreamingTrigger(ctx, &execution.Task{ID: "task-1"}, env))

	slice := env.StreamedSliceSnapshot()
	require.NotNil(t, slice, "the compiled slice must land on the envelope")
	require.Len(t, slice.Chunks, 1)
	require.Equal(t, "CHAINER-SENTINEL-BODY", slice.Chunks[0].Body)
	require.Equal(t, uint64(1), slice.Epoch)

	out, stats, err := contextstream.RenderStreamedSection(env)
	require.NoError(t, err)
	require.Contains(t, out, "CHAINER-SENTINEL-BODY")
	require.Equal(t, 1, stats.Chunks)

	// The request carried the run's compile identity (with epoch), so the
	// cache can never serve this compile across runs.
	require.Equal(t, uint64(1), invoker.captured.EpochID)
	require.Equal(t, env.AssemblyMetadataSnapshot().RunID, invoker.captured.RunID)
	require.NotEmpty(t, invoker.captured.RunID)
}

// TestChainerWorkingStateBounded proves working state holds the bounded
// summary only — never the compiled bodies (the checkpoint InlineRaw leak
// class).
func TestChainerWorkingStateBounded(t *testing.T) {
	agent, env, _, ctx := chainerStreamFixture(t)

	require.NoError(t, agent.executeStreamingTrigger(ctx, &execution.Task{ID: "task-1"}, env))

	snapshot := env.WorkingDataSnapshot()
	raw, err := json.Marshal(snapshot["chainer.stream.result"])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "CHAINER-SENTINEL-BODY",
		"compiled bodies must never enter working state")

	summary, ok := snapshot["chainer.stream.result"].(map[string]any)
	require.True(t, ok, "the summary is a bounded map, not a raw result")
	require.Equal(t, 1, summary["chunks"])
}

// TestChainerQueryStillRecorded pins the query entry the chain runner reads
// for diagnostics.
func TestChainerQueryStillRecorded(t *testing.T) {
	agent, env, _, ctx := chainerStreamFixture(t)

	require.NoError(t, agent.executeStreamingTrigger(ctx, &execution.Task{ID: "task-1"}, env))

	require.Equal(t, "sentinel query", env.WorkingDataSnapshot()["chainer.stream.query"])
}

// TestChainerStaleDropMutatesNothingStatewise pins the designed-drop path: a
// stale-dropped compile is reported, the node succeeds, and the summary still
// reflects reality (zero chunks — nothing was applied).
func TestChainerStaleDropMutatesNothingStatewise(t *testing.T) {
	agent, env, invoker, ctx := chainerStreamFixture(t)

	// A barrier closes WHILE the compile is in flight (the chainer's request
	// was stamped at epoch 1): the envelope's memory order advances to 5, so
	// the epoch-1 compile must drop as stale.
	invoker.onCompile = func() {
		env.UpdateAssemblyMetadata(func(m contextdata.AssemblyMeta) contextdata.AssemblyMeta {
			m.EpochID = 5
			return m
		})
	}

	require.NoError(t, agent.executeStreamingTrigger(ctx, &execution.Task{ID: "task-1"}, env))

	// The dropped compile applied nothing: the envelope holds no slice and its
	// memory order stands at the newer epoch.
	require.Nil(t, env.StreamedSliceSnapshot(), "a stale-dropped compile must not apply a slice")
	require.Equal(t, uint64(5), env.AssemblyMetadataSnapshot().EpochID, "memory order unmoved")
	require.True(t, strings.Contains(env.WorkingDataSnapshot()["chainer.stream.query"].(string), "sentinel"))
}

package envcomposition

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
)

// TestCompilerTriggerAdapterCarriesBodies is FR-2: every compile — the port
// the backward pass crosses — carries the ranked chunk bodies (verbatim, with
// hash, token estimate, trust class), not just ID references.
func TestCompilerTriggerAdapterCarriesBodies(t *testing.T) {
	ctx := context.Background()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(ctx)) })

	runtime, err := BuildKnowledgeRuntime(KnowledgeRuntimeInput{GraphDB: engine})
	require.NoError(t, err)
	t.Cleanup(runtime.Close)

	for id, text := range map[string]string{
		"chunk:alpha": "alpha grounded body text",
		"chunk:beta":  "beta grounded body text",
	} {
		_, err := runtime.KnowledgeStore.Save(ctx, knowledge.KnowledgeChunk{
			ID:            knowledge.ChunkID(id),
			WorkspaceID:   "ws",
			ContentHash:   "hash-" + id,
			TokenEstimate: 16,
			TrustClass:    agentspec.TrustClassBuiltinTrusted,
			Body:          knowledge.ChunkBody{Raw: text},
		})
		require.NoError(t, err)
	}

	result, err := runtime.StreamTrigger.RequestBlocking(ctx, contextstream.Request{
		ID:        "stream.adapter",
		MaxTokens: 1000,
		Query:     retrieval.RetrievalQuery{Text: "grounded body"},
	})
	require.NoError(t, err)
	require.NotNil(t, result.Compilation)
	require.NotEmpty(t, result.Compilation.StreamedChunks, "compile must carry bodies at the port")
	require.Equal(t, len(result.Compilation.StreamedChunks), len(result.Compilation.StreamedRefs), "zip invariant: refs and views are 1:1")

	bodies := map[string]string{}
	for _, chunk := range result.Compilation.StreamedChunks {
		require.NotEmpty(t, chunk.ChunkID)
		require.NotEmpty(t, chunk.ContentHash, "chunk %s must carry its content hash", chunk.ChunkID)
		require.NotEmpty(t, chunk.Body, "chunk %s must carry its body", chunk.ChunkID)
		require.Equal(t, string(agentspec.TrustClassBuiltinTrusted), chunk.TrustClass)
		require.False(t, strings.Contains(chunk.Body, "\x00"))
		bodies[chunk.ChunkID] = chunk.Body
	}
	require.Contains(t, bodies, "chunk:alpha")
	require.Contains(t, bodies, "chunk:beta")
	require.Equal(t, "alpha grounded body text", bodies["chunk:alpha"])
	require.Equal(t, "beta grounded body text", bodies["chunk:beta"])
}

// TestApplyResultLandsSliceThroughTrigger closes adapter→ApplyResult: the
// typed chunks from the real trigger become the envelope slice the renderer
// reads, in compiler rank order.
func TestApplyResultLandsSliceThroughTrigger(t *testing.T) {
	ctx := context.Background()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(ctx)) })

	runtime, err := BuildKnowledgeRuntime(KnowledgeRuntimeInput{GraphDB: engine})
	require.NoError(t, err)
	t.Cleanup(runtime.Close)

	_, err = runtime.KnowledgeStore.Save(ctx, knowledge.KnowledgeChunk{
		ID:          "chunk:solo",
		WorkspaceID: "ws",
		TrustClass:  agentspec.TrustClassBuiltinTrusted,
		Body:        knowledge.ChunkBody{Raw: "the one grounded fact"},
	})
	require.NoError(t, err)

	result, err := runtime.StreamTrigger.RequestBlocking(ctx, contextstream.Request{
		ID:        "stream.apply",
		MaxTokens: 1000,
		Query:     retrieval.RetrievalQuery{Text: "grounded fact"},
	})
	require.NoError(t, err)

	env := contextdata.NewEnvelope("t", "s")
	require.NoError(t, contextstream.ApplyResult(ctx, env, result, 1))

	section, stats, err := contextstream.RenderStreamedSection(env)
	require.NoError(t, err)
	require.Contains(t, section, "the one grounded fact")
	require.Contains(t, section, `v=1`)
	require.Equal(t, len(result.Compilation.StreamedChunks), stats.Chunks)
}

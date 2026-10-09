package envcomposition

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// wiringTelemetry records telemetry events emitted through a composed runtime.
type wiringTelemetry struct {
	mu     sync.Mutex
	counts map[fwtelemetry.EventType]int
}

func newWiringTelemetry() *wiringTelemetry {
	return &wiringTelemetry{counts: make(map[fwtelemetry.EventType]int)}
}

func (w *wiringTelemetry) Emit(ev fwtelemetry.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.counts[ev.Type]++
}

func (w *wiringTelemetry) count(kind fwtelemetry.EventType) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.counts[kind]
}

// TestBuildKnowledgeRuntimeWiresOneBus proves the retriever and compiler both
// react to events published on the runtime's single bus.
func TestBuildKnowledgeRuntimeWiresOneBus(t *testing.T) {
	ctx := context.Background()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(ctx)) })

	runtime, err := BuildKnowledgeRuntime(KnowledgeRuntimeInput{GraphDB: engine})
	require.NoError(t, err)
	t.Cleanup(runtime.Close)

	tel := newWiringTelemetry()
	runtime.Compiler.SetTelemetry(tel)
	runtime.Retriever.SetTelemetry(tel)

	_, err = runtime.KnowledgeStore.Save(ctx, knowledge.KnowledgeChunk{
		ID:          "chunk:alpha",
		WorkspaceID: "ws",
		TrustClass:  agentspec.TrustClassBuiltinTrusted,
		Body:        knowledge.ChunkBody{Raw: "alpha term", Fields: map[string]any{"content": "alpha term"}},
	})
	require.NoError(t, err)

	request := compiler.CompilationRequest{
		Query:     retrieval.RetrievalQuery{Text: "alpha"},
		MaxTokens: 1000,
	}
	_, first, err := runtime.Compiler.Compile(ctx, request)
	require.NoError(t, err)
	require.False(t, first.CacheHit)
	_, second, err := runtime.Compiler.Compile(ctx, request)
	require.NoError(t, err)
	require.True(t, second.CacheHit, "second identical compile must hit the cache")

	// One bus event must reach the compiler's invalidation consumer.
	runtime.KnowledgeEvents.EmitChunkIngested(knowledge.ChunkIngestedPayload{ChunkID: "chunk:alpha"})
	require.Eventually(t, func() bool {
		return tel.count(fwtelemetry.EventType("compilation_cache_invalidated")) >= 1
	}, 2*time.Second, time.Millisecond, "compiler must consume the runtime bus")

	_, third, err := runtime.Compiler.Compile(ctx, request)
	require.NoError(t, err)
	require.False(t, third.CacheHit, "invalidated compilation must miss the cache")

	// A chunk saved after the snapshot is visible only once the retriever's
	// subscription on the same bus fires.
	_, err = runtime.KnowledgeStore.Save(ctx, knowledge.KnowledgeChunk{
		ID:          "chunk:beta",
		WorkspaceID: "ws",
		TrustClass:  agentspec.TrustClassBuiltinTrusted,
		Body:        knowledge.ChunkBody{Raw: "beta term", Fields: map[string]any{"content": "beta term"}},
	})
	require.NoError(t, err)
	runtime.KnowledgeEvents.EmitChunkIngested(knowledge.ChunkIngestedPayload{ChunkID: "chunk:beta"})

	require.Eventually(t, func() bool {
		result, err := runtime.Retriever.Retrieve(ctx, retrieval.RetrievalQuery{Text: "beta"})
		if err != nil || result == nil {
			return false
		}
		for _, ranked := range result.Ranked {
			if ranked.ChunkID == "chunk:beta" {
				return true
			}
		}
		return false
	}, 2*time.Second, time.Millisecond, "retriever must consume the runtime bus")
}

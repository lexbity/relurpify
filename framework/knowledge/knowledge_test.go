package knowledge

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	return &ChunkStore{Graph: engine}
}

func TestChunkStoreSaveLoadAndEdge(t *testing.T) {
	store := newTestStore(t)
	saved, err := store.Save(KnowledgeChunk{
		ID:          ChunkID("chunk:test:1"),
		WorkspaceID: "ws-1",
		Provenance: ChunkProvenance{
			Sources:    []ProvenanceSource{{Kind: "pattern", Ref: "pattern-1"}},
			CompiledBy: CompilerDeterministic,
			Timestamp:  time.Now().UTC(),
		},
		Body: ChunkBody{Raw: `{"title":"test"}`, Fields: map[string]any{"title": "test"}},
	})
	require.NoError(t, err)
	require.Equal(t, FreshnessValid, saved.Freshness)

	loaded, ok, err := store.Load(saved.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, saved.ID, loaded.ID)

	edge, err := store.SaveEdge(ChunkEdge{
		FromChunk: saved.ID,
		ToChunk:   ChunkID("chunk:test:2"),
		Kind:      EdgeKindRequiresContext,
		Weight:    0.8,
	})
	require.NoError(t, err)
	require.NotEmpty(t, edge.ID)

	edges, err := store.LoadEdgesFrom(saved.ID, EdgeKindRequiresContext)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.Equal(t, edge.ID, edges[0].ID)
}

func TestChunkGraphTraversalAndAmplification(t *testing.T) {
	store := newTestStore(t)
	for _, chunk := range []KnowledgeChunk{
		{ID: ChunkID("chunk:root"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "root"}},
		{ID: ChunkID("chunk:dep"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "dep"}},
		{ID: ChunkID("chunk:amp"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "amp"}},
	} {
		_, err := store.Save(chunk)
		require.NoError(t, err)
	}
	_, err := store.SaveEdge(ChunkEdge{FromChunk: ChunkID("chunk:root"), ToChunk: ChunkID("chunk:dep"), Kind: EdgeKindRequiresContext, Weight: 1})
	require.NoError(t, err)
	_, err = store.SaveEdge(ChunkEdge{FromChunk: ChunkID("chunk:root"), ToChunk: ChunkID("chunk:amp"), Kind: EdgeKindAmplifies, Weight: 0.9})
	require.NoError(t, err)

	graph := &ChunkGraph{Store: store}
	ordered, err := graph.OrderRequiresContext([]ChunkID{"chunk:root"})
	require.NoError(t, err)
	require.NotEmpty(t, ordered)
	require.Equal(t, ChunkID("chunk:dep"), ordered[0].ID)

	amp, err := graph.AmplifyFrom([]ChunkID{"chunk:root"}, 2)
	require.NoError(t, err)
	require.Len(t, amp, 1)
	require.Equal(t, ChunkID("chunk:amp"), amp[0].ID)
}

func TestStreamerSkipsStaleChunks(t *testing.T) {
	store := newTestStore(t)
	for _, chunk := range []KnowledgeChunk{
		{ID: ChunkID("chunk:seed"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "seed"}},
		{ID: ChunkID("chunk:stale"), WorkspaceID: "ws", Freshness: FreshnessStale, Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "stale"}},
	} {
		_, err := store.Save(chunk)
		require.NoError(t, err)
	}
	_, err := store.SaveEdge(ChunkEdge{FromChunk: ChunkID("chunk:seed"), ToChunk: ChunkID("chunk:stale"), Kind: EdgeKindRequiresContext, Weight: 1})
	require.NoError(t, err)

	streamer := &Streamer{Store: store}
	result, err := streamer.Stream(context.Background(), StreamSeed{ChunkIDs: []ChunkID{"chunk:seed"}}, 100)
	require.NoError(t, err)
	require.Len(t, result.Chunks, 1)
	require.Len(t, result.StaleDuringStream, 1)
	require.Equal(t, ChunkID("chunk:stale"), result.StaleDuringStream[0])
}

func TestStalenessManagerPropagatesInvalidation(t *testing.T) {
	store := newTestStore(t)
	for _, chunk := range []KnowledgeChunk{
		{ID: ChunkID("chunk:root"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "root"}},
		{ID: ChunkID("chunk:child"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "child"}},
	} {
		_, err := store.Save(chunk)
		require.NoError(t, err)
	}
	_, err := store.SaveEdge(ChunkEdge{FromChunk: ChunkID("chunk:root"), ToChunk: ChunkID("chunk:child"), Kind: EdgeKindInvalidates, Weight: 1})
	require.NoError(t, err)

	manager := &StalenessManager{Store: store, Propagate: true, MaxDepth: 3}
	require.NoError(t, manager.MarkStale(ChunkID("chunk:root")))

	root, ok, err := store.Load(ChunkID("chunk:root"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessStale, root.Freshness)

	child, ok, err := store.Load(ChunkID("chunk:child"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessStale, child.Freshness)
}

func TestEventBusAndViews(t *testing.T) {
	bus := &EventBus{}
	ch, unsub := bus.Subscribe(1)
	defer unsub()
	bus.EmitBootstrapComplete(BootstrapCompletePayload{WorkspaceRoot: "/tmp/work", IndexedFiles: 3})
	select {
	case event := <-ch:
		require.Equal(t, EventBootstrapComplete, event.Kind)
	default:
		t.Fatal("expected event to be delivered")
	}

	registry := &ViewRendererRegistry{}
	registry.Register(ViewKindPattern, func(chunk KnowledgeChunk) (ChunkView, bool) {
		return ChunkView{Kind: ViewKindPattern, Data: chunk.ID}, true
	})
	views := registry.RenderViews(KnowledgeChunk{ID: ChunkID("chunk:view")}, ViewKindPattern)
	require.Len(t, views, 1)
	require.Equal(t, ChunkID("chunk:view"), views[0].Data)
}

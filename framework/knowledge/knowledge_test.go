package knowledge

import (
	"context"
	"fmt"
	"sync/atomic"
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

func TestChunkStoreFindByCoverageHashAndFilePath(t *testing.T) {
	store := newTestStore(t)
	for i := 0; i < 5; i++ {
		hash := "hash-a"
		if i%2 == 1 {
			hash = "hash-b"
		}
		path := "src/a.go"
		if i >= 3 {
			path = "src/b.go"
		}
		_, err := store.Save(KnowledgeChunk{
			ID:           ChunkID(fmt.Sprintf("chunk:%d", i)),
			WorkspaceID:  "ws",
			CoverageHash: hash,
			Provenance: ChunkProvenance{
				CompiledBy: CompilerDeterministic,
				Timestamp:  time.Now().UTC(),
			},
			Body: ChunkBody{
				Raw: fmt.Sprintf("chunk-%d", i),
				Fields: map[string]any{
					"file_path": path,
				},
			},
		})
		require.NoError(t, err)
	}

	coverage, err := store.FindByCoverageHash("hash-a")
	require.NoError(t, err)
	require.Len(t, coverage, 3)

	filePath, err := store.FindByFilePath("src/a.go")
	require.NoError(t, err)
	require.Len(t, filePath, 3)

	prefix, err := store.FindByFilePathPrefix("src")
	require.NoError(t, err)
	require.Len(t, prefix, 5)

	fresh, err := store.FindFreshByFilePath("src/a.go")
	require.NoError(t, err)
	require.Len(t, fresh, 3)
}

func TestChunkStoreSaveAddsIndexLabels(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Save(KnowledgeChunk{
		ID:           ChunkID("chunk:indexed"),
		WorkspaceID:  "ws",
		CoverageHash: "hash-1",
		Provenance: ChunkProvenance{
			CompiledBy: CompilerDeterministic,
			Timestamp:  time.Now().UTC(),
		},
		Body: ChunkBody{
			Raw: "indexed",
			Fields: map[string]any{
				"file_path": "src/indexed.go",
			},
		},
	})
	require.NoError(t, err)

	loaded, ok, err := store.Load(ChunkID("chunk:indexed"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "src/indexed.go", loaded.Body.Fields["file_path"])

	require.Len(t, store.Graph.ListNodesByLabel(nodeKindChunk, "coverage_hash:hash-1"), 1)
	require.Len(t, store.Graph.ListNodesByLabel(nodeKindChunk, "file_path:src/indexed.go"), 1)
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

func TestStalenessManagerDirectMarkIsLocal(t *testing.T) {
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
	marked, err := manager.MarkOneSync(ChunkID("chunk:root"), FreshnessStale)
	require.NoError(t, err)
	require.Equal(t, []ChunkID{ChunkID("chunk:root")}, marked)

	root, ok, err := store.Load(ChunkID("chunk:root"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessStale, root.Freshness)

	child, ok, err := store.Load(ChunkID("chunk:child"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessValid, child.Freshness)
}

func TestStalenessManagerPropagateSyncFollowsInvalidatesAndDerivedFrom(t *testing.T) {
	store := newTestStore(t)
	for _, chunk := range []KnowledgeChunk{
		{ID: ChunkID("chunk:root"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "root"}},
		{ID: ChunkID("chunk:child"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "child"}},
		{ID: ChunkID("chunk:grand"), WorkspaceID: "ws", Provenance: ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()}, Body: ChunkBody{Raw: "grand"}},
	} {
		_, err := store.Save(chunk)
		require.NoError(t, err)
	}
	_, err := store.SaveEdge(ChunkEdge{FromChunk: ChunkID("chunk:root"), ToChunk: ChunkID("chunk:child"), Kind: EdgeKindInvalidates, Weight: 1})
	require.NoError(t, err)
	_, err = store.SaveEdge(ChunkEdge{FromChunk: ChunkID("chunk:child"), ToChunk: ChunkID("chunk:grand"), Kind: EdgeKindDerivesFrom, Weight: 1})
	require.NoError(t, err)

	manager := &StalenessManager{Store: store, MaxDepth: 3}
	_, err = manager.MarkOneSync(ChunkID("chunk:root"), FreshnessStale)
	require.NoError(t, err)

	propagated, err := manager.PropagateSync([]ChunkID{ChunkID("chunk:root")}, 0)
	require.NoError(t, err)
	require.ElementsMatch(t, []ChunkID{"chunk:child", "chunk:grand"}, propagated)

	child, ok, err := store.Load(ChunkID("chunk:child"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessStale, child.Freshness)

	grand, ok, err := store.Load(ChunkID("chunk:grand"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessStale, grand.Freshness)
}

func TestInvalidationPassHandleRevisionChangedIsNonBlocking(t *testing.T) {
	store := newTestStore(t)
	for i := 0; i < 10; i++ {
		id := ChunkID(fmt.Sprintf("chunk:%d", i))
		_, err := store.Save(KnowledgeChunk{
			ID:          id,
			WorkspaceID: "ws",
			Provenance:  ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()},
			Body:        ChunkBody{Raw: "body", Fields: map[string]any{"file_path": fmt.Sprintf("src/%d.go", i)}},
		})
		require.NoError(t, err)
		if i > 0 {
			_, err := store.SaveEdge(ChunkEdge{FromChunk: ChunkID(fmt.Sprintf("chunk:%d", i-1)), ToChunk: id, Kind: EdgeKindInvalidates, Weight: 1})
			require.NoError(t, err)
		}
	}
	pass := &InvalidationPass{Store: store, Events: &EventBus{}}
	start := time.Now()
	require.NoError(t, pass.HandleRevisionChanged(context.Background(), CodeRevisionChangedPayload{
		AffectedPaths: []string{"src/0.go"},
		NewRevision:   "rev-2",
	}))
	require.Less(t, time.Since(start), 5*time.Millisecond)

	child, ok, err := store.Load(ChunkID("chunk:1"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, FreshnessValid, child.Freshness)
}

func TestInvalidationPassCoalescesChunkStaledEvents(t *testing.T) {
	store := newTestStore(t)
	for i := 0; i < 3; i++ {
		_, err := store.Save(KnowledgeChunk{
			ID:          ChunkID(fmt.Sprintf("chunk:%d", i)),
			WorkspaceID: "ws",
			Provenance:  ChunkProvenance{CompiledBy: CompilerDeterministic, Timestamp: time.Now().UTC()},
			Body:        ChunkBody{Raw: "body"},
		})
		require.NoError(t, err)
		if i > 0 {
			_, err := store.SaveEdge(ChunkEdge{FromChunk: ChunkID("chunk:0"), ToChunk: ChunkID(fmt.Sprintf("chunk:%d", i)), Kind: EdgeKindInvalidates, Weight: 1})
			require.NoError(t, err)
		}
	}
	bus := &EventBus{}
	pass := &InvalidationPass{
		Store:     store,
		Staleness: &StalenessManager{Store: store, MaxDepth: 3},
		Events:    bus,
	}
	var propagateCalls int32
	pass.Staleness.propagateHook = func(ids []ChunkID, depth int) {
		atomic.AddInt32(&propagateCalls, 1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- pass.Start(ctx) }()

	for i := 0; i < 20; i++ {
		bus.EmitChunkStaled(ChunkStaledPayload{ChunkIDs: []string{fmt.Sprintf("chunk:%d", i%3)}, Reason: "burst"})
	}
	time.Sleep(150 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	require.LessOrEqual(t, atomic.LoadInt32(&propagateCalls), int32(2))
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

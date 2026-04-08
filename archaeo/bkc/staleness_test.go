package bkc

import "testing"

func TestStalenessManagerMarkStale(t *testing.T) {
	store := newTestChunkStore(t)
	manager := &StalenessManager{Store: store}
	chunk := testChunk("stale-1", "ws", "rev")
	_, _ = store.Save(chunk)
	if err := manager.MarkStale(chunk.ID); err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	got, ok, err := store.Load(chunk.ID)
	if err != nil || !ok {
		t.Fatalf("load stale chunk: %v ok=%v", err, ok)
	}
	if got.Freshness != FreshnessStale {
		t.Fatalf("expected stale freshness, got %s", got.Freshness)
	}
}

func TestStalenessManagerPropagatesInvalidates(t *testing.T) {
	store := newTestChunkStore(t)
	manager := &StalenessManager{Store: store, Propagate: true, MaxDepth: 3}
	a := testChunk("stale-a", "ws", "rev")
	b := testChunk("stale-b", "ws", "rev")
	_, _ = store.Save(a)
	_, _ = store.Save(b)
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: a.ID, ToChunk: b.ID, Kind: EdgeKindInvalidates})
	if err := manager.MarkStale(a.ID); err != nil {
		t.Fatalf("mark stale with propagation: %v", err)
	}
	got, ok, err := store.Load(b.ID)
	if err != nil || !ok {
		t.Fatalf("load propagated chunk: %v ok=%v", err, ok)
	}
	if got.Freshness != FreshnessStale {
		t.Fatalf("expected propagated stale freshness, got %s", got.Freshness)
	}
}

func TestStalenessManagerPropagationDepthLimit(t *testing.T) {
	store := newTestChunkStore(t)
	manager := &StalenessManager{Store: store, Propagate: true, MaxDepth: 1}
	for _, id := range []ChunkID{"depth-a", "depth-b", "depth-c"} {
		_, _ = store.Save(testChunk(string(id), "ws", "rev"))
	}
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "depth-a", ToChunk: "depth-b", Kind: EdgeKindInvalidates})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "depth-b", ToChunk: "depth-c", Kind: EdgeKindInvalidates})
	if err := manager.MarkStale("depth-a"); err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	b, _, _ := store.Load("depth-b")
	c, _, _ := store.Load("depth-c")
	if b.Freshness != FreshnessStale {
		t.Fatalf("expected depth-b to be stale, got %s", b.Freshness)
	}
	if c.Freshness == FreshnessStale {
		t.Fatalf("expected depth-c to remain non-stale due to depth limit")
	}
}

func TestStalenessManagerBulkMarkStale(t *testing.T) {
	store := newTestChunkStore(t)
	manager := &StalenessManager{Store: store}
	for _, id := range []ChunkID{"bulk-a", "bulk-b", "bulk-c"} {
		_, _ = store.Save(testChunk(string(id), "ws", "rev"))
	}
	if err := manager.BulkMarkStale([]ChunkID{"bulk-a", "bulk-b", "bulk-c"}); err != nil {
		t.Fatalf("bulk mark stale: %v", err)
	}
	for _, id := range []ChunkID{"bulk-a", "bulk-b", "bulk-c"} {
		chunk, ok, err := store.Load(id)
		if err != nil || !ok {
			t.Fatalf("load %s: %v ok=%v", id, err, ok)
		}
		if chunk.Freshness != FreshnessStale {
			t.Fatalf("expected %s to be stale, got %s", id, chunk.Freshness)
		}
	}
}

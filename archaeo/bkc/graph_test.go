package bkc

import "testing"

func TestChunkGraphExtractRequiresContextSubgraph(t *testing.T) {
	store := newTestChunkStore(t)
	graph := &ChunkGraph{Store: store}
	a := testChunk("graph-a", "ws", "rev")
	b := testChunk("graph-b", "ws", "rev")
	c := testChunk("graph-c", "ws", "rev")
	_, _ = store.Save(a)
	_, _ = store.Save(b)
	_, _ = store.Save(c)
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: a.ID, ToChunk: b.ID, Kind: EdgeKindRequiresContext, Weight: 1})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: b.ID, ToChunk: c.ID, Kind: EdgeKindRequiresContext, Weight: 1})
	chunks, edges, err := graph.ExtractRequiresContextSubgraph([]ChunkID{a.ID})
	if err != nil {
		t.Fatalf("extract subgraph: %v", err)
	}
	if len(chunks) != 3 || len(edges) != 2 {
		t.Fatalf("unexpected subgraph sizes chunks=%d edges=%d", len(chunks), len(edges))
	}
}

func TestChunkGraphOrderRequiresContextTopological(t *testing.T) {
	store := newTestChunkStore(t)
	graph := &ChunkGraph{Store: store}
	ids := []ChunkID{"n1", "n2", "n3", "n4", "n5"}
	for _, id := range ids {
		_, _ = store.Save(testChunk(string(id), "ws", "rev"))
	}
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "n1", ToChunk: "n2", Kind: EdgeKindRequiresContext, Weight: 1})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "n1", ToChunk: "n3", Kind: EdgeKindRequiresContext, Weight: 0.9})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "n2", ToChunk: "n4", Kind: EdgeKindRequiresContext, Weight: 1})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "n3", ToChunk: "n4", Kind: EdgeKindRequiresContext, Weight: 1})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "n4", ToChunk: "n5", Kind: EdgeKindRequiresContext, Weight: 1})
	ordered, err := graph.OrderRequiresContext([]ChunkID{"n1"})
	if err != nil {
		t.Fatalf("order graph: %v", err)
	}
	if len(ordered) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(ordered))
	}
	pos := make(map[ChunkID]int, len(ordered))
	for i, chunk := range ordered {
		pos[chunk.ID] = i
	}
	if !(pos["n5"] < pos["n4"] && pos["n4"] < pos["n2"] && pos["n4"] < pos["n3"] && pos["n2"] < pos["n1"] && pos["n3"] < pos["n1"]) {
		t.Fatalf("dependency order not respected: %+v", pos)
	}
}

func TestChunkGraphOrderRequiresContextCycleSafe(t *testing.T) {
	store := newTestChunkStore(t)
	graph := &ChunkGraph{Store: store}
	for _, id := range []ChunkID{"cycle-a", "cycle-b", "cycle-c"} {
		_, _ = store.Save(testChunk(string(id), "ws", "rev"))
	}
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "cycle-a", ToChunk: "cycle-b", Kind: EdgeKindRequiresContext})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "cycle-b", ToChunk: "cycle-c", Kind: EdgeKindRequiresContext})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "cycle-c", ToChunk: "cycle-a", Kind: EdgeKindRequiresContext})
	ordered, err := graph.OrderRequiresContext([]ChunkID{"cycle-a"})
	if err != nil {
		t.Fatalf("order graph with cycle: %v", err)
	}
	if len(ordered) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(ordered))
	}
}

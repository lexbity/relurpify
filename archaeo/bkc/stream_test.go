package bkc

import (
	"context"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestStreamerDependencyOrder(t *testing.T) {
	store := newTestChunkStore(t)
	streamer := &Streamer{Store: store, Graph: &ChunkGraph{Store: store}}
	for _, chunk := range []KnowledgeChunk{
		withTokens(testChunk("dep-a", "ws", "rev"), 10),
		withTokens(testChunk("dep-b", "ws", "rev"), 10),
		withTokens(testChunk("dep-c", "ws", "rev"), 10),
	} {
		_, _ = store.Save(chunk)
	}
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "dep-a", ToChunk: "dep-b", Kind: EdgeKindRequiresContext, Weight: 1})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "dep-b", ToChunk: "dep-c", Kind: EdgeKindRequiresContext, Weight: 1})
	result, err := streamer.Stream(context.Background(), StreamSeed{ChunkIDs: []ChunkID{"dep-a"}}, 100)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if len(result.Chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(result.Chunks))
	}
	if !(result.Chunks[0].ID == "dep-c" && result.Chunks[1].ID == "dep-b" && result.Chunks[2].ID == "dep-a") {
		t.Fatalf("unexpected order: %+v", result.Chunks)
	}
}

func TestStreamerPlanningSeedAndBudget(t *testing.T) {
	store := newTestChunkStore(t)
	streamer := &Streamer{Store: store, Graph: &ChunkGraph{Store: store}}
	_, _ = store.Save(withTokens(testChunk("plan-root", "ws", "rev"), 60))
	_, _ = store.Save(withTokens(testChunk("plan-dep", "ws", "rev"), 60))
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "plan-root", ToChunk: "plan-dep", Kind: EdgeKindRequiresContext, Weight: 1})
	result, err := streamer.Stream(context.Background(), streamer.PlanningSeed([]string{"plan-root"}), 60)
	if err != nil {
		t.Fatalf("stream planning seed: %v", err)
	}
	if result.TokenTotal > 60 {
		t.Fatalf("budget exceeded: %+v", result)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "plan-dep" {
		t.Fatalf("expected only dependency chunk within budget, got %+v", result.Chunks)
	}
}

func TestStreamerPlanningSeedForVersionedPlan(t *testing.T) {
	streamer := &Streamer{}
	seed := streamer.PlanningSeedForVersion(&archaeodomain.VersionedLivingPlan{
		RootChunkIDs: []string{"chunk-a", "chunk-b"},
	})
	if len(seed.ChunkIDs) != 2 || seed.ChunkIDs[0] != "chunk-a" || seed.ChunkIDs[1] != "chunk-b" {
		t.Fatalf("unexpected planning seed: %+v", seed)
	}
}

func TestStreamerSkipsStaleAndReportsIt(t *testing.T) {
	store := newTestChunkStore(t)
	streamer := &Streamer{Store: store, Graph: &ChunkGraph{Store: store}}
	stale := withTokens(testChunk("stale-seed", "ws", "rev"), 10)
	stale.Freshness = FreshnessStale
	_, _ = store.Save(stale)
	result, err := streamer.Stream(context.Background(), StreamSeed{ChunkIDs: []ChunkID{"stale-seed"}}, 100)
	if err != nil {
		t.Fatalf("stream stale chunk: %v", err)
	}
	if len(result.Chunks) != 0 || len(result.StaleDuringStream) != 1 || result.StaleDuringStream[0] != "stale-seed" {
		t.Fatalf("unexpected stale handling: %+v", result)
	}
}

func TestStreamerLoadsAmplifiesAfterRequiredDeps(t *testing.T) {
	store := newTestChunkStore(t)
	streamer := &Streamer{Store: store, Graph: &ChunkGraph{Store: store}}
	_, _ = store.Save(withTokens(testChunk("seed", "ws", "rev"), 10))
	_, _ = store.Save(withTokens(testChunk("req", "ws", "rev"), 10))
	_, _ = store.Save(withTokens(testChunk("amp", "ws", "rev"), 10))
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "seed", ToChunk: "req", Kind: EdgeKindRequiresContext, Weight: 1})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "seed", ToChunk: "amp", Kind: EdgeKindAmplifies, Weight: 0.5})
	result, err := streamer.Stream(context.Background(), StreamSeed{ChunkIDs: []ChunkID{"seed"}}, 40)
	if err != nil {
		t.Fatalf("stream with amplifies: %v", err)
	}
	if len(result.Chunks) != 3 {
		t.Fatalf("expected required + seed + amplify, got %+v", result.Chunks)
	}
	if result.Chunks[2].ID != "amp" {
		t.Fatalf("expected amplify chunk last, got %+v", result.Chunks)
	}
}

func TestStreamerEmptySeed(t *testing.T) {
	store := newTestChunkStore(t)
	streamer := &Streamer{Store: store}
	result, err := streamer.Stream(context.Background(), StreamSeed{}, 100)
	if err != nil {
		t.Fatalf("stream empty seed: %v", err)
	}
	if len(result.Chunks) != 0 || len(result.StaleDuringStream) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestStreamerCycleSafe(t *testing.T) {
	store := newTestChunkStore(t)
	streamer := &Streamer{Store: store, Graph: &ChunkGraph{Store: store}}
	for _, id := range []ChunkID{"c1", "c2"} {
		_, _ = store.Save(withTokens(testChunk(string(id), "ws", "rev"), 10))
	}
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "c1", ToChunk: "c2", Kind: EdgeKindRequiresContext, Weight: 1})
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: "c2", ToChunk: "c1", Kind: EdgeKindRequiresContext, Weight: 1})
	result, err := streamer.Stream(context.Background(), StreamSeed{ChunkIDs: []ChunkID{"c1"}}, 100)
	if err != nil {
		t.Fatalf("stream cyclic graph: %v", err)
	}
	if len(result.Chunks) != 2 {
		t.Fatalf("expected both chunks, got %+v", result)
	}
}

type chunkLoadingStrategy struct {
	request *contextmgr.ContextRequest
	chunks  []contextmgr.ContextChunk
}

func (s chunkLoadingStrategy) SelectContext(task *core.Task, budget *core.ContextBudget) (*contextmgr.ContextRequest, error) {
	return s.request, nil
}
func (s chunkLoadingStrategy) ShouldCompress(ctx *core.SharedContext) bool { return false }
func (s chunkLoadingStrategy) DetermineDetailLevel(file string, relevance float64) contextmgr.DetailLevel {
	return contextmgr.DetailFull
}
func (s chunkLoadingStrategy) ShouldExpandContext(ctx *core.SharedContext, lastResult *core.Result) bool {
	return false
}
func (s chunkLoadingStrategy) PrioritizeContext(items []core.ContextItem) []core.ContextItem {
	return items
}
func (s chunkLoadingStrategy) LoadChunks(task *core.Task, budget *core.ContextBudget) ([]contextmgr.ContextChunk, error) {
	return append([]contextmgr.ContextChunk(nil), s.chunks...), nil
}

func TestToContextChunks(t *testing.T) {
	chunks := ToContextChunks([]KnowledgeChunk{withTokens(testChunk("ctx-1", "ws", "rev"), 12)})
	if len(chunks) != 1 || chunks[0].ID != "ctx-1" || chunks[0].TokenEstimate != 12 {
		t.Fatalf("unexpected context chunks: %+v", chunks)
	}
}

func withTokens(chunk KnowledgeChunk, tokens int) KnowledgeChunk {
	chunk.TokenEstimate = tokens
	return chunk
}

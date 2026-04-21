package bkc

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/graphdb"
)

func TestChunkStoreSaveLoadRoundTrip(t *testing.T) {
	store := newTestChunkStore(t)
	want := testChunk("chunk-1", "ws-a", "rev-1")
	if _, err := store.Save(want); err != nil {
		t.Fatalf("save chunk: %v", err)
	}
	got, ok, err := store.Load(want.ID)
	if err != nil {
		t.Fatalf("load chunk: %v", err)
	}
	if !ok || got == nil {
		t.Fatal("expected chunk to load")
	}
	if got.ID != want.ID || got.WorkspaceID != want.WorkspaceID || got.Provenance.CodeStateRef != want.Provenance.CodeStateRef {
		t.Fatalf("unexpected chunk: %+v", got)
	}
	if got.Body.Raw != want.Body.Raw || got.TokenEstimate != want.TokenEstimate || got.Freshness != want.Freshness {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestChunkStoreSaveIncrementsVersion(t *testing.T) {
	store := newTestChunkStore(t)
	chunk := testChunk("chunk-2", "ws-a", "rev-2")
	if _, err := store.Save(chunk); err != nil {
		t.Fatalf("first save: %v", err)
	}
	chunk.Body.Raw = "updated"
	if _, err := store.Save(chunk); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, ok, err := store.Load(chunk.ID)
	if err != nil || !ok {
		t.Fatalf("load after second save: %v ok=%v", err, ok)
	}
	if got.Version != 2 {
		t.Fatalf("expected version 2, got %d", got.Version)
	}
}

func TestChunkStoreSaveEdgeAndLoadEdgesFrom(t *testing.T) {
	store := newTestChunkStore(t)
	a := testChunk("chunk-a", "ws-a", "rev-a")
	b := testChunk("chunk-b", "ws-a", "rev-b")
	if _, err := store.Save(a); err != nil {
		t.Fatalf("save a: %v", err)
	}
	if _, err := store.Save(b); err != nil {
		t.Fatalf("save b: %v", err)
	}
	edge := ChunkEdge{
		FromChunk: a.ID,
		ToChunk:   b.ID,
		Kind:      EdgeKindRequiresContext,
		Weight:    0.8,
		Meta:      map[string]any{"reason": "dependency"},
		Provenance: ChunkProvenance{
			WorkflowID: "wf-1",
			CompiledBy: CompilerDeterministic,
			Timestamp:  time.Now().UTC(),
		},
	}
	if _, err := store.SaveEdge(edge); err != nil {
		t.Fatalf("save edge: %v", err)
	}
	edges, err := store.LoadEdgesFrom(a.ID)
	if err != nil {
		t.Fatalf("load edges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Kind != EdgeKindRequiresContext || edges[0].ToChunk != b.ID || math.Abs(edges[0].Weight-0.8) > 0.0001 {
		t.Fatalf("unexpected edge: %+v", edges[0])
	}
	if edges[0].Provenance.WorkflowID != "wf-1" {
		t.Fatalf("expected provenance to round-trip, got %+v", edges[0])
	}
}

func TestChunkStoreFindByCodeStateRef(t *testing.T) {
	store := newTestChunkStore(t)
	for _, chunk := range []KnowledgeChunk{
		testChunk("chunk-1", "ws-a", "rev-match"),
		testChunk("chunk-2", "ws-a", "rev-other"),
		testChunk("chunk-3", "ws-b", "rev-match"),
	} {
		if _, err := store.Save(chunk); err != nil {
			t.Fatalf("save chunk: %v", err)
		}
	}
	got, err := store.FindByCodeStateRef("rev-match")
	if err != nil {
		t.Fatalf("find by code ref: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
}

func TestChunkStoreFindByWorkspace(t *testing.T) {
	store := newTestChunkStore(t)
	for _, chunk := range []KnowledgeChunk{
		testChunk("chunk-1", "ws-a", "rev-1"),
		testChunk("chunk-2", "ws-b", "rev-2"),
		testChunk("chunk-3", "ws-a", "rev-3"),
	} {
		if _, err := store.Save(chunk); err != nil {
			t.Fatalf("save chunk: %v", err)
		}
	}
	got, err := store.FindByWorkspace("ws-a")
	if err != nil {
		t.Fatalf("find by workspace: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	for _, chunk := range got {
		if chunk.WorkspaceID != "ws-a" {
			t.Fatalf("unexpected workspace in result: %+v", chunk)
		}
	}
}

func TestChunkStoreFindAll(t *testing.T) {
	store := newTestChunkStore(t)
	for _, chunk := range []KnowledgeChunk{
		testChunk("chunk-1", "ws-a", "rev-1"),
		testChunk("chunk-2", "ws-b", "rev-2"),
		testChunk("chunk-3", "", "rev-3"),
	} {
		if _, err := store.Save(chunk); err != nil {
			t.Fatalf("save chunk: %v", err)
		}
	}
	got, err := store.FindAll()
	if err != nil {
		t.Fatalf("find all: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
}

func TestChunkStoreDeleteRemovesChunkAndEdges(t *testing.T) {
	store := newTestChunkStore(t)
	a := testChunk("chunk-del-a", "ws-a", "rev-a")
	b := testChunk("chunk-del-b", "ws-a", "rev-b")
	_, _ = store.Save(a)
	_, _ = store.Save(b)
	_, _ = store.SaveEdge(ChunkEdge{FromChunk: a.ID, ToChunk: b.ID, Kind: EdgeKindRequiresContext})
	if err := store.Delete(a.ID); err != nil {
		t.Fatalf("delete chunk: %v", err)
	}
	if _, ok, err := store.Load(a.ID); err != nil || ok {
		t.Fatalf("expected deleted chunk to be absent, ok=%v err=%v", ok, err)
	}
	edges, err := store.LoadEdgesFrom(a.ID)
	if err != nil {
		t.Fatalf("load edges after delete: %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("expected no active edges after delete, got %d", len(edges))
	}
}

func newTestChunkStore(t *testing.T) *ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(t.TempDir(), "graphdb")))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Close()
	})
	return &ChunkStore{Graph: engine}
}

func testChunk(id, workspace, codeRef string) KnowledgeChunk {
	return KnowledgeChunk{
		ID:            ChunkID(id),
		WorkspaceID:   workspace,
		ContentHash:   "hash:" + id,
		TokenEstimate: 42,
		Provenance: ChunkProvenance{
			WorkflowID:   "wf-" + workspace,
			CodeStateRef: codeRef,
			CompiledBy:   CompilerDeterministic,
			Timestamp:    time.Now().UTC(),
		},
		Freshness: FreshnessValid,
		Body: ChunkBody{
			Raw: "body:" + id,
			Fields: map[string]any{
				"title": id,
			},
		},
	}
}

package retrieval

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// buildTestSnapshot builds a corpus snapshot from the store's current state
// for ranker tests.
func buildTestSnapshot(t *testing.T, store *knowledge.ChunkStore) *CorpusSnapshot {
	t.Helper()
	snap, err := buildCorpusSnapshot(store, 1, time.Now())
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	return snap
}

func newRankerTestStore(t *testing.T) *knowledge.ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close(context.Background()) })
	return &knowledge.ChunkStore{Graph: engine}
}

func saveRankerChunk(t *testing.T, store *knowledge.ChunkStore, id, raw string, updatedAt time.Time, trust agentspec.TrustClass, filePath string) {
	t.Helper()
	chunk := knowledge.KnowledgeChunk{
		ID:          knowledge.ChunkID(id),
		WorkspaceID: "ws",
		TrustClass:  trust,
		Provenance:  knowledge.ChunkProvenance{CompiledBy: knowledge.CompilerDeterministic, Timestamp: updatedAt},
		Freshness:   knowledge.FreshnessValid,
		Body: knowledge.ChunkBody{
			Raw: raw,
			Fields: map[string]any{
				"content":   raw,
				"file_path": filepath.ToSlash(filepath.Clean(filePath)),
			},
		},
		CreatedAt: updatedAt,
		UpdatedAt: updatedAt,
	}
	if _, err := store.Save(context.TODO(), chunk); err != nil {
		t.Fatalf("save chunk %s: %v", id, err)
	}
}

// newRankerBenchStore builds a generated corpus for retrieval benchmarks.
func newRankerBenchStore(b *testing.B, chunks int) *knowledge.ChunkStore {
	b.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(b.TempDir()))
	if err != nil {
		b.Fatalf("open graphdb: %v", err)
	}
	b.Cleanup(func() { _ = engine.Close(context.Background()) })
	store := &knowledge.ChunkStore{Graph: engine}
	now := time.Unix(100000, 0)
	ctx := context.Background()
	for i := 0; i < chunks; i++ {
		content := "corpus filler document number filler filler filler"
		if i%100 == 0 {
			content = "needle term appears here once in a hundred"
		}
		if _, err := store.Save(ctx, knowledge.KnowledgeChunk{
			ID:          knowledge.ChunkID(fmt.Sprintf("chunk:%06d", i)),
			WorkspaceID: "bench",
			TrustClass:  agentspec.TrustClassBuiltinTrusted,
			UpdatedAt:   now.Add(time.Duration(i) * time.Second),
			CreatedAt:   now,
			Body:        knowledge.ChunkBody{Raw: content, Fields: map[string]any{"content": content}},
		}); err != nil {
			b.Fatalf("save chunk %d: %v", i, err)
		}
	}
	return store
}

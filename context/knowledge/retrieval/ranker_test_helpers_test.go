package retrieval

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

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

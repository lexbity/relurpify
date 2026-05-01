package retrieval

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/ast"
)

func TestKeywordRanker_BM25Scoring(t *testing.T) {
	store := newRankerTestStore(t)
	now := time.Now().UTC()
	saveRankerChunk(t, store, "chunk:1", "context streaming context streaming context", now, agentspec.TrustClassBuiltinTrusted, "/tmp/a.go")
	saveRankerChunk(t, store, "chunk:2", "context streaming", now, agentspec.TrustClassBuiltinTrusted, "/tmp/b.go")
	saveRankerChunk(t, store, "chunk:3", "unrelated text", now, agentspec.TrustClassBuiltinTrusted, "/tmp/c.go")

	ranker := &KeywordRanker{K1: 1.2, B: 0.75}
	ids, err := ranker.Rank(context.Background(), RetrievalQuery{Text: "context streaming"}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) < 2 {
		t.Fatalf("expected at least 2 ids, got %d", len(ids))
	}
	if ids[0] != "chunk:1" {
		t.Fatalf("expected repeated-term chunk first, got %s", ids[0])
	}
}

func TestKeywordRanker_EmptyQuery(t *testing.T) {
	store := newRankerTestStore(t)
	ranker := &KeywordRanker{}
	ids, err := ranker.Rank(context.Background(), RetrievalQuery{Text: ""}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty ranking for empty query, got %d", len(ids))
	}
}

func TestRecencyRanker_HalfLife(t *testing.T) {
	store := newRankerTestStore(t)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	saveRankerChunk(t, store, "chunk:old", "old", now.Add(-24*time.Hour), agentspec.TrustClassBuiltinTrusted, "/tmp/old.go")
	saveRankerChunk(t, store, "chunk:mid", "mid", now.Add(-1*time.Hour), agentspec.TrustClassBuiltinTrusted, "/tmp/mid.go")
	saveRankerChunk(t, store, "chunk:new", "new", now.Add(-time.Minute), agentspec.TrustClassBuiltinTrusted, "/tmp/new.go")

	ranker := &RecencyRanker{HalfLifeHours: 24.0, Now: func() time.Time { return now }}
	ids, err := ranker.Rank(context.Background(), RetrievalQuery{}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(ids))
	}
	if ids[0] != "chunk:new" || ids[1] != "chunk:mid" || ids[2] != "chunk:old" {
		t.Fatalf("unexpected recency order: %v", ids)
	}
}

func TestASTProximityRanker_SameFile(t *testing.T) {
	workspace := t.TempDir()
	store, err := ast.NewSQLiteStore(filepath.Join(workspace, "ast.db"))
	if err != nil {
		t.Fatalf("open ast store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: workspace})

	now := time.Now().UTC()
	activeFile := filepath.ToSlash(filepath.Join(workspace, "active.go"))
	otherFile := filepath.ToSlash(filepath.Join(workspace, "pkg", "other.go"))
	unrelatedFile := filepath.ToSlash(filepath.Join(workspace, "vendor", "x.go"))

	files := []*ast.FileMetadata{
		{ID: activeFile, Path: activeFile, RelativePath: "active.go", Language: "go", Category: ast.CategoryCode, IndexedAt: now},
		{ID: otherFile, Path: otherFile, RelativePath: "pkg/other.go", Language: "go", Category: ast.CategoryCode, IndexedAt: now},
		{ID: unrelatedFile, Path: unrelatedFile, RelativePath: "vendor/x.go", Language: "go", Category: ast.CategoryCode, IndexedAt: now},
	}
	for _, file := range files {
		if err := store.SaveFile(file); err != nil {
			t.Fatalf("save file %s: %v", file.Path, err)
		}
	}

	nodes := []*ast.Node{
		{ID: "node:active", FileID: activeFile, Name: "Active", Type: ast.NodeTypeFunction, Category: ast.CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
		{ID: "node:other", FileID: otherFile, Name: "Other", Type: ast.NodeTypeFunction, Category: ast.CategoryCode, Language: "go", CreatedAt: now, UpdatedAt: now},
	}
	if err := store.SaveNodes(nodes); err != nil {
		t.Fatalf("save nodes: %v", err)
	}

	chunkStore := newRankerTestStore(t)
	saveRankerChunk(t, chunkStore, "chunk:active", "same file", now, agentspec.TrustClassBuiltinTrusted, activeFile)
	saveRankerChunk(t, chunkStore, "chunk:other", "same dir", now, agentspec.TrustClassBuiltinTrusted, otherFile)
	saveRankerChunk(t, chunkStore, "chunk:unrelated", "unrelated", now, agentspec.TrustClassBuiltinTrusted, unrelatedFile)

	ranker := &ASTProximityRanker{Index: manager}
	ids, err := ranker.Rank(context.Background(), RetrievalQuery{Scope: activeFile}, chunkStore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(ids))
	}
	if ids[0] != "chunk:active" {
		t.Fatalf("expected same-file chunk first, got %s", ids[0])
	}
}

func TestASTProximityRanker_NilIndex(t *testing.T) {
	store := newRankerTestStore(t)
	ranker := &ASTProximityRanker{}
	ids, err := ranker.Rank(context.Background(), RetrievalQuery{Scope: "/tmp/x.go"}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no ids with nil index, got %d", len(ids))
	}
}

func TestTrustRanker_MultiplierApplied(t *testing.T) {
	store := newRankerTestStore(t)
	now := time.Now().UTC()
	saveRankerChunk(t, store, "chunk:builtin", "builtin", now, agentspec.TrustClassBuiltinTrusted, "/tmp/a.go")
	saveRankerChunk(t, store, "chunk:tool", "tool", now, agentspec.TrustClassToolResult, "/tmp/b.go")
	saveRankerChunk(t, store, "chunk:llm", "llm", now, agentspec.TrustClassLLMGenerated, "/tmp/c.go")
	saveRankerChunk(t, store, "chunk:unknown", "unknown", now, "", "/tmp/d.go")

	ranker := &TrustRanker{}
	ids, err := ranker.Rank(context.Background(), RetrievalQuery{}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 4 {
		t.Fatalf("expected 4 ids, got %d", len(ids))
	}
	if ids[0] != "chunk:builtin" || ids[1] != "chunk:tool" || ids[2] != "chunk:llm" || ids[3] != "chunk:unknown" {
		t.Fatalf("unexpected trust order: %v", ids)
	}
}

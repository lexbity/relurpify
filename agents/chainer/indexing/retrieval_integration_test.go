package indexing_test

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/chainer/indexing"
)

func TestRetrieveFromLoadedSnapshot_BySymbol(t *testing.T) {
	store := indexing.NewPersistedCodeIndexStore(t.TempDir())
	snapshot := &indexing.CodeIndexSnapshot{
		SnapshotID: "snap-symbol",
		TaskID:     "task-1",
		Timestamp:  time.Now(),
		Snippets: []*indexing.IndexedCodeSnippet{
			{ID: "snippet-1", FilePath: "/tmp/foo.go", Symbols: []string{"FooBar"}},
			{ID: "snippet-2", FilePath: "/tmp/bar.go", Symbols: []string{"Other"}},
		},
	}
	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	loaded, err := store.LoadSnapshot("task-1", "snap-symbol")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	index := indexing.NewCodeIndex()
	for _, snippet := range loaded.Snippets {
		if err := index.Index(snippet); err != nil {
			t.Fatalf("Index: %v", err)
		}
	}

	results := indexing.NewRetriever(index).Retrieve(&indexing.RetrievalQuery{
		SymbolNames: []string{"FooBar"},
	})
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].ID != "snippet-1" {
		t.Fatalf("result ID = %q, want snippet-1", results[0].ID)
	}
}

func TestRetrieveFromLoadedSnapshot_ByPath(t *testing.T) {
	store := indexing.NewPersistedCodeIndexStore(t.TempDir())
	snapshot := &indexing.CodeIndexSnapshot{
		SnapshotID: "snap-path",
		TaskID:     "task-1",
		Timestamp:  time.Now(),
		Snippets: []*indexing.IndexedCodeSnippet{
			{ID: "snippet-1", FilePath: "/tmp/foo.go", Symbols: []string{"FooBar"}},
			{ID: "snippet-2", FilePath: "/tmp/bar.go", Symbols: []string{"Other"}},
		},
	}
	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	loaded, err := store.LoadSnapshot("task-1", "snap-path")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	index := indexing.NewCodeIndex()
	for _, snippet := range loaded.Snippets {
		if err := index.Index(snippet); err != nil {
			t.Fatalf("Index: %v", err)
		}
	}

	results := indexing.NewRetriever(index).Retrieve(&indexing.RetrievalQuery{
		FilePath: "/tmp/bar.go",
	})
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].ID != "snippet-2" {
		t.Fatalf("result ID = %q, want snippet-2", results[0].ID)
	}
}

func TestRetrieveFromLoadedSnapshot_EmptyResult(t *testing.T) {
	store := indexing.NewPersistedCodeIndexStore(t.TempDir())
	snapshot := &indexing.CodeIndexSnapshot{
		SnapshotID: "snap-empty",
		TaskID:     "task-1",
		Timestamp:  time.Now(),
		Snippets: []*indexing.IndexedCodeSnippet{
			{ID: "snippet-1", FilePath: "/tmp/foo.go", Symbols: []string{"FooBar"}},
		},
	}
	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	loaded, err := store.LoadSnapshot("task-1", "snap-empty")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	index := indexing.NewCodeIndex()
	for _, snippet := range loaded.Snippets {
		if err := index.Index(snippet); err != nil {
			t.Fatalf("Index: %v", err)
		}
	}

	results := indexing.NewRetriever(index).Retrieve(&indexing.RetrievalQuery{
		SymbolNames: []string{"Missing"},
	})
	if results == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

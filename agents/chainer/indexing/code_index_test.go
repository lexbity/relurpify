package indexing_test

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/chainer/indexing"
)

func TestCodeIndex_Index(t *testing.T) {
	idx := indexing.NewCodeIndex()

	snippet := &indexing.IndexedCodeSnippet{
		ID:       "snippet-1",
		TaskID:   "task-1",
		LinkName: "link-1",
		Source:   "func foo() {}",
		Language: "go",
		Symbols:  []string{"foo"},
	}

	err := idx.Index(snippet)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	if idx.Count() != 1 {
		t.Errorf("expected count 1, got %d", idx.Count())
	}
}

func TestCodeIndex_IndexNilSnippet(t *testing.T) {
	idx := indexing.NewCodeIndex()

	err := idx.Index(nil)
	if err == nil {
		t.Fatal("expected error for nil snippet")
	}
}

func TestCodeIndex_IndexNoID(t *testing.T) {
	idx := indexing.NewCodeIndex()

	snippet := &indexing.IndexedCodeSnippet{
		TaskID: "task-1",
		Source: "code",
	}

	err := idx.Index(snippet)
	if err == nil {
		t.Fatal("expected error for snippet without ID")
	}
}

func TestCodeIndex_Get(t *testing.T) {
	idx := indexing.NewCodeIndex()

	snippet := &indexing.IndexedCodeSnippet{
		ID:       "snippet-1",
		TaskID:   "task-1",
		LinkName: "link-1",
		Source:   "code",
	}

	idx.Index(snippet)
	retrieved := idx.Get("snippet-1")

	if retrieved == nil {
		t.Fatal("expected snippet")
	}

	if retrieved.Source != "code" {
		t.Errorf("expected 'code', got %q", retrieved.Source)
	}
}

func TestCodeIndex_GetMissing(t *testing.T) {
	idx := indexing.NewCodeIndex()

	retrieved := idx.Get("missing")
	if retrieved != nil {
		t.Fatal("expected nil for missing snippet")
	}
}

func TestCodeIndex_ByTask(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		TaskID: "task-1",
		Source: "code1",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-2",
		TaskID: "task-1",
		Source: "code2",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-3",
		TaskID: "task-2",
		Source: "code3",
	})

	task1 := idx.ByTask("task-1")
	if len(task1) != 2 {
		t.Errorf("expected 2 snippets for task-1, got %d", len(task1))
	}

	task2 := idx.ByTask("task-2")
	if len(task2) != 1 {
		t.Errorf("expected 1 snippet for task-2, got %d", len(task2))
	}
}

func TestCodeIndex_ByLink(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-1",
		LinkName: "link-1",
		Source:   "code1",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-2",
		LinkName: "link-1",
		Source:   "code2",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-3",
		LinkName: "link-2",
		Source:   "code3",
	})

	link1 := idx.ByLink("link-1")
	if len(link1) != 2 {
		t.Errorf("expected 2 snippets for link-1, got %d", len(link1))
	}

	link2 := idx.ByLink("link-2")
	if len(link2) != 1 {
		t.Errorf("expected 1 snippet for link-2, got %d", len(link2))
	}
}

func TestCodeIndex_ByPath(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-1",
		FilePath: "/src/main.go",
		Source:   "code1",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-2",
		FilePath: "/src/main.go",
		Source:   "code2",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-3",
		FilePath: "/src/util.go",
		Source:   "code3",
	})

	main := idx.ByPath("/src/main.go")
	if len(main) != 2 {
		t.Errorf("expected 2 snippets from main.go, got %d", len(main))
	}

	util := idx.ByPath("/src/util.go")
	if len(util) != 1 {
		t.Errorf("expected 1 snippet from util.go, got %d", len(util))
	}
}

func TestCodeIndex_BySymbol(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:      "snippet-1",
		Symbols: []string{"foo", "bar"},
		Source:  "code1",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:      "snippet-2",
		Symbols: []string{"foo", "baz"},
		Source:  "code2",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:      "snippet-3",
		Symbols: []string{"qux"},
		Source:  "code3",
	})

	foo := idx.BySymbol("foo")
	if len(foo) != 2 {
		t.Errorf("expected 2 snippets with foo, got %d", len(foo))
	}

	bar := idx.BySymbol("bar")
	if len(bar) != 1 {
		t.Errorf("expected 1 snippet with bar, got %d", len(bar))
	}

	missing := idx.BySymbol("missing")
	if len(missing) != 0 {
		t.Errorf("expected 0 snippets with missing, got %d", len(missing))
	}
}

func TestCodeIndex_AllSnippets(t *testing.T) {
	idx := indexing.NewCodeIndex()

	for i := 0; i < 3; i++ {
		idx.Index(&indexing.IndexedCodeSnippet{
			ID:     "snippet-" + string(rune(i)),
			Source: "code",
		})
	}

	all := idx.AllSnippets()
	if len(all) != 3 {
		t.Errorf("expected 3 snippets, got %d", len(all))
	}
}

func TestCodeIndex_Count(t *testing.T) {
	idx := indexing.NewCodeIndex()

	if idx.Count() != 0 {
		t.Fatal("new index should have count 0")
	}

	for i := 0; i < 5; i++ {
		idx.Index(&indexing.IndexedCodeSnippet{
			ID:     "snippet-" + string(rune(i)),
			Source: "code",
		})
	}

	if idx.Count() != 5 {
		t.Errorf("expected count 5, got %d", idx.Count())
	}
}

func TestCodeIndex_Clear(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		Source: "code",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-2",
		Source: "code",
	})

	if idx.Count() != 2 {
		t.Fatal("should have 2 snippets before clear")
	}

	idx.Clear()

	if idx.Count() != 0 {
		t.Fatal("should have 0 snippets after clear")
	}
}

func TestCodeIndex_Delete(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:       "snippet-1",
		LinkName: "link-1",
		FilePath: "/src/main.go",
		Symbols:  []string{"foo"},
		Source:   "code1",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-2",
		Source: "code2",
	})

	idx.Delete("snippet-1")

	if idx.Count() != 1 {
		t.Errorf("expected count 1 after delete, got %d", idx.Count())
	}

	if idx.Get("snippet-1") != nil {
		t.Fatal("deleted snippet should be nil")
	}

	if len(idx.ByLink("link-1")) != 0 {
		t.Fatal("deleted snippet should be removed from link index")
	}

	if len(idx.ByPath("/src/main.go")) != 0 {
		t.Fatal("deleted snippet should be removed from path index")
	}

	if len(idx.BySymbol("foo")) != 0 {
		t.Fatal("deleted snippet should be removed from symbol index")
	}
}

func TestCodeIndex_DeleteMissing(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		Source: "code",
	})

	idx.Delete("missing")

	if idx.Count() != 1 {
		t.Fatal("count should remain unchanged")
	}
}

func TestCodeIndex_TimestampTracking(t *testing.T) {
	idx := indexing.NewCodeIndex()

	now := time.Now()
	snippet := &indexing.IndexedCodeSnippet{
		ID:        "snippet-1",
		Source:    "code",
		Timestamp: now,
	}

	idx.Index(snippet)
	retrieved := idx.Get("snippet-1")

	if !retrieved.Timestamp.Equal(now) {
		t.Errorf("timestamp not preserved")
	}
}

func TestCodeIndex_NilIndex(t *testing.T) {
	var idx *indexing.CodeIndex

	err := idx.Index(&indexing.IndexedCodeSnippet{ID: "test", Source: "code"})
	if err == nil {
		t.Fatal("nil index should error on Index")
	}

	if idx.Get("test") != nil {
		t.Fatal("nil index should return nil for Get")
	}

	if idx.ByTask("task") != nil {
		t.Fatal("nil index should return nil for ByTask")
	}

	if idx.ByLink("link") != nil {
		t.Fatal("nil index should return nil for ByLink")
	}

	if idx.ByPath("path") != nil {
		t.Fatal("nil index should return nil for ByPath")
	}

	if idx.BySymbol("sym") != nil {
		t.Fatal("nil index should return nil for BySymbol")
	}

	if idx.AllSnippets() != nil {
		t.Fatal("nil index should return nil for AllSnippets")
	}

	if idx.Count() != 0 {
		t.Fatal("nil index should return 0 for Count")
	}

	// Should not panic
	idx.Clear()
	idx.Delete("test")
}

func TestCodeIndex_Isolation(t *testing.T) {
	// Verify indices are isolated
	idx1 := indexing.NewCodeIndex()
	idx2 := indexing.NewCodeIndex()

	idx1.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		Source: "code1",
	})
	idx2.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		Source: "code2",
	})

	snippet1 := idx1.Get("snippet-1")
	snippet2 := idx2.Get("snippet-1")

	if snippet1.Source != "code1" {
		t.Errorf("idx1 has wrong source: %q", snippet1.Source)
	}

	if snippet2.Source != "code2" {
		t.Errorf("idx2 has wrong source: %q", snippet2.Source)
	}
}

func TestCodeIndex_MultipleSymbols(t *testing.T) {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:      "snippet-1",
		Symbols: []string{"foo", "bar", "baz"},
		Source:  "code",
	})

	// Each symbol should index the same snippet
	if len(idx.BySymbol("foo")) != 1 {
		t.Error("foo not indexed")
	}
	if len(idx.BySymbol("bar")) != 1 {
		t.Error("bar not indexed")
	}
	if len(idx.BySymbol("baz")) != 1 {
		t.Error("baz not indexed")
	}
}

func TestCodeIndex_ErrorCodeTracking(t *testing.T) {
	idx := indexing.NewCodeIndex()

	errSnippet := &indexing.IndexedCodeSnippet{
		ID:           "error-1",
		Source:       "bad code",
		IsError:      true,
		ErrorMessage: "syntax error",
	}

	idx.Index(errSnippet)
	retrieved := idx.Get("error-1")

	if !retrieved.IsError {
		t.Fatal("error flag not preserved")
	}

	if retrieved.ErrorMessage != "syntax error" {
		t.Errorf("error message not preserved: %q", retrieved.ErrorMessage)
	}
}

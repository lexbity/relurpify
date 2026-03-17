package indexing_test

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/chainer/indexing"
)

func setupTestIndex() *indexing.CodeIndex {
	idx := indexing.NewCodeIndex()

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:        "snippet-1",
		TaskID:    "task-1",
		LinkName:  "link-1",
		FilePath:  "/src/main.go",
		Language:  "go",
		Symbols:   []string{"foo", "bar"},
		Timestamp: time.Now(),
	})

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:        "snippet-2",
		TaskID:    "task-1",
		LinkName:  "link-2",
		FilePath:  "/src/util.go",
		Language:  "go",
		Symbols:   []string{"foo", "baz"},
		Timestamp: time.Now().Add(-1 * time.Hour),
	})

	idx.Index(&indexing.IndexedCodeSnippet{
		ID:        "snippet-3",
		TaskID:    "task-2",
		LinkName:  "link-1",
		FilePath:  "/src/main.py",
		Language:  "python",
		Symbols:   []string{"qux"},
		IsError:   true,
		Timestamp: time.Now().Add(-2 * time.Hour),
	})

	return idx
}

func TestRetriever_NewRetriever(t *testing.T) {
	idx := indexing.NewCodeIndex()
	retriever := indexing.NewRetriever(idx)

	if retriever == nil {
		t.Fatal("expected retriever")
	}
}

func TestRetriever_RetrieveBySymbol(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		SymbolNames: []string{"foo"},
	}

	results := retriever.Retrieve(query)
	if len(results) != 2 {
		t.Errorf("expected 2 results with foo, got %d", len(results))
	}
}

func TestRetriever_RetrieveByPath(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		FilePath: "/src/main.go",
	}

	results := retriever.Retrieve(query)
	if len(results) != 1 {
		t.Errorf("expected 1 result from main.go, got %d", len(results))
	}

	if results[0].ID != "snippet-1" {
		t.Errorf("expected snippet-1, got %s", results[0].ID)
	}
}

func TestRetriever_RetrieveByPathPattern(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		FilePathPattern: ".go",
	}

	results := retriever.Retrieve(query)
	if len(results) != 2 {
		t.Errorf("expected 2 .go files, got %d", len(results))
	}
}

func TestRetriever_RetrieveByKeyword(t *testing.T) {
	idx := indexing.NewCodeIndex()
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		Source: "func main() { fmt.Println() }",
	})
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-2",
		Source: "import os",
	})

	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		KeywordPattern: "Println",
	}

	results := retriever.Retrieve(query)
	if len(results) != 1 {
		t.Errorf("expected 1 result with Println, got %d", len(results))
	}
}

func TestRetriever_RetrieveByLink(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		LinkName: "link-1",
	}

	results := retriever.Retrieve(query)
	if len(results) != 2 {
		t.Errorf("expected 2 results from link-1, got %d", len(results))
	}
}

func TestRetriever_RetrieveByTask(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		TaskID: "task-1",
	}

	results := retriever.Retrieve(query)
	if len(results) != 2 {
		t.Errorf("expected 2 results from task-1, got %d", len(results))
	}
}

func TestRetriever_RetrieveByLanguage(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	results := retriever.RetrieveByLanguage("go", 0)
	if len(results) != 2 {
		t.Errorf("expected 2 go snippets, got %d", len(results))
	}
}

func TestRetriever_RetrieveRecent(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	// Get snippets from last 30 minutes (should get newest one)
	results := retriever.RetrieveRecent(30*time.Minute, 0)
	if len(results) != 1 {
		t.Errorf("expected 1 recent snippet, got %d", len(results))
	}

	// Get snippets from last 2 hours (should get first two)
	results = retriever.RetrieveRecent(2*time.Hour, 0)
	if len(results) != 2 {
		t.Errorf("expected 2 snippets from last 2 hours, got %d", len(results))
	}
}

func TestRetriever_RetrieveSimilar(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	// Query for snippets similar to foo symbol
	query := &indexing.IndexedCodeSnippet{
		Symbols:  []string{"foo"},
		Language: "go",
	}

	results := retriever.RetrieveSimilar(query, 0)
	if len(results) != 2 {
		t.Errorf("expected 2 similar snippets, got %d", len(results))
	}
}

func TestRetriever_ExcludeErrors(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		ExcludeErrors: true,
	}

	results := retriever.Retrieve(query)
	if len(results) != 2 {
		t.Errorf("expected 2 non-error snippets, got %d", len(results))
	}

	for _, result := range results {
		if result.IsError {
			t.Fatal("should not include error snippets")
		}
	}
}

func TestRetriever_Limit(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		Limit: 2,
	}

	results := retriever.Retrieve(query)
	if len(results) != 2 {
		t.Errorf("expected 2 results with limit 2, got %d", len(results))
	}
}

func TestRetriever_Offset(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		Offset: 2,
		Limit:  1,
	}

	results := retriever.Retrieve(query)
	if len(results) != 1 {
		t.Errorf("expected 1 result with offset 2, got %d", len(results))
	}
}

func TestRetriever_CombinedFilters(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		SymbolNames:   []string{"foo"},
		Language:      "go",
		ExcludeErrors: true,
	}

	results := retriever.Retrieve(query)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	for _, result := range results {
		if result.Language != "go" {
			t.Error("non-go snippet in results")
		}
		if result.IsError {
			t.Error("error snippet in results")
		}
	}
}

func TestRetriever_Stats(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	stats := retriever.Stats()

	if stats.TotalSnippets != 3 {
		t.Errorf("expected 3 total snippets, got %d", stats.TotalSnippets)
	}

	if stats.SuccessSnippets != 2 {
		t.Errorf("expected 2 success snippets, got %d", stats.SuccessSnippets)
	}

	if stats.ErrorSnippets != 1 {
		t.Errorf("expected 1 error snippet, got %d", stats.ErrorSnippets)
	}

	if stats.UniqueLanguages != 2 {
		t.Errorf("expected 2 languages, got %d", stats.UniqueLanguages)
	}

	if stats.UniquePaths != 3 {
		t.Errorf("expected 3 unique paths, got %d", stats.UniquePaths)
	}

	if stats.UniqueSymbols != 4 {
		t.Errorf("expected 4 unique symbols (foo, bar, baz, qux), got %d", stats.UniqueSymbols)
	}
}

func TestRetriever_StatsByLanguage(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	stats := retriever.Stats()

	if stats.Languages["go"] != 2 {
		t.Errorf("expected 2 go snippets, got %d", stats.Languages["go"])
	}

	if stats.Languages["python"] != 1 {
		t.Errorf("expected 1 python snippet, got %d", stats.Languages["python"])
	}
}

func TestRetriever_Summary(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		SymbolNames: []string{"foo"},
	}

	results := retriever.Retrieve(query)
	summary := retriever.Summary(results, query)

	if summary.ResultCount != 2 {
		t.Errorf("expected 2 results in summary, got %d", summary.ResultCount)
	}

	if len(summary.LanguagesFound) != 1 {
		t.Errorf("expected 1 language in summary, got %d", len(summary.LanguagesFound))
	}
}

func TestRetriever_NilRetriever(t *testing.T) {
	var retriever *indexing.Retriever

	// Should not panic
	results := retriever.Retrieve(&indexing.RetrievalQuery{})
	if results != nil {
		t.Fatal("expected nil for nil retriever")
	}

	results = retriever.RetrieveSimilar(nil, 0)
	if results != nil {
		t.Fatal("expected nil for nil retriever")
	}

	results = retriever.RetrieveByLanguage("go", 0)
	if results != nil {
		t.Fatal("expected nil for nil retriever")
	}

	results = retriever.RetrieveRecent(time.Hour, 0)
	if results != nil {
		t.Fatal("expected nil for nil retriever")
	}

	stats := retriever.Stats()
	if stats == nil {
		t.Fatal("expected empty stats for nil retriever")
	}

	if stats.TotalSnippets != 0 {
		t.Fatal("expected 0 total snippets")
	}
}

func TestRetriever_NilQuery(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	results := retriever.Retrieve(nil)
	if results != nil {
		t.Fatal("expected nil for nil query")
	}
}

func TestRetriever_EmptyQuery(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{}
	results := retriever.Retrieve(query)

	if len(results) != 3 {
		t.Errorf("empty query should return all snippets, got %d", len(results))
	}
}

func TestRetriever_NoResults(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	query := &indexing.RetrievalQuery{
		FilePath: "/nonexistent",
	}

	results := retriever.Retrieve(query)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestRetriever_CaseSensitiveKeyword(t *testing.T) {
	idx := indexing.NewCodeIndex()
	idx.Index(&indexing.IndexedCodeSnippet{
		ID:     "snippet-1",
		Source: "func Main() {}",
	})

	retriever := indexing.NewRetriever(idx)

	// Should be case-insensitive
	query := &indexing.RetrievalQuery{
		KeywordPattern: "main",
	}

	results := retriever.Retrieve(query)
	if len(results) != 1 {
		t.Errorf("keyword search should be case-insensitive, got %d results", len(results))
	}
}

func TestRetriever_MultipleSymbolsAny(t *testing.T) {
	idx := setupTestIndex()
	retriever := indexing.NewRetriever(idx)

	// Query for any of multiple symbols
	query := &indexing.RetrievalQuery{
		SymbolNames: []string{"foo", "qux"},
	}

	results := retriever.Retrieve(query)
	if len(results) != 3 {
		t.Errorf("expected 3 results for foo or qux, got %d", len(results))
	}
}

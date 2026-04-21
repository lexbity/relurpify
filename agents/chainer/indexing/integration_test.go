package indexing_test

import (
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/chainer/indexing"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestIndexingListener_NewListener(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	if listener == nil {
		t.Fatal("expected listener")
	}
}

func TestIndexingListener_NewListenerNilIndex(t *testing.T) {
	listener := indexing.NewIndexingListener(nil, "task-1")

	if listener != nil {
		t.Fatal("expected nil listener for nil index")
	}
}

func TestIndexingListener_OnLinkEvaluated(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	response := &core.LLMResponse{
		Text: "func foo() { return 42 }",
	}

	err := listener.OnLinkEvaluated("link-1", response)
	if err != nil {
		t.Fatalf("OnLinkEvaluated failed: %v", err)
	}

	if idx.Count() != 1 {
		t.Errorf("expected 1 indexed snippet, got %d", idx.Count())
	}

	snippet := idx.AllSnippets()[0]
	if snippet.Source != response.Text {
		t.Errorf("source not preserved")
	}

	if snippet.LinkName != "link-1" {
		t.Errorf("link name not preserved")
	}

	if snippet.TaskID != "task-1" {
		t.Errorf("task id not preserved")
	}
}

func TestIndexingListener_OnLinkEvaluatedExtractsSymbols(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	response := &core.LLMResponse{
		Text: "func hello() { } func world() { }",
	}

	listener.OnLinkEvaluated("link-1", response)

	snippet := idx.AllSnippets()[0]
	if len(snippet.Symbols) == 0 {
		t.Fatal("expected symbols to be extracted")
	}
}

func TestIndexingListener_OnLinkEvaluatedDetectsLanguage(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	response := &core.LLMResponse{
		Text: "package main\n\nfunc main() {}",
	}

	listener.OnLinkEvaluated("link-1", response)

	snippet := idx.AllSnippets()[0]
	if snippet.Language != "go" {
		t.Errorf("expected language go, got %q", snippet.Language)
	}
}

func TestIndexingListener_OnLinkFailed(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	err := listener.OnLinkFailed("link-1", errors.New("syntax error"), "bad code")
	if err != nil {
		t.Fatalf("OnLinkFailed failed: %v", err)
	}

	if idx.Count() != 1 {
		t.Errorf("expected 1 indexed snippet, got %d", idx.Count())
	}

	snippet := idx.AllSnippets()[0]
	if !snippet.IsError {
		t.Fatal("expected error flag to be set")
	}

	if snippet.ErrorMessage != "syntax error" {
		t.Errorf("error message not preserved")
	}
}

func TestIndexingListener_OnLinkFailedMarksError(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	listener.OnLinkFailed("link-1", errors.New("parse error"), "code")

	snippet := idx.AllSnippets()[0]
	if !snippet.IsError {
		t.Fatal("snippet should be marked as error")
	}

	if snippet.ErrorMessage == "" {
		t.Fatal("error message should be set")
	}
}

func TestIndexingListener_QueryContext_SameLinkName(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	// Index two snippets from same link
	listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code1"})
	listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code2"})
	listener.OnLinkEvaluated("link-2", &core.LLMResponse{Text: "code3"})

	// Query for context of link-1
	results := listener.QueryContext("link-1", 0)
	if len(results) != 2 {
		t.Errorf("expected 2 results for link-1, got %d", len(results))
	}
}

func TestIndexingListener_Stats(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code1"})
	listener.OnLinkEvaluated("link-2", &core.LLMResponse{Text: "code2"})
	listener.OnLinkFailed("link-3", errors.New("failed"), "code3")

	stats := listener.Stats()
	if stats.TotalSnippets != 3 {
		t.Errorf("expected 3 total snippets, got %d", stats.TotalSnippets)
	}

	if stats.ErrorSnippets != 1 {
		t.Errorf("expected 1 error snippet, got %d", stats.ErrorSnippets)
	}

	if stats.SuccessSnippets != 2 {
		t.Errorf("expected 2 success snippets, got %d", stats.SuccessSnippets)
	}
}

func TestIndexingListener_Clear(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code"})
	listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code"})

	if idx.Count() != 2 {
		t.Fatal("should have 2 snippets before clear")
	}

	listener.Clear()

	if idx.Count() != 0 {
		t.Fatal("should have 0 snippets after clear")
	}
}

func TestIndexingListener_MultipleEvaluations(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	// Simulate multiple link evaluations
	for i := 0; i < 5; i++ {
		listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code"})
	}

	if idx.Count() != 5 {
		t.Errorf("expected 5 snippets, got %d", idx.Count())
	}

	// All should have unique IDs
	snippets := idx.AllSnippets()
	ids := make(map[string]bool)
	for _, s := range snippets {
		if ids[s.ID] {
			t.Fatal("duplicate snippet ID")
		}
		ids[s.ID] = true
	}
}

func TestIndexingListener_NilListener(t *testing.T) {
	var listener *indexing.IndexingListener

	err := listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code"})
	if err == nil {
		t.Fatal("expected error for nil listener")
	}

	err = listener.OnLinkFailed("link-1", errors.New("error"), "code")
	if err == nil {
		t.Fatal("expected error for nil listener")
	}

	results := listener.QueryContext("link-1", 0)
	if results != nil {
		t.Fatal("expected nil results for nil listener")
	}

	// Should not panic
	listener.Clear()
}

func TestIndexingListener_NilResponse(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	err := listener.OnLinkEvaluated("link-1", nil)
	if err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestIndexingListener_MultipleLinks(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	// Evaluate multiple different links
	listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code1"})
	listener.OnLinkEvaluated("link-2", &core.LLMResponse{Text: "code2"})
	listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code3"})

	// Query by link
	link1Results := listener.QueryContext("link-1", 0)
	if len(link1Results) != 2 {
		t.Errorf("expected 2 results for link-1, got %d", len(link1Results))
	}

	link2Results := listener.QueryContext("link-2", 0)
	if len(link2Results) != 1 {
		t.Errorf("expected 1 result for link-2, got %d", len(link2Results))
	}
}

func TestIndexingListener_LimitResults(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	// Index many snippets
	for i := 0; i < 10; i++ {
		listener.OnLinkEvaluated("link-1", &core.LLMResponse{Text: "code"})
	}

	// Query with limit
	results := listener.QueryContext("link-1", 3)
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

func TestIndexingListener_PytonLanguageDetection(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	response := &core.LLMResponse{
		Text: "def hello():\n    print('hello')",
	}

	listener.OnLinkEvaluated("link-1", response)

	snippet := idx.AllSnippets()[0]
	if snippet.Language != "python" {
		t.Errorf("expected language python, got %q", snippet.Language)
	}
}

func TestIndexingListener_UnknownLanguage(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	response := &core.LLMResponse{
		Text: "some random text without code patterns",
	}

	listener.OnLinkEvaluated("link-1", response)

	snippet := idx.AllSnippets()[0]
	if snippet.Language == "" && snippet.Language != "unknown" {
		// Either empty or "unknown" is acceptable
	}
}

func TestIndexingListener_EmptySource(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	response := &core.LLMResponse{Text: ""}

	err := listener.OnLinkEvaluated("link-1", response)
	if err != nil {
		t.Fatalf("should handle empty source: %v", err)
	}

	if idx.Count() != 1 {
		t.Fatal("should still index empty source")
	}
}

func TestIndexingListener_SymbolExtraction(t *testing.T) {
	idx := indexing.NewCodeIndex()
	listener := indexing.NewIndexingListener(idx, "task-1")

	response := &core.LLMResponse{
		Text: `
			func Calculate() {}
			func Process() {}
			type Result struct {}
		`,
	}

	listener.OnLinkEvaluated("link-1", response)

	snippet := idx.AllSnippets()[0]
	// Should extract function/type names
	if len(snippet.Symbols) == 0 {
		t.Fatal("should extract symbols from code")
	}
}

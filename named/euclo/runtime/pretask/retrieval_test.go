package pretask

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/patterns"
)

type stubExpandedRetriever struct {
	results []KnowledgeEvidenceItem
	err     error
	called  bool
}

func (s *stubExpandedRetriever) RetrieveExpanded(_ context.Context, _ HypotheticalSketch) ([]KnowledgeEvidenceItem, error) {
	s.called = true
	return append([]KnowledgeEvidenceItem(nil), s.results...), s.err
}

func TestIndexRetriever_Retrieve(t *testing.T) {
	retriever := &IndexRetriever{
		index: &dummyIndexQuerier{},
		config: IndexRetrieverConfig{
			DependencyHops:    1,
			MaxFilesPerSymbol: 3,
		},
	}

	anchors := AnchorSet{
		FilePaths:   []string{"test.go"},
		SymbolNames: []string{"TestSymbol"},
	}

	results, err := retriever.Retrieve(context.Background(), anchors)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}

	// Should at least return the file path
	if len(results) == 0 {
		t.Error("Expected at least one result")
	}
}

func TestIndexRetriever_AssignsTrustClass(t *testing.T) {
	retriever := &IndexRetriever{
		index: &dummyIndexQuerier{},
		config: IndexRetrieverConfig{
			DependencyHops:    1,
			MaxFilesPerSymbol: 3,
		},
	}
	anchors := AnchorSet{
		FilePaths:   []string{"user.go"},
		SymbolNames: []string{"SomeSymbol"},
	}
	results, err := retriever.Retrieve(context.Background(), anchors)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	for _, item := range results {
		if item.TrustClass == "" {
			t.Error("Expected TrustClass to be assigned")
		}
		// Check that file paths get workspace-trusted
		if item.Path == "user.go" && item.TrustClass != "workspace-trusted" {
			t.Errorf("Expected workspace-trusted for user.go, got %s", item.TrustClass)
		}
		// Symbol expansion items should be builtin-trusted
		if item.Path != "user.go" && item.TrustClass != "builtin-trusted" {
			t.Errorf("Expected builtin-trusted for expanded items, got %s", item.TrustClass)
		}
	}
}

func TestArchaeoRetriever_RetrieveTopic(t *testing.T) {
	retriever := &ArchaeoRetriever{
		config: ArchaeoRetrieverConfig{
			WorkflowID: "test-workflow",
			MaxItems:   4,
			MaxTokens:  500,
		},
	}

	results, err := retriever.RetrieveTopic(context.Background(), "test query", "test-workflow")
	if err != nil {
		t.Fatalf("RetrieveTopic failed: %v", err)
	}

	// With nil dependencies, should return empty slice
	if results == nil {
		t.Error("Results should not be nil")
	}
}

func TestArchaeoRetriever_RetrieveTopic_UsesPatternStoreQuerier(t *testing.T) {
	ctx := context.Background()
	db, err := patterns.OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := patterns.NewSQLitePatternStore(db)
	if err != nil {
		t.Fatalf("NewSQLitePatternStore: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Nanosecond)
	records := []patterns.PatternRecord{
		{
			ID:           "pattern-1",
			Kind:         patterns.PatternKindStructural,
			Title:        "Workflow pattern 1",
			Description:  "first pattern",
			Status:       patterns.PatternStatusProposed,
			CorpusScope:  "wf-1",
			CorpusSource: "workspace",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "pattern-2",
			Kind:         patterns.PatternKindSemantic,
			Title:        "Workflow pattern 2",
			Description:  "second pattern",
			Status:       patterns.PatternStatusConfirmed,
			CorpusScope:  "wf-1",
			CorpusSource: "workspace",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	for _, record := range records {
		if err := store.Save(ctx, record); err != nil {
			t.Fatalf("Save(%s): %v", record.ID, err)
		}
	}

	retriever := &ArchaeoRetriever{
		patternSvc: &patternStoreQuerier{store: store},
		config: ArchaeoRetrieverConfig{
			WorkflowID: "wf-1",
			MaxItems:   4,
			MaxTokens:  500,
		},
	}

	results, err := retriever.RetrieveTopic(ctx, "test query", "wf-1")
	if err != nil {
		t.Fatalf("RetrieveTopic failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 knowledge items from pattern store, got %d: %#v", len(results), results)
	}
	for _, item := range results {
		if item.Kind != KnowledgeKindPattern {
			t.Fatalf("expected pattern knowledge item, got %#v", item)
		}
	}
}

func TestArchaeoRetriever_EmptyWorkflowIDSkipsRetrieval(t *testing.T) {
	retriever := &ArchaeoRetriever{
		config: ArchaeoRetrieverConfig{
			WorkflowID: "",
			MaxItems:   4,
			MaxTokens:  500,
		},
	}
	results, err := retriever.RetrieveTopic(context.Background(), "query", "")
	if err != nil {
		t.Fatalf("RetrieveTopic failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected empty results with empty workflow ID, got %d", len(results))
	}
}

func TestArchaeoRetriever_KnowledgeItemsTrustClass(t *testing.T) {
	retriever := &ArchaeoRetriever{
		config: ArchaeoRetrieverConfig{
			WorkflowID: "test-workflow",
			MaxItems:   4,
			MaxTokens:  500,
		},
	}
	results, err := retriever.RetrieveTopic(context.Background(), "query", "test-workflow")
	if err != nil {
		t.Fatalf("RetrieveTopic failed: %v", err)
	}
	for _, item := range results {
		if item.TrustClass != "builtin-trusted" {
			t.Errorf("Expected builtin-trusted for archaeo items, got %s", item.TrustClass)
		}
	}
}

func TestArchaeoRetriever_RetrieveExpanded(t *testing.T) {
	retriever := &ArchaeoRetriever{
		config: ArchaeoRetrieverConfig{
			WorkflowID: "test-workflow",
			MaxItems:   4,
			MaxTokens:  500,
		},
	}

	sketch := HypotheticalSketch{
		Text:     "test sketch",
		Grounded: true,
	}

	results, err := retriever.RetrieveExpanded(context.Background(), sketch)
	if err != nil {
		t.Fatalf("RetrieveExpanded failed: %v", err)
	}

	// Should handle gracefully
	if results == nil {
		t.Error("Results should not be nil")
	}
}

func TestArchaeoRetriever_RetrieveExpanded_UsesInjectedRetriever(t *testing.T) {
	custom := &stubExpandedRetriever{
		results: []KnowledgeEvidenceItem{{
			RefID:      "custom-1",
			Kind:       KnowledgeKindDecision,
			Title:      "Custom decision",
			Summary:    "injected retriever result",
			Score:      0.9,
			Source:     EvidenceSourceArchaeoExpanded,
			TrustClass: "builtin-trusted",
		}},
	}
	retriever := &ArchaeoRetriever{
		retriever: custom,
		config: ArchaeoRetrieverConfig{
			WorkflowID: "test-workflow",
			MaxItems:   4,
			MaxTokens:  500,
		},
	}

	results, err := retriever.RetrieveExpanded(context.Background(), HypotheticalSketch{Text: "test sketch", Grounded: true})
	if err != nil {
		t.Fatalf("RetrieveExpanded failed: %v", err)
	}
	if !custom.called {
		t.Fatal("expected injected retriever to be called")
	}
	if len(results) != 1 || results[0].RefID != "custom-1" {
		t.Fatalf("expected injected retriever results, got %#v", results)
	}
}

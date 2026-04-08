package pretask

import (
	"context"
	"testing"
)

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

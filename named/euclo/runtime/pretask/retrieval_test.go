package pretask

import (
	"context"
	"testing"
)

func TestIndexRetriever_Retrieve(t *testing.T) {
	retriever := &IndexRetriever{
		index: &dummyIndexQuerier{},
		config: IndexRetrieverConfig{
			DependencyHops:     1,
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

func TestArchaeoRetriever_RetrieveExpanded(t *testing.T) {
	retriever := &ArchaeoRetriever{
		config: ArchaeoRetrieverConfig{
			WorkflowID: "test-workflow",
			MaxItems:   4,
			MaxTokens:  500,
		},
	}
	
	sketch := HypotheticalSketch{
		Text:      "test sketch",
		Grounded:  true,
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

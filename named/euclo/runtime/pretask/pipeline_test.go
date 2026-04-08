package pretask

import (
	"context"
	"testing"
)

func TestPipeline_RunBasic(t *testing.T) {
	// Create a minimal pipeline with stub dependencies
	config := DefaultPipelineConfig()

	// Create pipeline with nil dependencies (should handle gracefully)
	pipeline := &Pipeline{
		anchorExtractor: &AnchorExtractor{
			index: &dummyIndexQuerier{},
			config: AnchorConfig{
				MinSymbolLength: 3,
				MaxSymbols:      12,
			},
		},
		indexRetriever: &IndexRetriever{
			index: &dummyIndexQuerier{},
			config: IndexRetrieverConfig{
				DependencyHops:    1,
				MaxFilesPerSymbol: 3,
			},
		},
		archaeoRetriever: &ArchaeoRetriever{
			config: ArchaeoRetrieverConfig{
				WorkflowID: "",
				MaxItems:   4,
				MaxTokens:  500,
			},
		},
		hypotheticalGen: &HypotheticalGenerator{},
		merger: &ResultMerger{
			config: MergerConfig{
				TokenBudget:       2000,
				MaxCodeFiles:      6,
				MaxKnowledgeItems: 4,
			},
		},
		config: config,
	}

	input := PipelineInput{
		Query:            "How does PermissionManager work?",
		CurrentTurnFiles: []string{"framework/authorization/manager.go"},
		SessionPins:      []string{"framework/core/types.go"},
		WorkflowID:       "",
	}

	bundle, err := pipeline.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	// Check that anchored files are included
	if len(bundle.AnchoredFiles) < 1 {
		t.Errorf("Expected at least 1 anchored file, got %d", len(bundle.AnchoredFiles))
	}

	// Check pipeline trace is populated
	if bundle.PipelineTrace.AnchorsExtracted == 0 {
		t.Error("PipelineTrace should have AnchorsExtracted > 0")
	}
}

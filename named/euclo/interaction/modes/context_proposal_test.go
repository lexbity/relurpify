package modes

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/runtime/pretask"
)

// MockPipeline implements ContextEnrichmentPipeline for testing
type MockPipeline struct {
	ShouldError bool
	Bundle      pretask.EnrichedContextBundle
}

func (m *MockPipeline) Run(ctx context.Context, input pretask.PipelineInput) (pretask.EnrichedContextBundle, error) {
	if m.ShouldError {
		return pretask.EnrichedContextBundle{}, context.DeadlineExceeded
	}
	return m.Bundle, nil
}

// MockFileResolver implements pretask.FileResolver for testing
type MockFileResolver struct {
	Paths []string
}

func (m *MockFileResolver) Resolve(selections []string, text string) pretask.ResolvedFiles {
	return pretask.ResolvedFiles{
		Paths:   m.Paths,
		Skipped: []string{},
	}
}

func TestContextProposalPhase_SilentMode(t *testing.T) {
	phase := &ContextProposalPhase{
		Pipeline: &MockPipeline{
			Bundle: pretask.EnrichedContextBundle{
				AnchoredFiles: []pretask.CodeEvidenceItem{
					{Path: "test1.go", Summary: "Test file 1"},
				},
				ExpandedFiles: []pretask.CodeEvidenceItem{
					{Path: "test2.go", Summary: "Test file 2"},
				},
				KnowledgeTopic: []pretask.KnowledgeEvidenceItem{
					{RefID: "k1", Title: "Knowledge 1"},
				},
				PipelineTrace: pretask.PipelineTrace{
					AnchorsExtracted: 2,
				},
			},
		},
		FileResolver:          &MockFileResolver{Paths: []string{"test1.go"}},
		ShowConfirmationFrame: false,
	}

	ctx := context.Background()
	mc := interaction.PhaseMachineContext{
		Emitter: &interaction.NoopEmitter{},
		State:   make(map[string]any),
	}

	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !outcome.Advance {
		t.Error("Expected Advance to be true")
	}

	// Check that state updates were set
	if confirmedFiles, ok := outcome.StateUpdates["context.confirmed_files"].([]string); !ok || len(confirmedFiles) != 2 {
		t.Error("Expected confirmed_files in state updates")
	}
	if pinnedFiles, ok := outcome.StateUpdates["context.pinned_files"].([]string); !ok || len(pinnedFiles) != 2 {
		t.Error("Expected pinned_files in state updates")
	}
	if knowledgeItems, ok := outcome.StateUpdates["context.knowledge_items"].([]pretask.KnowledgeEvidenceItem); !ok || len(knowledgeItems) != 1 {
		t.Error("Expected knowledge_items in state updates")
	}
	if pipelineTrace, ok := outcome.StateUpdates["context.pipeline_trace"].(pretask.PipelineTrace); !ok || pipelineTrace.AnchorsExtracted != 2 {
		t.Error("Expected pipeline_trace in state updates")
	}
}

func TestContextProposalPhase_WithConfirmationFrame(t *testing.T) {
	phase := &ContextProposalPhase{
		Pipeline: &MockPipeline{
			Bundle: pretask.EnrichedContextBundle{
				AnchoredFiles: []pretask.CodeEvidenceItem{
					{Path: "test1.go", Summary: "Test file 1"},
				},
			},
		},
		FileResolver:          &MockFileResolver{Paths: []string{"test1.go"}},
		ShowConfirmationFrame: true,
	}

	ctx := context.Background()
	mc := interaction.PhaseMachineContext{
		Emitter: &interaction.NoopEmitter{},
		State:   make(map[string]any),
	}

	// The NoopEmitter doesn't actually wait for responses, so this will work
	outcome, err := phase.Execute(ctx, mc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should still advance
	if !outcome.Advance {
		t.Error("Expected Advance to be true")
	}
}

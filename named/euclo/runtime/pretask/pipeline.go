package pretask

import (
	"context"
)

// Pipeline orchestrates the full pre-task context enrichment flow.
type Pipeline struct {
	anchorExtractor  *AnchorExtractor
	indexRetriever   *IndexRetriever
	archaeoRetriever *ArchaeoRetriever
	hypotheticalGen  *HypotheticalGenerator
	merger           *ResultMerger
	config           PipelineConfig
}

// PipelineInput is everything the pipeline needs from the call site.
type PipelineInput struct {
	Query string

	// CurrentTurnFiles are files the user explicitly selected in this interaction
	// turn via @mention or file picker. These become highest-priority anchors
	// unconditionally — no index confirmation required.
	// Includes files not yet loaded into context.
	CurrentTurnFiles []string

	// SessionPins are files confirmed by the user in prior turns this session.
	// These also bypass index confirmation.
	SessionPins []string

	// WorkflowID scopes archaeo retrieval. Empty string disables both archaeo passes.
	WorkflowID string
}

// Run executes the full pipeline and returns an EnrichedContextBundle.
//
// Execution order:
//   Stage 0: AnchorExtractor.Extract(input)
//            — CurrentTurnFiles and SessionPins are included unconditionally
//   Stage 1: parallel — IndexRetriever.Retrieve(anchors)
//                     + ArchaeoRetriever.RetrieveTopic(input.Query, input.WorkflowID)
//   Stage 2: HypotheticalGenerator.Generate(input.Query, stage1)
//            — skipped if anchor + stage1 code coverage >= SkipHypotheticalIfAnchorsAbove
//   Stage 3: ArchaeoRetriever.RetrieveExpanded(sketch)
//            — skipped if input.WorkflowID empty or Stage 2 was skipped
//   Merge:   ResultMerger.Merge
//
// Any stage error is logged and the pipeline continues with partial results.
// The PipelineTrace in the returned bundle records what was skipped and why.
func (p *Pipeline) Run(ctx context.Context, input PipelineInput) (EnrichedContextBundle, error) {
	// Placeholder for Phase 1.
	// Returns an empty bundle; will be implemented in later phases.
	return EnrichedContextBundle{}, nil
}

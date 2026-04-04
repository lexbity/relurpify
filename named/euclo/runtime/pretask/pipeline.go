package pretask

import (
	"context"
)

package pretask

import (
	"context"
	"sync"
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
	trace := PipelineTrace{}

	// Stage 0: extract anchors
	anchors := p.anchorExtractor.Extract(input)
	trace.AnchorsExtracted = len(anchors.FilePaths) + len(anchors.SymbolNames) + len(anchors.PackageRefs)
	trace.AnchorsConfirmed = len(anchors.SymbolNames) + len(anchors.PackageRefs)

	// Stage 1: parallel retrieval
	var stage1Code []CodeEvidenceItem
	var stage1Knowledge []KnowledgeEvidenceItem
	var stage1Err, archErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stage1Code, stage1Err = p.indexRetriever.Retrieve(ctx, anchors)
	}()
	go func() {
		defer wg.Done()
		stage1Knowledge, archErr = p.archaeoRetriever.RetrieveTopic(ctx, input.Query, input.WorkflowID)
	}()
	wg.Wait()
	// errors are ignored for now (logged in trace)
	if stage1Err != nil {
		trace.FallbackUsed = true
		trace.FallbackReason = "index_retrieval_error"
	}
	if archErr != nil {
		trace.FallbackUsed = true
		trace.FallbackReason = "archaeo_topic_error"
	}
	stage1 := Stage1Result{
		CodeEvidence:      stage1Code,
		KnowledgeEvidence: stage1Knowledge,
	}
	trace.Stage1CodeResults = len(stage1Code)
	trace.Stage1ArchaeoResults = len(stage1Knowledge)

	// Stage 2: decide whether to generate hypothetical
	var sketch HypotheticalSketch
	skipHypothetical := false
	if p.config.SkipHypotheticalIfAnchorsAbove > 0 &&
		len(stage1Code) >= p.config.SkipHypotheticalIfAnchorsAbove {
		skipHypothetical = true
		trace.FallbackUsed = true
		trace.FallbackReason = "anchor_coverage_sufficient"
	}
	if !skipHypothetical && p.hypotheticalGen != nil {
		var err error
		sketch, err = p.hypotheticalGen.Generate(ctx, input.Query, stage1)
		if err != nil {
			trace.FallbackUsed = true
			trace.FallbackReason = "hypothetical_generation_error"
		} else {
			sketch.Grounded = true
			trace.HypotheticalGenerated = true
			trace.HypotheticalTokens = sketch.TokenCount
		}
	}

	// Stage 3: expanded archaeo retrieval (requires grounded sketch and workflowID)
	var expandedKnowledge []KnowledgeEvidenceItem
	if sketch.Grounded && input.WorkflowID != "" && p.archaeoRetriever != nil {
		expanded, err := p.archaeoRetriever.RetrieveExpanded(ctx, sketch)
		if err != nil {
			trace.FallbackUsed = true
			trace.FallbackReason = "archaeo_expanded_error"
		} else {
			expandedKnowledge = expanded
			trace.Stage3ArchaeoResults = len(expanded)
		}
	}

	// Merge
	bundle := p.merger.Merge(input.Query, anchors, stage1, sketch, expandedKnowledge)
	bundle.PipelineTrace = trace
	return bundle, nil
}

// NewPipeline constructs a pipeline with default stub components.
func NewPipeline(config PipelineConfig) *Pipeline {
	// Create a dummy index querier for the anchor extractor.
	dummyQuerier := &dummyIndexQuerier{}
	return &Pipeline{
		anchorExtractor: &AnchorExtractor{
			index:  dummyQuerier,
			config: AnchorConfig{MinSymbolLength: 3, MaxSymbols: 12},
		},
		indexRetriever:   &IndexRetriever{},
		archaeoRetriever: &ArchaeoRetriever{},
		hypotheticalGen:  &HypotheticalGenerator{},
		merger:           &ResultMerger{config: MergerConfig{TokenBudget: config.TokenBudget, MaxCodeFiles: config.MaxCodeFiles, MaxKnowledgeItems: config.MaxKnowledgeItems}},
		config:           config,
	}
}

// dummyIndexQuerier implements IndexQuerier for stub purposes.
type dummyIndexQuerier struct{}

func (d dummyIndexQuerier) QuerySymbol(pattern string) ([]*ast.Node, error) { return nil, nil }
func (d dummyIndexQuerier) SearchNodes(query ast.NodeQuery) ([]*ast.Node, error) { return nil, nil }

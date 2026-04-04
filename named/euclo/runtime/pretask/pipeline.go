package pretask

import (
	"context"
	"sync"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/patterns"
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
	
	// Only run index retriever if we have anchors
	if len(anchors.FilePaths) > 0 || len(anchors.SymbolNames) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stage1Code, stage1Err = p.indexRetriever.Retrieve(ctx, anchors)
		}()
	}
	
	// Only run archaeo retriever if we have a workflow ID
	if input.WorkflowID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stage1Knowledge, archErr = p.archaeoRetriever.RetrieveTopic(ctx, input.Query, input.WorkflowID)
		}()
	}
	
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
		sketch, err := p.hypotheticalGen.Generate(ctx, input.Query, stage1)
		if err != nil {
			trace.FallbackUsed = true
			trace.FallbackReason = "hypothetical_generation_error"
		} else if sketch.Grounded {
			trace.HypotheticalGenerated = true
			trace.HypotheticalTokens = sketch.TokenCount
		} else {
			// Generation succeeded but returned ungrounded (e.g., no evidence)
			trace.FallbackUsed = true
			trace.FallbackReason = "hypothetical_not_grounded"
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

// NewPipeline constructs a pipeline from a WorkspaceEnvironment and an optional
// TensionQuerier. All service fields are optional — nil dependencies cause the
// relevant stage to be skipped gracefully.
func NewPipeline(
	env ayenitd.WorkspaceEnvironment,
	tensions TensionQuerier,  // optional — nil degrades archaeo topic retrieval to pattern-only
	config PipelineConfig,
) *Pipeline {
	// Create anchor extractor with index from environment
	var indexQuerier IndexQuerier
	if env.IndexManager != nil {
		indexQuerier = &indexManagerQuerier{im: env.IndexManager}
	} else {
		indexQuerier = &dummyIndexQuerier{}
	}
	
	anchorExtractor := &AnchorExtractor{
		index: indexQuerier,
		config: AnchorConfig{
			MinSymbolLength: 3,
			MaxSymbols:      12,
		},
	}
	
	// Create index retriever
	// For dependency querier, we can use the IndexManager if available
	var depQuerier DependencyQuerier
	if env.IndexManager != nil {
		depQuerier = env.IndexManager
	}
	
	indexRetriever := &IndexRetriever{
		index: indexQuerier,
		deps:  depQuerier,
		config: IndexRetrieverConfig{
			DependencyHops:   1,
			MaxFilesPerSymbol: 3,
		},
	}
	
	// Create archaeo retriever
	// For pattern store, we can use env.PatternStore
	var patternQuerier PatternQuerier
	if env.PatternStore != nil {
		patternQuerier = &patternStoreQuerier{store: env.PatternStore}
	}
	
	// Get retriever service from environment if available
	var retrieverSvc retrieval.RetrieverService
	if env.RetrievalDB != nil {
		// In a real implementation, we'd create a retriever service from the DB
		// For now, we'll leave it nil
		retrieverSvc = nil
	}
	
	archaeoRetriever := &ArchaeoRetriever{
		tensionSvc: tensions,
		patternSvc: patternQuerier,
		retriever:  retrieverSvc,
		config: ArchaeoRetrieverConfig{
			WorkflowID: config.WorkflowID,
			MaxItems:   config.MaxKnowledgeItems,
			MaxTokens:  500,
		},
	}
	
	// Create hypothetical generator with model and embedder from environment
	var hypotheticalGen *HypotheticalGenerator
	if env.Model != nil && env.Embedder != nil {
		hypotheticalGen = &HypotheticalGenerator{
			model:    env.Model,
			embedder: env.Embedder,
			config: HypotheticalConfig{
				MaxTokens:   config.HypotheticalMaxTokens,
				Temperature: 0.1,
			},
		}
	} else {
		// Create stub generator
		hypotheticalGen = &HypotheticalGenerator{}
	}
	
	// Create merger
	merger := &ResultMerger{
		config: MergerConfig{
			TokenBudget:       config.TokenBudget,
			MaxCodeFiles:      config.MaxCodeFiles,
			MaxKnowledgeItems: config.MaxKnowledgeItems,
		},
	}
	
	return &Pipeline{
		anchorExtractor:  anchorExtractor,
		indexRetriever:   indexRetriever,
		archaeoRetriever: archaeoRetriever,
		hypotheticalGen:  hypotheticalGen,
		merger:           merger,
		config:           config,
	}
}

// patternStoreQuerier implements PatternQuerier using patterns.PatternStore
type patternStoreQuerier struct {
	store patterns.PatternStore
}

func (q *patternStoreQuerier) ListByWorkflow(ctx context.Context, workflowID string) ([]interface{}, error) {
	// This is a simplified implementation
	// In reality, we would call the appropriate method on PatternStore
	_ = ctx
	_ = workflowID
	_ = q.store
	return []interface{}{}, nil
}

// indexManagerQuerier implements IndexQuerier using ast.IndexManager
type indexManagerQuerier struct {
	im *ast.IndexManager
}

func (q *indexManagerQuerier) QuerySymbol(pattern string) ([]*ast.Node, error) {
	if q.im == nil {
		return nil, nil
	}
	return q.im.QuerySymbol(pattern)
}

func (q *indexManagerQuerier) SearchNodes(query ast.NodeQuery) ([]*ast.Node, error) {
	if q.im == nil {
		return nil, nil
	}
	return q.im.SearchNodes(query)
}

// dummyIndexQuerier implements IndexQuerier for stub purposes.
type dummyIndexQuerier struct{}

func (d dummyIndexQuerier) QuerySymbol(pattern string) ([]*ast.Node, error) { return nil, nil }
func (d dummyIndexQuerier) SearchNodes(query ast.NodeQuery) ([]*ast.Node, error) { return nil, nil }

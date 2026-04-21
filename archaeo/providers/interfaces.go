package providers

import (
	"context"

	"codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

// PatternSurfacingRequest describes a runtime request to surface candidate
// patterns for an exploration/workflow context.
type PatternSurfacingRequest struct {
	WorkflowID      string
	ExplorationID   string
	WorkspaceID     string
	SymbolScope     string
	CorpusScope     string
	Kinds           []patterns.PatternKind
	MaxProposals    int
	BasedOnRevision string
}

// TensionAnalysisRequest describes a runtime request to analyze tensions for an
// exploration/workflow context.
type TensionAnalysisRequest struct {
	WorkflowID      string
	ExplorationID   string
	SnapshotID      string
	WorkspaceID     string
	FilePath        string
	AnchorIDs       []string
	BasedOnRevision string
}

// ProspectiveAnalysisRequest describes a runtime request for prospective
// analysis over candidate structural changes or likely pattern evolution.
type ProspectiveAnalysisRequest struct {
	WorkflowID      string
	ExplorationID   string
	WorkspaceID     string
	CorpusScope     string
	Description     string
	Limit           int
	MinScore        float64
	BasedOnRevision string
}

// ConvergenceReviewRequest describes a runtime request to review convergence of
// a living plan against current semantic/runtime state.
type ConvergenceReviewRequest struct {
	WorkflowID      string
	ExplorationID   string
	Plan            *frameworkplan.LivingPlan
	BasedOnRevision string
}

// PatternSurfacer is implemented by archaeology-specialist providers that can
// surface candidate patterns from a codebase/workspace.
type PatternSurfacer interface {
	SurfacePatterns(context.Context, PatternSurfacingRequest) ([]patterns.PatternRecord, error)
}

// TensionAnalyzer is implemented by archaeology-specialist providers that can
// surface tensions and contradictions from current findings.
type TensionAnalyzer interface {
	AnalyzeTensions(context.Context, TensionAnalysisRequest) ([]domain.Tension, error)
}

// ProspectiveAnalyzer is implemented by providers that can reason about likely
// or possible downstream structure implied by current findings.
type ProspectiveAnalyzer interface {
	AnalyzeProspective(context.Context, ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error)
}

// ConvergenceReviewer is implemented by providers that can review whether a
// current living plan still coheres with archaeology state and evidence.
type ConvergenceReviewer interface {
	ReviewConvergence(context.Context, ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error)
}

// Bundle groups archaeology-specialist provider interfaces so the runtime can
// depend on capability families through explicit contracts rather than direct
// capability handler wiring.
type Bundle struct {
	PatternSurfacer     PatternSurfacer
	TensionAnalyzer     TensionAnalyzer
	ProspectiveAnalyzer ProspectiveAnalyzer
	ConvergenceReviewer ConvergenceReviewer
}

package relurpicadapters

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/agents/relurpic"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeoretrieval "github.com/lexcodex/relurpify/archaeo/retrieval"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

// Runtime constructs an archaeology provider bundle backed by relurpic
// capability families without making the archaeology runtime depend directly on
// relurpic capability handler internals.
type PatternSurfacingDeps struct {
	Model        core.LanguageModel
	Config       *core.Config
	Registry     *capability.Registry
	IndexManager *ast.IndexManager
	GraphDB      *graphdb.Engine
	PatternStore patterns.PatternStore
	Retrieval    archaeoretrieval.Store
}

type TensionAnalysisDeps struct {
	Model         core.LanguageModel
	Config        *core.Config
	Registry      *capability.Registry
	IndexManager  *ast.IndexManager
	GraphDB       *graphdb.Engine
	Retrieval     archaeoretrieval.Store
	PlanStore     frameworkplan.PlanStore
	Guidance      *guidance.GuidanceBroker
	WorkflowStore memory.WorkflowStateStore
}

type ProspectiveAnalysisDeps struct {
	Model        core.LanguageModel
	Config       *core.Config
	PatternStore patterns.PatternStore
	Retrieval    archaeoretrieval.Store
}

type ConvergenceReviewDeps struct {
	PatternStore patterns.PatternStore
	TensionStore memory.WorkflowStateStore
}

type Runtime struct {
	PatternSurfacing    *PatternSurfacingDeps
	TensionAnalysis     *TensionAnalysisDeps
	ProspectiveAnalysis *ProspectiveAnalysisDeps
	ConvergenceReview   *ConvergenceReviewDeps

	Model         core.LanguageModel
	Config        *core.Config
	Registry      *capability.Registry
	IndexManager  *ast.IndexManager
	GraphDB       *graphdb.Engine
	PatternStore  patterns.PatternStore
	CommentStore  patterns.CommentStore
	Retrieval     archaeoretrieval.Store
	PlanStore     frameworkplan.PlanStore
	Guidance      *guidance.GuidanceBroker
	WorkflowStore memory.WorkflowStateStore
}

func (r Runtime) Bundle() providers.Bundle {
	patternDeps := r.patternSurfacingDeps()
	tensionDeps := r.tensionAnalysisDeps()
	prospectiveDeps := r.prospectiveAnalysisDeps()
	convergenceDeps := r.convergenceReviewDeps()
	return providers.Bundle{
		PatternSurfacer: relurpic.NewPatternSurfacingProvider(
			patternDeps.Model,
			patternDeps.Config,
			patternDeps.Registry,
			patternDeps.IndexManager,
			patternDeps.GraphDB,
			patternDeps.PatternStore,
			archaeoretrieval.SQLDB(patternDeps.Retrieval),
		),
		TensionAnalyzer: relurpic.NewTensionAnalysisProvider(
			tensionDeps.Model,
			tensionDeps.Config,
			tensionDeps.Registry,
			tensionDeps.IndexManager,
			tensionDeps.GraphDB,
			archaeoretrieval.SQLDB(tensionDeps.Retrieval),
			tensionDeps.PlanStore,
			tensionDeps.Guidance,
			tensionDeps.WorkflowStore,
		),
		ProspectiveAnalyzer: relurpic.NewProspectiveAnalysisProvider(
			prospectiveDeps.Model,
			prospectiveDeps.Config,
			prospectiveDeps.PatternStore,
			archaeoretrieval.SQLDB(prospectiveDeps.Retrieval),
		),
		ConvergenceReviewer: relurpic.NewConvergenceReviewProvider(
			convergenceDeps.PatternStore,
			convergenceDeps.TensionStore,
		),
	}
}

func (r Runtime) patternSurfacingDeps() PatternSurfacingDeps {
	if r.PatternSurfacing != nil {
		return *r.PatternSurfacing
	}
	return PatternSurfacingDeps{
		Model:        r.Model,
		Config:       r.Config,
		Registry:     r.Registry,
		IndexManager: r.IndexManager,
		GraphDB:      r.GraphDB,
		PatternStore: r.PatternStore,
		Retrieval:    r.Retrieval,
	}
}

func (r Runtime) tensionAnalysisDeps() TensionAnalysisDeps {
	if r.TensionAnalysis != nil {
		return *r.TensionAnalysis
	}
	return TensionAnalysisDeps{
		Model:         r.Model,
		Config:        r.Config,
		Registry:      r.Registry,
		IndexManager:  r.IndexManager,
		GraphDB:       r.GraphDB,
		Retrieval:     r.Retrieval,
		PlanStore:     r.PlanStore,
		Guidance:      r.Guidance,
		WorkflowStore: r.WorkflowStore,
	}
}

func (r Runtime) prospectiveAnalysisDeps() ProspectiveAnalysisDeps {
	if r.ProspectiveAnalysis != nil {
		return *r.ProspectiveAnalysis
	}
	return ProspectiveAnalysisDeps{
		Model:        r.Model,
		Config:       r.Config,
		PatternStore: r.PatternStore,
		Retrieval:    r.Retrieval,
	}
}

func (r Runtime) convergenceReviewDeps() ConvergenceReviewDeps {
	if r.ConvergenceReview != nil {
		return *r.ConvergenceReview
	}
	return ConvergenceReviewDeps{
		PatternStore: r.PatternStore,
		TensionStore: r.WorkflowStore,
	}
}

// Service routes archaeology operations through a provider bundle and maps the
// results back into archaeology runtime artifacts where appropriate.
type Service struct {
	Providers providers.Bundle
	Learning  archaeolearning.Service
	Tensions  archaeotensions.Service
}

func (s Service) SurfacePatterns(ctx context.Context, req providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
	if s.Providers.PatternSurfacer == nil {
		return nil, nil
	}
	return s.Providers.PatternSurfacer.SurfacePatterns(ctx, req)
}

func (s Service) SurfacePatternsAndSyncLearning(ctx context.Context, req providers.PatternSurfacingRequest) ([]patterns.PatternRecord, []archaeolearning.Interaction, error) {
	records, err := s.SurfacePatterns(ctx, req)
	if err != nil || len(records) == 0 {
		return records, nil, err
	}
	interactions, err := s.Learning.SyncPatternProposals(ctx, req.WorkflowID, req.ExplorationID, req.CorpusScope, req.BasedOnRevision)
	return records, interactions, err
}

func (s Service) AnalyzeProspective(ctx context.Context, req providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
	if s.Providers.ProspectiveAnalyzer == nil {
		return nil, nil
	}
	return s.Providers.ProspectiveAnalyzer.AnalyzeProspective(ctx, req)
}

func (s Service) ReviewConvergence(ctx context.Context, req providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	if s.Providers.ConvergenceReviewer == nil {
		return nil, nil
	}
	return s.Providers.ConvergenceReviewer.ReviewConvergence(ctx, req)
}

func (s Service) AnalyzeAndPersistTensions(ctx context.Context, req providers.TensionAnalysisRequest) ([]archaeodomain.Tension, []archaeolearning.Interaction, error) {
	if s.Providers.TensionAnalyzer == nil {
		return nil, nil, nil
	}
	tensionRecords, err := s.Providers.TensionAnalyzer.AnalyzeTensions(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	persisted := make([]archaeodomain.Tension, 0, len(tensionRecords))
	for _, record := range tensionRecords {
		created, err := s.Tensions.CreateOrUpdate(ctx, archaeotensions.CreateInput{
			WorkflowID:         strings.TrimSpace(req.WorkflowID),
			ExplorationID:      strings.TrimSpace(req.ExplorationID),
			SnapshotID:         strings.TrimSpace(req.SnapshotID),
			SourceRef:          firstNonEmpty(strings.TrimSpace(record.SourceRef), strings.TrimSpace(record.ID)),
			PatternIDs:         append([]string(nil), record.PatternIDs...),
			AnchorRefs:         append([]string(nil), record.AnchorRefs...),
			SymbolScope:        append([]string(nil), record.SymbolScope...),
			Kind:               firstNonEmpty(strings.TrimSpace(record.Kind), "analysis_tension"),
			Description:        strings.TrimSpace(record.Description),
			Severity:           strings.TrimSpace(record.Severity),
			Status:             firstNonEmptyTensionStatus(record.Status, archaeodomain.TensionUnresolved),
			BlastRadiusNodeIDs: append([]string(nil), record.BlastRadiusNodeIDs...),
			RelatedPlanStepIDs: append([]string(nil), record.RelatedPlanStepIDs...),
			CommentRefs:        append([]string(nil), record.CommentRefs...),
			BasedOnRevision:    firstNonEmpty(strings.TrimSpace(record.BasedOnRevision), strings.TrimSpace(req.BasedOnRevision)),
		})
		if err != nil {
			return nil, nil, err
		}
		if created != nil {
			persisted = append(persisted, *created)
		}
	}
	interactions, err := s.Learning.SyncTensions(ctx, req.WorkflowID, req.ExplorationID, req.SnapshotID, req.BasedOnRevision)
	if err != nil {
		return persisted, nil, err
	}
	return persisted, interactions, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmptyTensionStatus(value archaeodomain.TensionStatus, fallback archaeodomain.TensionStatus) archaeodomain.TensionStatus {
	if strings.TrimSpace(string(value)) != "" {
		return value
	}
	return fallback
}

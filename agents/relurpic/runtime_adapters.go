package relurpic

import (
	"context"
	"fmt"
	"strings"

	relurpicruntime "github.com/lexcodex/relurpify/agents/relurpic/runtime"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/archaeo/providers"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

func newPatternSurfacingService(p PatternSurfacingProvider) relurpicruntime.PatternSurfacingService {
	return relurpicruntime.PatternSurfacingFunc(func(ctx context.Context, req providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
		scope := strings.TrimSpace(req.SymbolScope)
		if scope == "" {
			return nil, fmt.Errorf("symbol scope required")
		}
		handler := patternDetectorDetectCapabilityHandler{
			model:        p.Model,
			config:       p.Config,
			registry:     p.Registry,
			indexManager: p.IndexManager,
			graphDB:      p.GraphDB,
			patternStore: p.PatternStore,
			retrievalDB:  p.RetrievalDB,
		}
		args := map[string]any{
			"symbol_scope":  scope,
			"corpus_scope":  strings.TrimSpace(req.CorpusScope),
			"max_proposals": req.MaxProposals,
		}
		if len(req.Kinds) > 0 {
			kinds := make([]any, 0, len(req.Kinds))
			for _, kind := range req.Kinds {
				kinds = append(kinds, string(kind))
			}
			args["kinds"] = kinds
		}
		result, err := handler.Invoke(ctx, nil, args)
		if err != nil {
			return nil, err
		}
		return patternRecordsFromCapabilityResult(ctx, p.PatternStore, result)
	})
}

func newTensionAnalysisService(p TensionAnalysisProvider) relurpicruntime.TensionAnalysisService {
	return relurpicruntime.TensionAnalysisFunc(func(ctx context.Context, req providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
		filePath := strings.TrimSpace(req.FilePath)
		if filePath == "" {
			return nil, fmt.Errorf("file path required")
		}
		handler := gapDetectorDetectCapabilityHandler{
			model:         p.Model,
			config:        p.Config,
			registry:      p.Registry,
			indexManager:  p.IndexManager,
			graphDB:       p.GraphDB,
			retrievalDB:   p.RetrievalDB,
			planStore:     p.PlanStore,
			guidance:      p.Guidance,
			workflowStore: p.WorkflowStore,
		}
		args := map[string]any{
			"file_path":               filePath,
			"corpus_scope":            strings.TrimSpace(req.WorkspaceID),
			"anchor_ids":              stringsToAny(req.AnchorIDs),
			"workflow_id":             strings.TrimSpace(req.WorkflowID),
			"exploration_id":          strings.TrimSpace(req.ExplorationID),
			"exploration_snapshot_id": strings.TrimSpace(req.SnapshotID),
			"based_on_revision":       strings.TrimSpace(req.BasedOnRevision),
		}
		result, err := handler.Invoke(ctx, nil, args)
		if err != nil {
			return nil, err
		}
		return tensionsFromGapResult(result), nil
	})
}

func newProspectiveAnalysisService(p ProspectiveAnalysisProvider) relurpicruntime.ProspectiveAnalysisService {
	return relurpicruntime.ProspectiveAnalysisFunc(func(ctx context.Context, req providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
		description := strings.TrimSpace(req.Description)
		if description == "" {
			return nil, fmt.Errorf("description required")
		}
		handler := prospectiveMatcherMatchCapabilityHandler{
			model:        p.Model,
			config:       p.Config,
			patternStore: p.PatternStore,
			retrievalDB:  p.RetrievalDB,
		}
		result, err := handler.Invoke(ctx, nil, map[string]any{
			"description":  description,
			"corpus_scope": strings.TrimSpace(req.CorpusScope),
			"limit":        req.Limit,
			"min_score":    req.MinScore,
		})
		if err != nil {
			return nil, err
		}
		return patternRecordsFromProspectiveResult(ctx, p.PatternStore, result)
	})
}

func newConvergenceReviewService(p ConvergenceReviewProvider) relurpicruntime.ConvergenceReviewService {
	return relurpicruntime.ConvergenceReviewFunc(func(ctx context.Context, req providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
		if req.Plan == nil || req.Plan.ConvergenceTarget == nil {
			return nil, nil
		}
		var detector TensionDetector
		if p.TensionStore != nil {
			detector = archaeotensions.Service{Store: p.TensionStore}
		}
		return (&PatternCoherenceVerifier{
			PatternStore:    p.PatternStore,
			TensionDetector: detector,
		}).Verify(ctx, *req.Plan.ConvergenceTarget)
	})
}

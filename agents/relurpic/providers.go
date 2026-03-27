package relurpic

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/archaeo/providers"
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

type PatternSurfacingProvider struct {
	Model        core.LanguageModel
	Config       *core.Config
	Registry     *capability.Registry
	IndexManager *ast.IndexManager
	GraphDB      *graphdb.Engine
	PatternStore patterns.PatternStore
	RetrievalDB  *sql.DB
}

func (p PatternSurfacingProvider) SurfacePatterns(ctx context.Context, req providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
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
}

type TensionAnalysisProvider struct {
	Model         core.LanguageModel
	Config        *core.Config
	Registry      *capability.Registry
	IndexManager  *ast.IndexManager
	GraphDB       *graphdb.Engine
	RetrievalDB   *sql.DB
	PlanStore     frameworkplan.PlanStore
	Guidance      *guidance.GuidanceBroker
	WorkflowStore memory.WorkflowStateStore
}

func (p TensionAnalysisProvider) AnalyzeTensions(ctx context.Context, req providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
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
}

type ProspectiveAnalysisProvider struct {
	Model        core.LanguageModel
	Config       *core.Config
	PatternStore patterns.PatternStore
	RetrievalDB  *sql.DB
}

func (p ProspectiveAnalysisProvider) AnalyzeProspective(ctx context.Context, req providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
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
}

type ConvergenceReviewProvider struct {
	PatternStore patterns.PatternStore
	TensionStore memory.WorkflowStateStore
}

func (p ConvergenceReviewProvider) ReviewConvergence(ctx context.Context, req providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
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
}

func patternRecordsFromCapabilityResult(ctx context.Context, store patterns.PatternStore, result *core.CapabilityExecutionResult) ([]patterns.PatternRecord, error) {
	if result == nil {
		return nil, nil
	}
	raw, _ := result.Data["proposals"].([]any)
	out := make([]patterns.PatternRecord, 0, len(raw))
	for _, item := range raw {
		payload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		patternID := stringArg(payload["id"])
		if store != nil && patternID != "" {
			record, err := store.Load(ctx, patternID)
			if err != nil {
				return nil, err
			}
			if record != nil {
				out = append(out, *record)
				continue
			}
		}
		out = append(out, patterns.PatternRecord{
			ID:          patternID,
			Kind:        normalizePatternKind(stringArg(payload["kind"])),
			Title:       stringArg(payload["title"]),
			Description: stringArg(payload["description"]),
			Status:      patterns.PatternStatusProposed,
		})
	}
	return out, nil
}

func patternRecordsFromProspectiveResult(ctx context.Context, store patterns.PatternStore, result *core.CapabilityExecutionResult) ([]patterns.PatternRecord, error) {
	if result == nil || store == nil {
		return nil, nil
	}
	raw, _ := result.Data["matches"].([]any)
	out := make([]patterns.PatternRecord, 0, len(raw))
	for _, item := range raw {
		payload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		patternID := stringArg(payload["pattern_id"])
		if patternID == "" {
			continue
		}
		record, err := store.Load(ctx, patternID)
		if err != nil {
			return nil, err
		}
		if record != nil {
			out = append(out, *record)
		}
	}
	return out, nil
}

func tensionsFromGapResult(result *core.CapabilityExecutionResult) []archaeodomain.Tension {
	if result == nil {
		return nil
	}
	raw, _ := result.Data["gaps"].([]any)
	out := make([]archaeodomain.Tension, 0, len(raw))
	for _, item := range raw {
		payload, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, archaeodomain.Tension{
			SourceRef:   stringArg(payload["gap_id"]),
			AnchorRefs:  nonEmptyStrings(stringArg(payload["anchor_id"])),
			SymbolScope: nonEmptyStrings(stringArg(payload["symbol_id"])),
			Kind:        "intent_gap",
			Description: stringArg(payload["description"]),
			Severity:    stringArg(payload["severity"]),
		})
	}
	return out
}

func stringsToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

package relurpic

import (
	"context"
	"database/sql"
	"strings"

	relurpicruntime "github.com/lexcodex/relurpify/agents/relurpic/runtime"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/archaeo/providers"
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
	Service      relurpicruntime.PatternSurfacingService
}

func (p PatternSurfacingProvider) SurfacePatterns(ctx context.Context, req providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
	service := p.Service
	if service == nil {
		service = newPatternSurfacingService(p)
	}
	return service.SurfacePatterns(ctx, req)
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
	Service       relurpicruntime.TensionAnalysisService
}

func (p TensionAnalysisProvider) AnalyzeTensions(ctx context.Context, req providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
	service := p.Service
	if service == nil {
		service = newTensionAnalysisService(p)
	}
	return service.AnalyzeTensions(ctx, req)
}

type ProspectiveAnalysisProvider struct {
	Model        core.LanguageModel
	Config       *core.Config
	PatternStore patterns.PatternStore
	RetrievalDB  *sql.DB
	Service      relurpicruntime.ProspectiveAnalysisService
}

func (p ProspectiveAnalysisProvider) AnalyzeProspective(ctx context.Context, req providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
	service := p.Service
	if service == nil {
		service = newProspectiveAnalysisService(p)
	}
	return service.AnalyzeProspective(ctx, req)
}

type ConvergenceReviewProvider struct {
	PatternStore patterns.PatternStore
	TensionStore memory.WorkflowStateStore
	Service      relurpicruntime.ConvergenceReviewService
}

func (p ConvergenceReviewProvider) ReviewConvergence(ctx context.Context, req providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	service := p.Service
	if service == nil {
		service = newConvergenceReviewService(p)
	}
	return service.ReviewConvergence(ctx, req)
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

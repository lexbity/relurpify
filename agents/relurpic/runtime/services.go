package runtime

import (
	"context"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/archaeo/providers"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

type PatternSurfacingService interface {
	SurfacePatterns(context.Context, providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error)
}

type TensionAnalysisService interface {
	AnalyzeTensions(context.Context, providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error)
}

type ProspectiveAnalysisService interface {
	AnalyzeProspective(context.Context, providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error)
}

type ConvergenceReviewService interface {
	ReviewConvergence(context.Context, providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error)
}

type GapAnalysisService interface {
	AnalyzeGap(context.Context, providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error)
}

type VerificationRepairService interface {
	RepairVerification(context.Context, map[string]any) (*frameworkplan.ConvergenceFailure, error)
}

type PatternSurfacingFunc func(context.Context, providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error)

func (f PatternSurfacingFunc) SurfacePatterns(ctx context.Context, req providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
	return f(ctx, req)
}

type TensionAnalysisFunc func(context.Context, providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error)

func (f TensionAnalysisFunc) AnalyzeTensions(ctx context.Context, req providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
	return f(ctx, req)
}

type ProspectiveAnalysisFunc func(context.Context, providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error)

func (f ProspectiveAnalysisFunc) AnalyzeProspective(ctx context.Context, req providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
	return f(ctx, req)
}

type ConvergenceReviewFunc func(context.Context, providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error)

func (f ConvergenceReviewFunc) ReviewConvergence(ctx context.Context, req providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	return f(ctx, req)
}

package testscenario

import (
	"context"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoproviders "github.com/lexcodex/relurpify/archaeo/providers"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

type RealisticPatternSurfacer struct {
	Store *patterns.SQLitePatternStore
	Err   error
	Calls []archaeoproviders.PatternSurfacingRequest
}

func (s *RealisticPatternSurfacer) SurfacePatterns(ctx context.Context, req archaeoproviders.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
	if s == nil {
		return nil, nil
	}
	s.Calls = append(s.Calls, req)
	if s.Err != nil {
		return nil, s.Err
	}
	if s.Store == nil {
		return nil, nil
	}
	return s.Store.ListByStatus(ctx, patterns.PatternStatusProposed, req.CorpusScope)
}

type ConfigurableTensionAnalyzer struct {
	Tensions []archaeodomain.Tension
	Err      error
	Calls    []archaeoproviders.TensionAnalysisRequest
}

func (s *ConfigurableTensionAnalyzer) AnalyzeTensions(_ context.Context, req archaeoproviders.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
	if s == nil {
		return nil, nil
	}
	s.Calls = append(s.Calls, req)
	if s.Err != nil {
		return nil, s.Err
	}
	out := make([]archaeodomain.Tension, 0, len(s.Tensions))
	out = append(out, s.Tensions...)
	return out, nil
}

type FailingConvergenceReviewer struct {
	Failure *frameworkplan.ConvergenceFailure
	Err     error
	Calls   []archaeoproviders.ConvergenceReviewRequest
}

func (s *FailingConvergenceReviewer) ReviewConvergence(_ context.Context, req archaeoproviders.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	if s == nil {
		return nil, nil
	}
	s.Calls = append(s.Calls, req)
	if s.Err != nil {
		return nil, s.Err
	}
	if s.Failure == nil {
		return nil, nil
	}
	copy := *s.Failure
	copy.UnconfirmedPatterns = append([]string(nil), s.Failure.UnconfirmedPatterns...)
	copy.UnresolvedTensions = append([]string(nil), s.Failure.UnresolvedTensions...)
	return &copy, nil
}

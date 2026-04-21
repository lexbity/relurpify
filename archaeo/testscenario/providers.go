package testscenario

import (
	"context"
	"strings"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoproviders "codeburg.org/lexbit/relurpify/archaeo/providers"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

type PatternSurfacerStub struct {
	Records []patterns.PatternRecord
	Err     error
	Calls   []archaeoproviders.PatternSurfacingRequest
}

func (s *PatternSurfacerStub) SurfacePatterns(_ context.Context, req archaeoproviders.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
	if s == nil {
		return nil, nil
	}
	s.Calls = append(s.Calls, req)
	if s.Err != nil {
		return nil, s.Err
	}
	out := make([]patterns.PatternRecord, 0, len(s.Records))
	for _, record := range s.Records {
		out = append(out, record)
	}
	return out, nil
}

type TensionAnalyzerStub struct {
	Records []archaeodomain.Tension
	Err     error
	Calls   []archaeoproviders.TensionAnalysisRequest
}

func (s *TensionAnalyzerStub) AnalyzeTensions(_ context.Context, req archaeoproviders.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
	if s == nil {
		return nil, nil
	}
	s.Calls = append(s.Calls, req)
	if s.Err != nil {
		return nil, s.Err
	}
	out := make([]archaeodomain.Tension, 0, len(s.Records))
	for _, record := range s.Records {
		out = append(out, record)
	}
	return out, nil
}

type ProspectiveAnalyzerStub struct {
	Records []patterns.PatternRecord
	Err     error
	Calls   []archaeoproviders.ProspectiveAnalysisRequest
}

func (s *ProspectiveAnalyzerStub) AnalyzeProspective(_ context.Context, req archaeoproviders.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
	if s == nil {
		return nil, nil
	}
	s.Calls = append(s.Calls, req)
	if s.Err != nil {
		return nil, s.Err
	}
	out := make([]patterns.PatternRecord, 0, len(s.Records))
	for _, record := range s.Records {
		out = append(out, record)
	}
	return out, nil
}

type ConvergenceReviewerStub struct {
	Failure *frameworkplan.ConvergenceFailure
	Err     error
	Calls   []archaeoproviders.ConvergenceReviewRequest
}

func (s *ConvergenceReviewerStub) ReviewConvergence(_ context.Context, req archaeoproviders.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
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

func (s *ConvergenceReviewerStub) SetFailure(reason string, patterns []string, tensions []string) {
	if s == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	s.Failure = &frameworkplan.ConvergenceFailure{
		Description:         reason,
		UnconfirmedPatterns: append([]string(nil), patterns...),
		UnresolvedTensions:  append([]string(nil), tensions...),
	}
}

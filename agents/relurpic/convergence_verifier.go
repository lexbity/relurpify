package relurpic

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/archaeo/plan"
)

// TensionDetector reports currently active tension IDs. It is optional; when nil,
// convergence verification only checks pattern confirmation state.
type TensionDetector interface {
	ActiveTensions(ctx context.Context) ([]string, error)
}

// PatternCoherenceVerifier implements plan.ConvergenceVerifier using persisted
// pattern confirmation state and optional tension detection.
type PatternCoherenceVerifier struct {
	PatternStore    patterns.PatternStore
	TensionDetector TensionDetector
}

func (v *PatternCoherenceVerifier) Verify(ctx context.Context, target frameworkplan.ConvergenceTarget) (*frameworkplan.ConvergenceFailure, error) {
	if v == nil || v.PatternStore == nil {
		return nil, nil
	}

	failure := &frameworkplan.ConvergenceFailure{}
	for _, patternID := range target.PatternIDs {
		patternID = strings.TrimSpace(patternID)
		if patternID == "" {
			continue
		}
		record, err := v.PatternStore.Load(ctx, patternID)
		if err != nil {
			return nil, err
		}
		if record == nil || record.Status != patterns.PatternStatusConfirmed {
			failure.UnconfirmedPatterns = append(failure.UnconfirmedPatterns, patternID)
		}
	}

	if v.TensionDetector != nil && len(target.TensionIDs) > 0 {
		active, err := v.TensionDetector.ActiveTensions(ctx)
		if err != nil {
			return nil, err
		}
		activeSet := make(map[string]struct{}, len(active))
		for _, id := range active {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			activeSet[id] = struct{}{}
		}
		for _, id := range target.TensionIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := activeSet[id]; ok {
				failure.UnresolvedTensions = append(failure.UnresolvedTensions, id)
			}
		}
	}

	if len(failure.UnconfirmedPatterns) == 0 && len(failure.UnresolvedTensions) == 0 {
		return nil, nil
	}
	failure.Description = describeConvergenceFailure(failure)
	return failure, nil
}

func describeConvergenceFailure(failure *frameworkplan.ConvergenceFailure) string {
	if failure == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if len(failure.UnconfirmedPatterns) > 0 {
		parts = append(parts, fmt.Sprintf("unconfirmed patterns: %s", strings.Join(failure.UnconfirmedPatterns, ", ")))
	}
	if len(failure.UnresolvedTensions) > 0 {
		parts = append(parts, fmt.Sprintf("unresolved tensions: %s", strings.Join(failure.UnresolvedTensions, ", ")))
	}
	return strings.Join(parts, "; ")
}

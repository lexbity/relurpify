package relurpic

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

type stubTensionDetector struct {
	active []string
	err    error
}

func (s stubTensionDetector) ActiveTensions(context.Context) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]string(nil), s.active...), nil
}

func TestPatternCoherenceVerifierReturnsNilWhenAllTargetsSatisfied(t *testing.T) {
	_, _, patternStore, _, _ := newPatternDetectorFixtures(t)
	now := time.Now().UTC()
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-1",
		Kind:         patterns.PatternKindStructural,
		Title:        "Confirmed pattern",
		Description:  "Confirmed",
		Status:       patterns.PatternStatusConfirmed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		Confidence:   1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))

	verifier := &PatternCoherenceVerifier{
		PatternStore:    patternStore,
		TensionDetector: stubTensionDetector{active: []string{"tension-other"}},
	}
	failure, err := verifier.Verify(context.Background(), frameworkplan.ConvergenceTarget{
		PatternIDs: []string{"pattern-1"},
		TensionIDs: []string{"tension-1"},
	})
	require.NoError(t, err)
	require.Nil(t, failure)
}

func TestPatternCoherenceVerifierReportsUnconfirmedPatternsAndTensions(t *testing.T) {
	_, _, patternStore, _, _ := newPatternDetectorFixtures(t)
	now := time.Now().UTC()
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-confirmed",
		Kind:         patterns.PatternKindBoundary,
		Title:        "Confirmed",
		Description:  "Confirmed",
		Status:       patterns.PatternStatusConfirmed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		Confidence:   1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-proposed",
		Kind:         patterns.PatternKindBoundary,
		Title:        "Proposed",
		Description:  "Proposed",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		Confidence:   0.4,
		CreatedAt:    now,
		UpdatedAt:    now,
	}))

	verifier := &PatternCoherenceVerifier{
		PatternStore:    patternStore,
		TensionDetector: stubTensionDetector{active: []string{"tension-live"}},
	}
	failure, err := verifier.Verify(context.Background(), frameworkplan.ConvergenceTarget{
		PatternIDs: []string{"pattern-confirmed", "pattern-proposed", "pattern-missing"},
		TensionIDs: []string{"tension-live"},
	})
	require.NoError(t, err)
	require.NotNil(t, failure)
	require.Equal(t, []string{"pattern-proposed", "pattern-missing"}, failure.UnconfirmedPatterns)
	require.Equal(t, []string{"tension-live"}, failure.UnresolvedTensions)
	require.Contains(t, failure.Description, "unconfirmed patterns")
	require.Contains(t, failure.Description, "unresolved tensions")
}

package plan

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/stretchr/testify/require"
)

func TestPlanGraphTopologicalOrder(t *testing.T) {
	now := time.Now().UTC()
	living := &LivingPlan{
		Steps: map[string]*PlanStep{
			"a": {ID: "a", Status: PlanStepPending, CreatedAt: now, UpdatedAt: now},
			"b": {ID: "b", Status: PlanStepPending, DependsOn: []string{"a"}, CreatedAt: now, UpdatedAt: now},
			"c": {ID: "c", Status: PlanStepPending, DependsOn: []string{"b"}, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"a", "b", "c"},
	}
	order, err := NewPlanGraph(living).TopologicalOrder()
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, order)

	living.Steps["a"].DependsOn = []string{"c"}
	_, err = NewPlanGraph(living).TopologicalOrder()
	require.Error(t, err)
}

func TestPlanGraphReadyStepsAndDependents(t *testing.T) {
	now := time.Now().UTC()
	living := &LivingPlan{
		Steps: map[string]*PlanStep{
			"a": {ID: "a", Status: PlanStepCompleted, CreatedAt: now, UpdatedAt: now},
			"b": {ID: "b", Status: PlanStepPending, DependsOn: []string{"a"}, CreatedAt: now, UpdatedAt: now},
			"c": {ID: "c", Status: PlanStepPending, DependsOn: []string{"b"}, CreatedAt: now, UpdatedAt: now},
			"d": {ID: "d", Status: PlanStepFailed, DependsOn: []string{"a"}, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"a", "b", "c", "d"},
	}
	ready := NewPlanGraph(living).ReadySteps()
	require.Len(t, ready, 1)
	require.Equal(t, "b", ready[0].ID)
	require.ElementsMatch(t, []string{"b", "c", "d"}, NewPlanGraph(living).Dependents("a"))
}

func TestRecalculateConfidenceAndDefaults(t *testing.T) {
	cfg := DefaultConfidenceDegradation()
	require.Equal(t, 0.2, cfg.AnchorDriftPenalty)
	require.Equal(t, 0.3, cfg.SymbolMissingPenalty)
	require.Equal(t, 0.4, cfg.Threshold)

	step := &PlanStep{ConfidenceScore: 1.0}
	score := RecalculateConfidence(step, []string{"anchor-1", "anchor-2"}, []string{"sym-1"}, cfg)
	require.InDelta(t, 0.3, score, 0.0001)

	score = RecalculateConfidence(&PlanStep{ConfidenceScore: 0.1}, nil, []string{"sym-1"}, cfg)
	require.Zero(t, score)
}

func TestEvidenceGateMaxTotalLoss(t *testing.T) {
	origin := core.OriginDerivation("retrieval")
	derived := origin.Derive("summarize", "retrieval", 0.5, "lossy")
	evidence := retrieval.MixedEvidenceResult{Derivation: &derived}

	allowed := EvidenceGateAllows(&EvidenceGate{MaxTotalLoss: 0.3}, evidence, nil, nil)
	require.False(t, allowed)

	allowed = EvidenceGateAllows(&EvidenceGate{MaxTotalLoss: 0.8}, evidence, nil, nil)
	require.True(t, allowed)
}

func TestPropagateInvalidation(t *testing.T) {
	now := time.Now().UTC()
	living := &LivingPlan{
		Steps: map[string]*PlanStep{
			"a": {
				ID: "a",
				InvalidatedBy: []InvalidationRule{{
					Kind:   InvalidationSymbolChanged,
					Target: "auth.Check",
				}},
				Status:    PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
			"b": {
				ID:        "b",
				DependsOn: []string{"a"},
				Status:    PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
			"c": {
				ID: "c",
				InvalidatedBy: []InvalidationRule{{
					Kind:   InvalidationAnchorDrifted,
					Target: "anchor:policy",
				}},
				Status:    PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	invalidated := PropagateInvalidation(living, InvalidationEvent{
		Kind:   InvalidationSymbolChanged,
		Target: "auth.Check",
		At:     now.Add(time.Minute),
	})
	require.ElementsMatch(t, []string{"a", "b"}, invalidated)
	require.Equal(t, PlanStepInvalidated, living.Steps["a"].Status)
	require.Equal(t, PlanStepInvalidated, living.Steps["b"].Status)

	invalidated = PropagateInvalidation(living, InvalidationEvent{
		Kind:   InvalidationAnchorDrifted,
		Target: "anchor:policy",
		At:     now.Add(2 * time.Minute),
	})
	require.ElementsMatch(t, []string{"c"}, invalidated)
	require.Equal(t, PlanStepInvalidated, living.Steps["c"].Status)
}

type stubConvergenceVerifier struct{}

func (stubConvergenceVerifier) Verify(_ context.Context, target ConvergenceTarget) (*ConvergenceFailure, error) {
	return &ConvergenceFailure{
		UnconfirmedPatterns: append([]string(nil), target.PatternIDs...),
		UnresolvedTensions:  append([]string(nil), target.TensionIDs...),
		Description:         "not converged",
	}, nil
}

func TestConvergenceVerifierInterface(t *testing.T) {
	verifier := stubConvergenceVerifier{}
	failure, err := verifier.Verify(context.Background(), ConvergenceTarget{
		PatternIDs: []string{"pattern-1"},
		TensionIDs: []string{"tension-1"},
		Commentary: "done means coherent",
	})
	require.NoError(t, err)
	require.NotNil(t, failure)
	require.NotNil(t, failure.UnconfirmedPatterns)
	require.NotNil(t, failure.UnresolvedTensions)
	require.Equal(t, []string{"pattern-1"}, failure.UnconfirmedPatterns)
}

package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/archaeo/execution"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

type preflightPlanStore struct{}

func (preflightPlanStore) SavePlan(context.Context, *frameworkplan.LivingPlan) error { return nil }
func (preflightPlanStore) LoadPlan(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return nil, nil
}
func (preflightPlanStore) LoadPlanByWorkflow(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return nil, nil
}
func (preflightPlanStore) UpdateStep(context.Context, string, string, *frameworkplan.PlanStep) error {
	return nil
}
func (preflightPlanStore) InvalidateStep(context.Context, string, string, frameworkplan.InvalidationRule) error {
	return nil
}
func (preflightPlanStore) DeletePlan(context.Context, string) error { return nil }
func (preflightPlanStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

func TestPreflightCoordinatorBlocksMissingSymbols(t *testing.T) {
	now := time.Date(2026, 3, 26, 23, 0, 0, 0, time.UTC)
	opts := graphdb.DefaultOptions(t.TempDir())
	engine, err := graphdb.Open(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	coord := execution.PreflightCoordinator{
		Service: execution.Service{Now: func() time.Time { return now }},
		Plans:   archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
	}
	plan := &frameworkplan.LivingPlan{
		ID: "plan-1",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				Scope:     []string{"missing.symbol"},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	outcome, err := coord.EvaluatePlanStepGate(context.Background(), &core.Task{}, core.NewContext(), plan, plan.Steps["step-1"], engine)
	require.Error(t, err)
	require.True(t, outcome.ShouldInvalidate)
}

func TestPreflightCoordinatorShortCircuitsOnGuidance(t *testing.T) {
	now := time.Date(2026, 3, 26, 23, 0, 0, 0, time.UTC)
	opts := graphdb.DefaultOptions(t.TempDir())
	engine, err := graphdb.Open(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	require.NoError(t, engine.UpsertNode(graphdb.NodeRecord{ID: "symbol.present", Kind: "symbol"}))

	coord := execution.PreflightCoordinator{
		Service: execution.Service{Now: func() time.Time { return now }},
		Plans:   archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
		RequestGuidance: func(context.Context, guidance.GuidanceRequest, string) guidance.GuidanceDecision {
			return guidance.GuidanceDecision{ChoiceID: "skip", DecidedBy: "test"}
		},
	}
	plan := &frameworkplan.LivingPlan{
		ID: "plan-1",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:              "step-1",
				Status:          frameworkplan.PlanStepPending,
				Scope:           []string{"symbol.present"},
				ConfidenceScore: 0.1,
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
	}
	outcome, err := coord.EvaluatePlanStepGate(context.Background(), &core.Task{}, core.NewContext(), plan, plan.Steps["step-1"], engine)
	require.NoError(t, err)
	require.NotNil(t, outcome.Result)
	require.True(t, outcome.Result.Success)
	require.Equal(t, frameworkplan.PlanStepSkipped, plan.Steps["step-1"].Status)
}

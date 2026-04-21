package execution_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/archaeo/execution"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	archaeoverification "codeburg.org/lexbit/relurpify/archaeo/verification"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

type finalizePlanStore struct {
	plan    *frameworkplan.LivingPlan
	updates map[string]*frameworkplan.PlanStep
}

func (s *finalizePlanStore) SavePlan(context.Context, *frameworkplan.LivingPlan) error { return nil }
func (s *finalizePlanStore) LoadPlan(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return nil, nil
}
func (s *finalizePlanStore) LoadPlanByWorkflow(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return s.plan, nil
}
func (s *finalizePlanStore) UpdateStep(_ context.Context, _ string, stepID string, step *frameworkplan.PlanStep) error {
	if s.updates == nil {
		s.updates = map[string]*frameworkplan.PlanStep{}
	}
	copy := *step
	s.updates[stepID] = &copy
	return nil
}
func (s *finalizePlanStore) InvalidateStep(context.Context, string, string, frameworkplan.InvalidationRule) error {
	return nil
}
func (s *finalizePlanStore) DeletePlan(context.Context, string) error { return nil }
func (s *finalizePlanStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

type finalizeVerifier struct {
	failure *frameworkplan.ConvergenceFailure
	err     error
}

func (v finalizeVerifier) Verify(context.Context, frameworkplan.ConvergenceTarget) (*frameworkplan.ConvergenceFailure, error) {
	return v.failure, v.err
}

func TestFinalizerRecordsStepOutcomeAndCheckpoint(t *testing.T) {
	now := time.Date(2026, 3, 26, 22, 0, 0, 0, time.UTC)
	store := &finalizePlanStore{}
	planSvc := archaeoplans.Service{Store: store, Now: func() time.Time { return now }}
	finalizer := execution.Finalizer{
		Plans: planSvc,
		GitCheckpoint: func(context.Context, *core.Task) string {
			return "abc123"
		},
	}
	plan := &frameworkplan.LivingPlan{
		ID: "plan-1",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
	}
	state := core.NewContext()
	finalizer.FinalizeLivingPlan(context.Background(), &core.Task{}, state, plan, plan.Steps["step-1"], &core.Result{Success: true}, nil)
	require.Equal(t, frameworkplan.PlanStepCompleted, plan.Steps["step-1"].Status)
	require.Len(t, plan.Steps["step-1"].History, 1)
	require.Equal(t, "abc123", plan.Steps["step-1"].History[0].GitCheckpoint)
	raw, ok := state.Get("euclo.living_plan")
	require.True(t, ok)
	require.Same(t, plan, raw)
}

func TestFinalizerSurfacesConvergenceFailure(t *testing.T) {
	now := time.Date(2026, 3, 26, 22, 0, 0, 0, time.UTC)
	store := &finalizePlanStore{}
	finalizer := execution.Finalizer{
		Plans: archaeoplans.Service{Store: store, Now: func() time.Time { return now }},
		Verification: archaeoverification.Service{
			Store: store,
			Verifier: finalizeVerifier{failure: &frameworkplan.ConvergenceFailure{
				Description: "not converged",
			}},
		},
	}
	plan := &frameworkplan.LivingPlan{
		ID: "plan-1",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{PatternIDs: []string{"pattern-1"}},
	}
	state := core.NewContext()
	finalizer.FinalizeLivingPlan(context.Background(), &core.Task{}, state, plan, plan.Steps["step-1"], &core.Result{Success: true}, nil)
	raw, ok := state.Get("euclo.convergence_failure")
	require.True(t, ok)
	failure, ok := raw.(frameworkplan.ConvergenceFailure)
	require.True(t, ok)
	require.Equal(t, "not converged", failure.Description)
}

func TestFinalizerSkipsConvergenceOnExecutionError(t *testing.T) {
	now := time.Date(2026, 3, 26, 22, 0, 0, 0, time.UTC)
	store := &finalizePlanStore{}
	finalizer := execution.Finalizer{
		Plans: archaeoplans.Service{Store: store, Now: func() time.Time { return now }},
		Verification: archaeoverification.Service{
			Store:    store,
			Verifier: finalizeVerifier{err: errors.New("should not run")},
		},
	}
	plan := &frameworkplan.LivingPlan{
		ID: "plan-1",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{PatternIDs: []string{"pattern-1"}},
	}
	state := core.NewContext()
	finalizer.FinalizeLivingPlan(context.Background(), &core.Task{}, state, plan, plan.Steps["step-1"], &core.Result{Success: false, Error: errors.New("boom")}, errors.New("boom"))
	_, ok := state.Get("euclo.convergence_failure")
	require.False(t, ok)
	require.Equal(t, frameworkplan.PlanStepFailed, plan.Steps["step-1"].Status)
}

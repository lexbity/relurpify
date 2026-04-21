package verification_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/archaeo/verification"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

type stubVerifier struct {
	failure *frameworkplan.ConvergenceFailure
	err     error
}

func (s stubVerifier) Verify(context.Context, frameworkplan.ConvergenceTarget) (*frameworkplan.ConvergenceFailure, error) {
	return s.failure, s.err
}

type stubSavePlanStore struct {
	saved *frameworkplan.LivingPlan
}

func (s *stubSavePlanStore) SavePlan(_ context.Context, plan *frameworkplan.LivingPlan) error {
	s.saved = plan
	return nil
}
func (s *stubSavePlanStore) LoadPlan(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return nil, nil
}
func (s *stubSavePlanStore) LoadPlanByWorkflow(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return nil, nil
}
func (s *stubSavePlanStore) UpdateStep(context.Context, string, string, *frameworkplan.PlanStep) error {
	return nil
}
func (s *stubSavePlanStore) InvalidateStep(context.Context, string, string, frameworkplan.InvalidationRule) error {
	return nil
}
func (s *stubSavePlanStore) DeletePlan(context.Context, string) error { return nil }
func (s *stubSavePlanStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

func TestFinalizeConvergenceMarksVerifiedAtOnSuccess(t *testing.T) {
	now := time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC)
	store := &stubSavePlanStore{}
	svc := verification.Service{
		Store:    store,
		Verifier: stubVerifier{},
		Now:      func() time.Time { return now },
	}
	plan := &frameworkplan.LivingPlan{
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{PatternIDs: []string{"p1"}},
	}

	failure, err := svc.FinalizeConvergence(context.Background(), plan, &core.Result{})
	require.NoError(t, err)
	require.Nil(t, failure)
	require.NotNil(t, plan.ConvergenceTarget.VerifiedAt)
	require.Equal(t, now, *plan.ConvergenceTarget.VerifiedAt)
	require.Same(t, plan, store.saved)
}

func TestFinalizeConvergenceAddsFailureToResult(t *testing.T) {
	svc := verification.Service{
		Verifier: stubVerifier{failure: &frameworkplan.ConvergenceFailure{Description: "still broken"}},
	}
	plan := &frameworkplan.LivingPlan{
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{PatternIDs: []string{"p1"}},
	}
	result := &core.Result{}

	failure, err := svc.FinalizeConvergence(context.Background(), plan, result)
	require.NoError(t, err)
	require.NotNil(t, failure)
	require.Contains(t, result.Data, "convergence_failure")
}

func TestFinalizeConvergenceIncludesUnresolvedTensionRecords(t *testing.T) {
	ctx := context.Background()
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-verify",
		TaskID:      "task-verify",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "verify",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	record, err := (archaeotensions.Service{Store: workflowStore}).CreateOrUpdate(ctx, archaeotensions.CreateInput{
		WorkflowID:  "wf-verify",
		SourceRef:   "gap-1",
		Kind:        "intent_gap",
		Description: "still broken",
		Status:      archaeodomain.TensionUnresolved,
	})
	require.NoError(t, err)

	svc := verification.Service{
		Verifier: stubVerifier{failure: &frameworkplan.ConvergenceFailure{
			Description:        "still broken",
			UnresolvedTensions: []string{record.ID},
		}},
		Tensions: archaeotensions.Service{Store: workflowStore},
	}
	plan := &frameworkplan.LivingPlan{
		WorkflowID: "wf-verify",
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{
			TensionIDs: []string{record.ID},
		},
	}
	result := &core.Result{}

	failure, err := svc.FinalizeConvergence(ctx, plan, result)
	require.NoError(t, err)
	require.NotNil(t, failure)
	records, ok := result.Data["unresolved_tension_records"].([]any)
	require.True(t, ok)
	require.Len(t, records, 1)
}

func newWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})
	return store
}

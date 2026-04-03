package plans

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

type versioningTestStore struct {
	plans   map[string]*frameworkplan.LivingPlan
	updates map[string]*frameworkplan.PlanStep
}

func (s *versioningTestStore) SavePlan(_ context.Context, plan *frameworkplan.LivingPlan) error {
	if s.plans == nil {
		s.plans = make(map[string]*frameworkplan.LivingPlan)
	}
	cpy := *plan
	s.plans[plan.ID] = &cpy
	return nil
}
func (s *versioningTestStore) LoadPlan(_ context.Context, planID string) (*frameworkplan.LivingPlan, error) {
	return s.plans[planID], nil
}
func (s *versioningTestStore) LoadPlanByWorkflow(context.Context, string) (*frameworkplan.LivingPlan, error) {
	// not needed for these tests
	return nil, nil
}
func (s *versioningTestStore) UpdateStep(_ context.Context, _ string, stepID string, step *frameworkplan.PlanStep) error {
	if s.updates == nil {
		s.updates = make(map[string]*frameworkplan.PlanStep)
	}
	cpy := *step
	s.updates[stepID] = &cpy
	return nil
}
func (s *versioningTestStore) InvalidateStep(context.Context, string, string, frameworkplan.InvalidationRule) error {
	return nil
}
func (s *versioningTestStore) DeletePlan(context.Context, string) error { return nil }
func (s *versioningTestStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

func TestArchiveVersion(t *testing.T) {
	now := time.Now().UTC()
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-archive",
		TaskID:      "task-archive",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	store := &versioningTestStore{}
	svc := Service{
		Store:         store,
		WorkflowStore: workflowStore,
		Now:           func() time.Time { return now },
	}

	// create a draft version and activate it
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-archive",
		WorkflowID: "wf-archive",
		Title:      "Test",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v1, err := svc.DraftVersion(ctx, plan, DraftVersionInput{
		WorkflowID:             "wf-archive",
		DerivedFromExploration: "exp1",
		BasedOnRevision:        "rev1",
	})
	require.NoError(t, err)
	require.Equal(t, 1, v1.Version)

	active, err := svc.ActivateVersion(ctx, "wf-archive", v1.Version)
	require.NoError(t, err)
	require.Equal(t, archaeodomain.LivingPlanVersionActive, active.Status)

	// archive it
	archived, err := svc.ArchiveVersion(ctx, "wf-archive", v1.Version, "obsolete")
	require.NoError(t, err)
	require.Equal(t, archaeodomain.LivingPlanVersionArchived, archived.Status)
	require.True(t, archived.RecomputeRequired)
	require.Equal(t, "obsolete", archived.StaleReason)

	// verify it's not active anymore
	lineage, err := svc.LoadLineage(ctx, "wf-archive")
	require.NoError(t, err)
	require.NotNil(t, lineage)
	require.Nil(t, lineage.ActiveVersion) // because the only version is archived
}

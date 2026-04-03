package plans_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

type stubPlanStore struct {
	plan    *frameworkplan.LivingPlan
	updates map[string]*frameworkplan.PlanStep
	plans   map[string]*frameworkplan.LivingPlan
}

func (s *stubPlanStore) SavePlan(_ context.Context, plan *frameworkplan.LivingPlan) error {
	if s.plans == nil {
		s.plans = map[string]*frameworkplan.LivingPlan{}
	}
	copy := *plan
	s.plans[plan.ID] = &copy
	s.plan = &copy
	return nil
}
func (s *stubPlanStore) LoadPlan(_ context.Context, planID string) (*frameworkplan.LivingPlan, error) {
	if s.plans == nil {
		return nil, nil
	}
	return s.plans[planID], nil
}
func (s *stubPlanStore) LoadPlanByWorkflow(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return s.plan, nil
}
func (s *stubPlanStore) UpdateStep(_ context.Context, _ string, stepID string, step *frameworkplan.PlanStep) error {
	if s.updates == nil {
		s.updates = map[string]*frameworkplan.PlanStep{}
	}
	copy := *step
	s.updates[stepID] = &copy
	return nil
}
func (s *stubPlanStore) InvalidateStep(context.Context, string, string, frameworkplan.InvalidationRule) error {
	return nil
}
func (s *stubPlanStore) DeletePlan(context.Context, string) error { return nil }
func (s *stubPlanStore) ListPlans(context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

func TestLoadActiveContextResolvesPlanAndCurrentStep(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
			},
		},
	}
	svc := plans.Service{Store: store}
	ctx, err := svc.LoadActiveContext(context.Background(), "wf-1", &core.Task{
		Context: map[string]any{"current_step_id": "step-1"},
	})
	require.NoError(t, err)
	require.NotNil(t, ctx)
	require.Equal(t, "plan-1", ctx.Plan.ID)
	require.NotNil(t, ctx.Step)
	require.Equal(t, "step-1", ctx.Step.ID)
}

func TestRecordStepOutcomeMarksCompletedAndPersists(t *testing.T) {
	now := time.Date(2026, 3, 26, 16, 0, 0, 0, time.UTC)
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
	}

	svc.RecordStepOutcome(plan, plan.Steps["step-1"], "completed", "", "abc123")
	require.Equal(t, frameworkplan.PlanStepCompleted, plan.Steps["step-1"].Status)
	require.Len(t, plan.Steps["step-1"].History, 1)
	require.Equal(t, "abc123", plan.Steps["step-1"].History[0].GitCheckpoint)
	require.NoError(t, svc.PersistStep(context.Background(), plan, "step-1"))
	require.Contains(t, store.updates, "step-1")
}

func TestApplyInvalidationsAndRecordBlockedStep(t *testing.T) {
	now := time.Date(2026, 3, 26, 20, 0, 0, 0, time.UTC)
	svc := plans.Service{Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		CreatedAt:  now,
		UpdatedAt:  now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				Scope:     []string{"symbol.a"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			"step-2": {
				ID:        "step-2",
				Status:    frameworkplan.PlanStepPending,
				DependsOn: []string{"step-1"},
				InvalidatedBy: []frameworkplan.InvalidationRule{{
					Kind:   frameworkplan.InvalidationSymbolChanged,
					Target: "symbol.a",
				}},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	changed := svc.ApplySymbolInvalidations(plan, "step-1", []string{"symbol.a"})
	require.Equal(t, []string{"step-2"}, changed)
	require.Equal(t, frameworkplan.PlanStepInvalidated, plan.Steps["step-2"].Status)

	svc.RecordBlockedStep(plan, plan.Steps["step-1"], "missing symbol", true)
	require.Equal(t, frameworkplan.PlanStepInvalidated, plan.Steps["step-1"].Status)
	require.Len(t, plan.Steps["step-1"].History, 1)
	require.Equal(t, "blocked", plan.Steps["step-1"].History[0].Outcome)
}

func TestPersistPreflightHelpers(t *testing.T) {
	now := time.Date(2026, 3, 26, 21, 0, 0, 0, time.UTC)
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		CreatedAt:  now,
		UpdatedAt:  now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
	}

	result, err := svc.PersistPreflightBlocked(context.Background(), plan, plan.Steps["step-1"], "blocked by test", true, nil)
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Equal(t, frameworkplan.PlanStepInvalidated, plan.Steps["step-1"].Status)

	plan.Steps["step-1"].Status = frameworkplan.PlanStepPending
	short, shortErr := svc.PersistPreflightShortCircuit(context.Background(), plan, plan.Steps["step-1"], &core.Result{Success: true}, nil)
	require.NoError(t, shortErr)
	require.True(t, short.Success)

	require.NoError(t, svc.PersistPreflightConfidenceUpdate(context.Background(), plan, plan.Steps["step-1"]))
	require.Contains(t, store.updates, "step-1")
}

func TestPlanVersionLifecycleAndActiveContext(t *testing.T) {
	now := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{
		Store:         store,
		WorkflowStore: workflowStore,
		Now:           func() time.Time { return now },
	}

	plan1 := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		Title:      "First",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v1, err := svc.DraftVersion(ctx, plan1, plans.DraftVersionInput{
		WorkflowID:             "wf-1",
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-a",
	})
	require.NoError(t, err)
	require.Equal(t, 1, v1.Version)
	require.Nil(t, v1.ParentVersion)

	active, err := svc.ActivateVersion(ctx, "wf-1", 1)
	require.NoError(t, err)
	require.Equal(t, archaeodomain.LivingPlanVersionActive, active.Status)

	plan2 := &frameworkplan.LivingPlan{
		ID:         "plan-2",
		WorkflowID: "wf-1",
		Title:      "Second",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-2": {ID: "step-2", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-2"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v2, err := svc.DraftVersion(ctx, plan2, plans.DraftVersionInput{
		WorkflowID:             "wf-1",
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-b",
		SemanticSnapshotRef:    "snap-2",
	})
	require.NoError(t, err)
	require.Equal(t, 2, v2.Version)
	require.NotNil(t, v2.ParentVersion)
	require.Equal(t, 1, *v2.ParentVersion)

	active, err = svc.ActivateVersion(ctx, "wf-1", 2)
	require.NoError(t, err)
	require.Equal(t, 2, active.Version)
	require.Equal(t, archaeodomain.LivingPlanVersionActive, active.Status)

	versions, err := svc.ListVersions(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, archaeodomain.LivingPlanVersionSuperseded, versions[0].Status)
	require.Equal(t, archaeodomain.LivingPlanVersionActive, versions[1].Status)

	loaded, err := svc.LoadActiveContext(ctx, "wf-1", &core.Task{Context: map[string]any{"current_step_id": "step-2"}})
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "plan-2", loaded.Plan.ID)
	require.Equal(t, 2, loaded.Plan.Version)
	require.NotNil(t, loaded.Step)
	require.Equal(t, "step-2", loaded.Step.ID)
}

func TestEnsureActiveVersionCreatesVersionedHandoffForExecution(t *testing.T) {
	now := time.Date(2026, 3, 27, 1, 0, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-handoff",
		TaskID:      "task-handoff",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{
		Store:         store,
		WorkflowStore: workflowStore,
		Now:           func() time.Time { return now },
	}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-handoff",
		WorkflowID: "wf-handoff",
		Title:      "Handoff",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	record, err := svc.EnsureActiveVersion(ctx, "wf-handoff", plan, plans.DraftVersionInput{
		WorkflowID:             "wf-handoff",
		DerivedFromExploration: "explore-handoff",
		BasedOnRevision:        "rev-handoff",
	})
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, archaeodomain.LivingPlanVersionActive, record.Status)
	require.Equal(t, 1, record.Version)
}

func TestPlanVersionCanBeMarkedStaleArchivedAndCompared(t *testing.T) {
	now := time.Date(2026, 3, 27, 2, 0, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-compare",
		TaskID:      "task-compare",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{
		Store:         store,
		WorkflowStore: workflowStore,
		Now:           func() time.Time { return now },
	}

	first := &frameworkplan.LivingPlan{
		ID:         "plan-a",
		WorkflowID: "wf-compare",
		Title:      "A",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = svc.DraftVersion(ctx, first, plans.DraftVersionInput{
		WorkflowID:  "wf-compare",
		PatternRefs: []string{"pattern-a"},
	})
	require.NoError(t, err)
	_, err = svc.ActivateVersion(ctx, "wf-compare", 1)
	require.NoError(t, err)

	second := &frameworkplan.LivingPlan{
		ID:         "plan-b",
		WorkflowID: "wf-compare",
		Title:      "B",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
			"step-2": {ID: "step-2", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1", "step-2"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = svc.DraftVersion(ctx, second, plans.DraftVersionInput{
		WorkflowID:  "wf-compare",
		PatternRefs: []string{"pattern-a", "pattern-b"},
		AnchorRefs:  []string{"anchor-b"},
	})
	require.NoError(t, err)
	_, err = svc.ActivateVersion(ctx, "wf-compare", 2)
	require.NoError(t, err)

	stale, err := svc.MarkVersionStale(ctx, "wf-compare", 2, "workspace drift")
	require.NoError(t, err)
	require.True(t, stale.RecomputeRequired)
	require.Equal(t, "workspace drift", stale.StaleReason)

	mutations, err := archaeoevents.ReadMutationEvents(ctx, workflowStore, "wf-compare")
	require.NoError(t, err)
	require.NotEmpty(t, mutations)
	require.Equal(t, archaeodomain.MutationPlanStaleness, mutations[len(mutations)-1].Category)

	archived, err := svc.ArchiveVersion(ctx, "wf-compare", 1, "obsolete")
	require.NoError(t, err)
	require.Equal(t, archaeodomain.LivingPlanVersionArchived, archived.Status)

	diff, err := svc.CompareVersions(ctx, "wf-compare", 1, 2)
	require.NoError(t, err)
	require.NotNil(t, diff)
	require.Equal(t, 1, diff["step_count_delta"])
	require.Contains(t, diff["pattern_refs_added"].([]string), "pattern-b")
	require.Contains(t, diff["anchor_refs_added"].([]string), "anchor-b")
	require.Contains(t, diff["step_ids_added"].([]string), "step-2")
	stepDiffs := diff["step_diffs"].(map[string]any)
	require.Contains(t, stepDiffs, "step-2")
}

func TestCompareVersionsIncludesModifiedAndRemovedStepDetails(t *testing.T) {
	now := time.Date(2026, 3, 27, 2, 30, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-steps",
		TaskID:      "task-steps",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}

	first := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-steps",
		Title:      "first",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "old", Scope: []string{"a"}, CreatedAt: now, UpdatedAt: now},
			"step-2": {ID: "step-2", Description: "remove me", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1", "step-2"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = svc.DraftVersion(ctx, first, plans.DraftVersionInput{WorkflowID: "wf-steps"})
	require.NoError(t, err)

	second := &frameworkplan.LivingPlan{
		ID:         "plan-2",
		WorkflowID: "wf-steps",
		Title:      "second",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "new", Scope: []string{"b"}, CreatedAt: now, UpdatedAt: now},
			"step-3": {ID: "step-3", Description: "added", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1", "step-3"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = svc.DraftVersion(ctx, second, plans.DraftVersionInput{WorkflowID: "wf-steps"})
	require.NoError(t, err)

	diff, err := svc.CompareVersions(ctx, "wf-steps", 1, 2)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"step-3"}, diff["step_ids_added"].([]string))
	require.ElementsMatch(t, []string{"step-2"}, diff["step_ids_removed"].([]string))
	require.ElementsMatch(t, []string{"step-1"}, diff["step_ids_changed"].([]string))
	stepDiffs := diff["step_diffs"].(map[string]any)
	require.Contains(t, stepDiffs, "step-1")
	require.Contains(t, stepDiffs, "step-2")
	require.Contains(t, stepDiffs, "step-3")
}

func TestEnsureDraftSuccessorClonesFromStaleActiveVersion(t *testing.T) {
	now := time.Date(2026, 3, 27, 2, 45, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-successor",
		TaskID:      "task-successor",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}

	base := &frameworkplan.LivingPlan{
		ID:         "plan-successor",
		WorkflowID: "wf-successor",
		Title:      "base",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Description: "keep", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v1, err := svc.DraftVersion(ctx, base, plans.DraftVersionInput{
		WorkflowID:             "wf-successor",
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-1",
		PatternRefs:            []string{"pattern-a"},
	})
	require.NoError(t, err)
	_, err = svc.ActivateVersion(ctx, "wf-successor", v1.Version)
	require.NoError(t, err)
	_, err = svc.MarkVersionStale(ctx, "wf-successor", v1.Version, "pattern changed")
	require.NoError(t, err)

	successor, err := svc.EnsureDraftSuccessor(ctx, "wf-successor", 1, "pattern changed")
	require.NoError(t, err)
	require.NotNil(t, successor)
	require.Equal(t, archaeodomain.LivingPlanVersionDraft, successor.Status)
	require.NotNil(t, successor.ParentVersion)
	require.Equal(t, 1, *successor.ParentVersion)
	require.Equal(t, "explore-1", successor.DerivedFromExploration)
	require.Equal(t, "rev-1", successor.BasedOnRevision)
	require.Contains(t, successor.StaleReason, "pattern changed")
	require.NotEqual(t, "plan-successor", successor.Plan.ID)
	require.Equal(t, []string{"pattern-a"}, successor.PatternRefs)

	mutations, err := archaeoevents.ReadMutationEvents(ctx, workflowStore, "wf-successor")
	require.NoError(t, err)
	require.Len(t, mutations, 2)
	require.Equal(t, archaeodomain.MutationPlanStaleness, mutations[0].Category)
	require.Equal(t, archaeodomain.DispositionRequireReplan, mutations[0].Disposition)
	require.Equal(t, archaeodomain.MutationPlanStaleness, mutations[1].Category)
	require.Equal(t, archaeodomain.DispositionContinueOnStalePlan, mutations[1].Disposition)
}

func TestSyncActiveVersionWithExplorationCreatesAlignedDraftSuccessor(t *testing.T) {
	now := time.Date(2026, 3, 27, 3, 15, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-align",
		TaskID:      "task-align",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}

	base := &frameworkplan.LivingPlan{
		ID:         "plan-align",
		WorkflowID: "wf-align",
		Title:      "base",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v1, err := svc.DraftVersion(ctx, base, plans.DraftVersionInput{
		WorkflowID:             "wf-align",
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    "snapshot-old",
		PatternRefs:            []string{"pattern-old"},
		AnchorRefs:             []string{"anchor-old"},
	})
	require.NoError(t, err)
	_, err = svc.ActivateVersion(ctx, "wf-align", v1.Version)
	require.NoError(t, err)

	draft, err := svc.SyncActiveVersionWithExploration(ctx, "wf-align", &archaeodomain.ExplorationSnapshot{
		ID:                   "snapshot-new",
		ExplorationID:        "explore-1",
		WorkflowID:           "wf-align",
		BasedOnRevision:      "rev-2",
		SemanticSnapshotRef:  "snapshot-new",
		CandidatePatternRefs: []string{"pattern-new"},
		CandidateAnchorRefs:  []string{"anchor-new"},
		TensionIDs:           []string{"tension-1"},
	})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.Equal(t, archaeodomain.LivingPlanVersionDraft, draft.Status)
	require.NotNil(t, draft.ParentVersion)
	require.Equal(t, 1, *draft.ParentVersion)
	require.Equal(t, "rev-2", draft.BasedOnRevision)
	require.Equal(t, "snapshot-new", draft.SemanticSnapshotRef)
	require.Equal(t, []string{"pattern-new"}, draft.PatternRefs)
	require.Equal(t, []string{"anchor-new"}, draft.AnchorRefs)
	require.Equal(t, []string{"tension-1"}, draft.TensionRefs)

	active, err := svc.LoadActiveVersion(ctx, "wf-align")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.True(t, active.RecomputeRequired)
	require.Contains(t, active.StaleReason, "candidate patterns changed")
}

func TestEnsureDraftFromExplorationFormsInitialDraftPlan(t *testing.T) {
	now := time.Date(2026, 3, 27, 4, 0, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-form",
		TaskID:      "task-form",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "form plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}

	record, err := svc.EnsureDraftFromExploration(ctx, plans.FormationInput{
		WorkflowID:       "wf-form",
		ExplorationID:    "explore-1",
		SnapshotID:       "snapshot-1",
		BasedOnRevision:  "rev-1",
		SemanticSnapshot: "snapshot-1",
		PatternRefs:      []string{"pattern-a", "pattern-b"},
		AnchorRefs:       []string{"anchor-a"},
		TensionRefs:      []string{"tension-a"},
		PendingLearning:  []string{"learn-1"},
	})
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, archaeodomain.LivingPlanVersionDraft, record.Status)
	require.Equal(t, "explore-1", record.DerivedFromExploration)
	require.Equal(t, []string{"pattern-a", "pattern-b"}, record.PatternRefs)
	require.Equal(t, []string{"anchor-a"}, record.AnchorRefs)
	require.Equal(t, []string{"tension-a"}, record.TensionRefs)
	require.Equal(t, 4, len(record.Plan.StepOrder))
	require.Contains(t, record.Plan.Steps, "resolve_learning")
	require.Contains(t, record.Plan.Steps, "resolve_tensions")
	require.Contains(t, record.Plan.Steps, "ground_findings")
	require.Contains(t, record.Plan.Steps, "advance_execution")
	require.Equal(t, []string{"resolve_learning"}, record.Plan.Steps["resolve_tensions"].DependsOn)
	require.Equal(t, []string{"resolve_tensions"}, record.Plan.Steps["ground_findings"].DependsOn)
	require.Equal(t, []string{"ground_findings"}, record.Plan.Steps["advance_execution"].DependsOn)
	require.NotNil(t, record.Plan.ConvergenceTarget)
	require.Equal(t, []string{"pattern-a", "pattern-b"}, record.Plan.ConvergenceTarget.PatternIDs)
	require.Equal(t, []string{"tension-a"}, record.Plan.ConvergenceTarget.TensionIDs)
}

func TestEnsureDraftFromExplorationMarksActiveVersionStaleAndCreatesDraft(t *testing.T) {
	now := time.Date(2026, 3, 27, 4, 15, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-form-stale",
		TaskID:      "task-form-stale",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "form plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}

	base := &frameworkplan.LivingPlan{
		ID:         "plan-form-base",
		WorkflowID: "wf-form-stale",
		Title:      "base",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v1, err := svc.DraftVersion(ctx, base, plans.DraftVersionInput{
		WorkflowID:             "wf-form-stale",
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    "snapshot-old",
		PatternRefs:            []string{"pattern-old"},
	})
	require.NoError(t, err)
	_, err = svc.ActivateVersion(ctx, "wf-form-stale", v1.Version)
	require.NoError(t, err)

	draft, err := svc.EnsureDraftFromExploration(ctx, plans.FormationInput{
		WorkflowID:       "wf-form-stale",
		ExplorationID:    "explore-1",
		SnapshotID:       "snapshot-new",
		BasedOnRevision:  "rev-2",
		SemanticSnapshot: "snapshot-new",
		PatternRefs:      []string{"pattern-new"},
		AnchorRefs:       []string{"anchor-new"},
	})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.Equal(t, archaeodomain.LivingPlanVersionDraft, draft.Status)
	require.NotNil(t, draft.ParentVersion)
	require.Equal(t, 1, *draft.ParentVersion)

	active, err := svc.LoadActiveVersion(ctx, "wf-form-stale")
	require.NoError(t, err)
	require.NotNil(t, active)
	require.True(t, active.RecomputeRequired)
	require.Contains(t, active.StaleReason, "formation inputs changed")
}

func TestApplyInvalidationWithExcludeStepID(t *testing.T) {
	now := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)
	svc := plans.Service{Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:        "plan-1",
		CreatedAt: now,
		UpdatedAt: now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
			"step-2": {
				ID:        "step-2",
				Status:    frameworkplan.PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
				InvalidatedBy: []frameworkplan.InvalidationRule{{
					Kind:   frameworkplan.InvalidationSymbolChanged,
					Target: "sym",
				}},
			},
		},
	}
	invalidated := svc.ApplyInvalidation(plan, frameworkplan.InvalidationEvent{
		Kind:   frameworkplan.InvalidationSymbolChanged,
		Target: "sym",
		At:     now,
	}, "step-2")
	require.Empty(t, invalidated)
	require.Equal(t, frameworkplan.PlanStepPending, plan.Steps["step-2"].Status)
}

func TestApplyAnchorInvalidations(t *testing.T) {
	now := time.Date(2026, 3, 28, 1, 0, 0, 0, time.UTC)
	svc := plans.Service{Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:        "plan-1",
		CreatedAt: now,
		UpdatedAt: now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
			"step-2": {
				ID:        "step-2",
				Status:    frameworkplan.PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
				InvalidatedBy: []frameworkplan.InvalidationRule{{
					Kind:   frameworkplan.InvalidationAnchorDrifted,
					Target: "anchor1",
				}},
			},
		},
	}
	changed := svc.ApplyAnchorInvalidations(plan, "step-1", []string{"anchor1"})
	require.Equal(t, []string{"step-2"}, changed)
	require.Equal(t, frameworkplan.PlanStepInvalidated, plan.Steps["step-2"].Status)
}

func TestApplyScopeInvalidations(t *testing.T) {
	now := time.Date(2026, 3, 28, 2, 0, 0, 0, time.UTC)
	svc := plans.Service{Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:        "plan-1",
		CreatedAt: now,
		UpdatedAt: now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				Scope:     []string{"symbolA"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			"step-2": {
				ID:        "step-2",
				Status:    frameworkplan.PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
				InvalidatedBy: []frameworkplan.InvalidationRule{{
					Kind:   frameworkplan.InvalidationSymbolChanged,
					Target: "symbolA",
				}},
			},
		},
	}
	changed := svc.ApplyScopeInvalidations(plan, plan.Steps["step-1"])
	require.Equal(t, []string{"step-2"}, changed)
	require.Equal(t, frameworkplan.PlanStepInvalidated, plan.Steps["step-2"].Status)
	require.False(t, plan.UpdatedAt.IsZero())
}

func TestLoadActiveContextNilStore(t *testing.T) {
	svc := plans.Service{Store: nil}
	ctx, err := svc.LoadActiveContext(context.Background(), "wf-none", &core.Task{Context: map[string]any{"current_step_id": "step"}})
	require.NoError(t, err)
	require.Nil(t, ctx)
}

func TestLoadActiveContextNoPlan(t *testing.T) {
	store := &stubPlanStore{plan: nil}
	svc := plans.Service{Store: store}
	ctx, err := svc.LoadActiveContext(context.Background(), "wf-none", &core.Task{Context: map[string]any{"current_step_id": "step"}})
	require.NoError(t, err)
	require.Nil(t, ctx)
}

func TestPersistAllSteps(t *testing.T) {
	now := time.Date(2026, 3, 28, 3, 0, 0, 0, time.UTC)
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		CreatedAt:  now,
		UpdatedAt:  now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
			"step-2": {ID: "step-2", CreatedAt: now, UpdatedAt: now},
		},
	}
	err := svc.PersistAllSteps(context.Background(), plan)
	require.NoError(t, err)
	require.Contains(t, store.updates, "step-1")
	require.Contains(t, store.updates, "step-2")
}

func TestRecordBlockedStepWithoutInvalidate(t *testing.T) {
	now := time.Date(2026, 3, 28, 4, 0, 0, 0, time.UTC)
	svc := plans.Service{Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:        "plan-1",
		CreatedAt: now,
		UpdatedAt: now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	svc.RecordBlockedStep(plan, plan.Steps["step-1"], "blocked but not invalidated", false)
	require.Equal(t, frameworkplan.PlanStepPending, plan.Steps["step-1"].Status)
	require.Len(t, plan.Steps["step-1"].History, 1)
	require.Equal(t, "blocked", plan.Steps["step-1"].History[0].Outcome)
}

func TestPersistPreflightBlockedWithInvalidatedSteps(t *testing.T) {
	now := time.Date(2026, 3, 28, 5, 0, 0, 0, time.UTC)
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		CreatedAt:  now,
		UpdatedAt:  now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
			"step-2": {ID: "step-2", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
	}
	result, err := svc.PersistPreflightBlocked(context.Background(), plan, plan.Steps["step-1"], "blocked test", true, []string{"step-2"})
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Equal(t, frameworkplan.PlanStepInvalidated, plan.Steps["step-1"].Status)
	require.Contains(t, store.updates, "step-1")
	// step-2 should also have been persisted because invalidatedStepIDs non‑empty
	require.Contains(t, store.updates, "step-2")
}

func TestPersistPreflightShortCircuitWithError(t *testing.T) {
	now := time.Date(2026, 3, 28, 6, 0, 0, 0, time.UTC)
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		CreatedAt:  now,
		UpdatedAt:  now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
	}
	origErr := fmt.Errorf("original error")
	result, err := svc.PersistPreflightShortCircuit(context.Background(), plan, plan.Steps["step-1"], &core.Result{Success: false}, origErr)
	require.Error(t, err)
	require.Equal(t, origErr, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, store.updates, "step-1")
}

func TestActiveStepVariousInputs(t *testing.T) {
	now := time.Date(2026, 3, 28, 7, 0, 0, 0, time.UTC)
	plan := &frameworkplan.LivingPlan{
		ID:        "plan-1",
		CreatedAt: now,
		UpdatedAt: now,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
	}
	// nil task
	require.Nil(t, plans.ActiveStep(nil, plan))
	// nil plan
	require.Nil(t, plans.ActiveStep(&core.Task{Context: map[string]any{"current_step_id": "step-1"}}, nil))
	// empty context
	require.Nil(t, plans.ActiveStep(&core.Task{Context: map[string]any{}}, plan))
	// non‑string value
	require.Nil(t, plans.ActiveStep(&core.Task{Context: map[string]any{"current_step_id": 123}}, plan))
	// valid step id
	step := plans.ActiveStep(&core.Task{Context: map[string]any{"current_step_id": "step-1"}}, plan)
	require.NotNil(t, step)
	require.Equal(t, "step-1", step.ID)
}


func TestEnsureDraftSuccessorNotFound(t *testing.T) {
	now := time.Date(2026, 3, 28, 8, 0, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-notfound",
		TaskID:      "task-notfound",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}
	// no version exists
	successor, err := svc.EnsureDraftSuccessor(ctx, "wf-notfound", 1, "reason")
	require.Error(t, err)
	require.Nil(t, successor)
}

func TestSyncActiveVersionWithExplorationNoChanges(t *testing.T) {
	now := time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-nochange",
		TaskID:      "task-nochange",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-nochange",
		WorkflowID: "wf-nochange",
		Title:      "base",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v1, err := svc.DraftVersion(ctx, plan, plans.DraftVersionInput{
		WorkflowID:             "wf-nochange",
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    "snap-1",
		PatternRefs:            []string{"pattern-a"},
		AnchorRefs:             []string{"anchor-a"},
		TensionRefs:            []string{"tension-a"},
	})
	require.NoError(t, err)
	_, err = svc.ActivateVersion(ctx, "wf-nochange", v1.Version)
	require.NoError(t, err)

	// snapshot with identical data
	draft, err := svc.SyncActiveVersionWithExploration(ctx, "wf-nochange", &archaeodomain.ExplorationSnapshot{
		ID:                   "snap-1",
		ExplorationID:        "explore-1",
		WorkflowID:           "wf-nochange",
		BasedOnRevision:      "rev-1",
		SemanticSnapshotRef:  "snap-1",
		CandidatePatternRefs: []string{"pattern-a"},
		CandidateAnchorRefs:  []string{"anchor-a"},
		TensionIDs:           []string{"tension-a"},
	})
	require.NoError(t, err)
	require.Nil(t, draft) // should return nil when no changes
}

func TestEnsureDraftFromExplorationNoChanges(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	ctx := context.Background()
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-nochange-form",
		TaskID:      "task-nochange-form",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := &stubPlanStore{}
	svc := plans.Service{Store: store, WorkflowStore: workflowStore, Now: func() time.Time { return now }}
	// create an active version first
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-nochange-form",
		WorkflowID: "wf-nochange-form",
		Title:      "base",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	v1, err := svc.DraftVersion(ctx, plan, plans.DraftVersionInput{
		WorkflowID:             "wf-nochange-form",
		DerivedFromExploration: "explore-1",
		BasedOnRevision:        "rev-1",
		SemanticSnapshotRef:    "snap-1",
		PatternRefs:            []string{"pattern-a"},
		AnchorRefs:             []string{"anchor-a"},
		TensionRefs:            []string{"tension-a"},
	})
	require.NoError(t, err)
	_, err = svc.ActivateVersion(ctx, "wf-nochange-form", v1.Version)
	require.NoError(t, err)

	// call EnsureDraftFromExploration with identical inputs (active matches)
	record, err := svc.EnsureDraftFromExploration(ctx, plans.FormationInput{
		WorkflowID:       "wf-nochange-form",
		ExplorationID:    "explore-1",
		SnapshotID:       "snap-1",
		BasedOnRevision:  "rev-1",
		SemanticSnapshot: "snap-1",
		PatternRefs:      []string{"pattern-a"},
		AnchorRefs:       []string{"anchor-a"},
		TensionRefs:      []string{"tension-a"},
	})
	require.NoError(t, err)
	require.Nil(t, record) // should return nil when active already matches
}

func TestEnsureDraftFromExplorationNilWorkflowStore(t *testing.T) {
	svc := plans.Service{WorkflowStore: nil}
	record, err := svc.EnsureDraftFromExploration(context.Background(), plans.FormationInput{
		WorkflowID: "wf-none",
	})
	require.NoError(t, err)
	require.Nil(t, record)
}

package archaeology_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/archaeo/archaeology"
	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoexec "codeburg.org/lexbit/relurpify/archaeo/execution"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	"codeburg.org/lexbit/relurpify/archaeo/providers"
	archaeorequests "codeburg.org/lexbit/relurpify/archaeo/requests"
	archaeoretrieval "codeburg.org/lexbit/relurpify/archaeo/retrieval"
	archaeotensions "codeburg.org/lexbit/relurpify/archaeo/tensions"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type stubPatternSurfacer struct {
	records []patterns.PatternRecord
}

func (s stubPatternSurfacer) SurfacePatterns(context.Context, providers.PatternSurfacingRequest) ([]patterns.PatternRecord, error) {
	return append([]patterns.PatternRecord(nil), s.records...), nil
}

type stubTensionAnalyzer struct {
	records []archaeodomain.Tension
}

func (s stubTensionAnalyzer) AnalyzeTensions(context.Context, providers.TensionAnalysisRequest) ([]archaeodomain.Tension, error) {
	return append([]archaeodomain.Tension(nil), s.records...), nil
}

type stubProspectiveAnalyzer struct {
	records []patterns.PatternRecord
}

func (s stubProspectiveAnalyzer) AnalyzeProspective(context.Context, providers.ProspectiveAnalysisRequest) ([]patterns.PatternRecord, error) {
	return append([]patterns.PatternRecord(nil), s.records...), nil
}

type stubConvergenceReviewer struct {
	failure *frameworkplan.ConvergenceFailure
}

func (s stubConvergenceReviewer) ReviewConvergence(context.Context, providers.ConvergenceReviewRequest) (*frameworkplan.ConvergenceFailure, error) {
	if s.failure == nil {
		return nil, nil
	}
	copy := *s.failure
	copy.UnconfirmedPatterns = append([]string(nil), s.failure.UnconfirmedPatterns...)
	copy.UnresolvedTensions = append([]string(nil), s.failure.UnresolvedTensions...)
	return &copy, nil
}

type stubPlanStore struct {
	plan    *frameworkplan.LivingPlan
	updates map[string]*frameworkplan.PlanStep
}

func (s *stubPlanStore) SavePlan(context.Context, *frameworkplan.LivingPlan) error { return nil }
func (s *stubPlanStore) LoadPlan(context.Context, string) (*frameworkplan.LivingPlan, error) {
	return nil, nil
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

func TestPrepareLivingPlanHandlesNoActiveStep(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps:      map[string]*frameworkplan.PlanStep{},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	var phases []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phases = append(phases, phase)
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}

	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{}, state, "wf-1")
	require.NoError(t, result.Err)
	require.NotNil(t, result.Plan)
	require.Nil(t, result.Step)
	require.Equal(t, []archaeodomain.EucloPhase{archaeodomain.PhaseSurfacing}, phases)
}

func TestPrepareLivingPlanPersistsBlockedPreflight(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	var resetCalled bool
	var phases []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store, Now: func() time.Time { return now }},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phases = append(phases, phase)
		},
		ResetDoom: func() { resetCalled = true },
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{ShouldInvalidate: true}, context.DeadlineExceeded
		},
	}

	task := &core.Task{Context: map[string]any{"current_step_id": "step-1"}}
	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-1")
	require.Error(t, result.Err)
	require.True(t, resetCalled)
	require.NotNil(t, result.Result)
	require.False(t, result.Result.Success)
	require.Equal(t, frameworkplan.PlanStepInvalidated, store.plan.Steps["step-1"].Status)
	require.Equal(t, []archaeodomain.EucloPhase{archaeodomain.PhaseBlocked}, phases)
}

func TestPrepareLivingPlanTransitionsToIntentElicitationWhenLearningPending(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps:      map[string]*frameworkplan.PlanStep{},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, _ := openPatternStores(t)
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-1",
		Kind:         patterns.PatternKindStructural,
		Title:        "Adapter",
		Description:  "Use adapters",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    now,
		UpdatedAt:    now,
	}))
	var phasesSeen []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store},
		Learning: archaeolearning.Service{
			Store:        workflowStore,
			PatternStore: patternStore,
		},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phasesSeen = append(phasesSeen, phase)
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}

	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{
		Context: map[string]any{
			"workspace":    "/tmp/ws",
			"workflow_id":  "wf-1",
			"corpus_scope": "workspace",
		},
	}, state, "wf-1")
	require.NoError(t, result.Err)
	require.NotNil(t, result.Plan)
	require.Nil(t, result.Step)
	require.Equal(t, []archaeodomain.EucloPhase{archaeodomain.PhaseIntentElicitation}, phasesSeen)

	raw, ok := state.Get("euclo.learning_queue")
	require.True(t, ok)
	queue, ok := raw.([]archaeolearning.Interaction)
	require.True(t, ok)
	require.Len(t, queue, 1)
	require.Equal(t, "pattern-1", queue[0].SubjectID)
	require.NotEmpty(t, state.GetString("euclo.active_exploration_id"))
	require.NotEmpty(t, state.GetString("euclo.active_exploration_snapshot_id"))
}

func TestLoadExplorationViewIncludesTensions(t *testing.T) {
	ctx := context.Background()
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-view",
		TaskID:      "task-view",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "explore",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := archaeology.Service{Store: workflowStore}
	session, err := svc.EnsureExplorationSession(ctx, "wf-view", "/workspace/view", "rev-1")
	require.NoError(t, err)
	snapshot, err := svc.CreateExplorationSnapshot(ctx, session, archaeology.SnapshotInput{
		WorkflowID:      "wf-view",
		WorkspaceID:     "/workspace/view",
		BasedOnRevision: "rev-1",
	})
	require.NoError(t, err)
	_, err = (archaeotensions.Service{Store: workflowStore}).CreateOrUpdate(ctx, archaeotensions.CreateInput{
		WorkflowID:    "wf-view",
		ExplorationID: session.ID,
		SnapshotID:    snapshot.ID,
		SourceRef:     "gap-1",
		Kind:          "intent_gap",
		Description:   "Boundary mismatch",
		Status:        archaeodomain.TensionUnresolved,
	})
	require.NoError(t, err)

	view, err := svc.LoadExplorationView(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, view)
	require.Len(t, view.Tensions, 1)
	require.Equal(t, session.ID, view.Tensions[0].ExplorationID)
	require.NotNil(t, view.TensionSummary)
	require.Equal(t, 1, view.TensionSummary.Total)
	require.Equal(t, 1, view.TensionSummary.Unresolved)
}

func TestPrepareLivingPlanAddsAnchorDriftLearningToQueue(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps:      map[string]*frameworkplan.PlanStep{},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	retrievalDB := openRetrievalDB(t)
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "compatibility",
		Definition: "preserve wire format",
		Class:      "policy",
	}, "workspace", "workspace_trusted")
	require.NoError(t, err)
	require.NoError(t, retrieval.RecordAnchorDrift(context.Background(), retrievalDB, anchor.AnchorID, "critical", "major drift"))

	var phasesSeen []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store},
		Learning: archaeolearning.Service{
			Store:     workflowStore,
			Retrieval: archaeoretrieval.NewSQLStore(retrievalDB),
		},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phasesSeen = append(phasesSeen, phase)
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}

	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{
		Context: map[string]any{
			"workspace":    "/tmp/ws",
			"workflow_id":  "wf-1",
			"corpus_scope": "workspace",
		},
	}, state, "wf-1")
	require.NoError(t, result.Err)
	require.Equal(t, []archaeodomain.EucloPhase{archaeodomain.PhaseIntentElicitation}, phasesSeen)

	raw, ok := state.Get("euclo.learning_queue")
	require.True(t, ok)
	queue, ok := raw.([]archaeolearning.Interaction)
	require.True(t, ok)
	require.Len(t, queue, 1)
	require.Equal(t, archaeolearning.SubjectAnchor, queue[0].SubjectType)
	require.Equal(t, anchor.AnchorID, queue[0].SubjectID)
}

func TestEnsureExplorationSessionIsWorkspaceScopedAcrossWorkflows(t *testing.T) {
	store := newWorkflowStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "scan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "scan again",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := archaeology.Service{Store: store}

	first, err := svc.EnsureExplorationSession(ctx, "wf-1", "/workspace/a", "rev-1")
	require.NoError(t, err)
	second, err := svc.EnsureExplorationSession(ctx, "wf-2", "/workspace/a", "rev-2")
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "rev-2", second.BasedOnRevision)
}

func TestLoadExplorationByWorkflowUsesLatestSnapshotWorkflowLink(t *testing.T) {
	store := newWorkflowStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "scan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "scan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := archaeology.Service{Store: store}
	session, err := svc.EnsureExplorationSession(ctx, "wf-1", "/workspace/a", "rev-1")
	require.NoError(t, err)
	_, err = svc.CreateExplorationSnapshot(ctx, session, archaeology.SnapshotInput{
		WorkflowID:      "wf-2",
		WorkspaceID:     "/workspace/a",
		BasedOnRevision: "rev-2",
	})
	require.NoError(t, err)

	loaded, err := svc.LoadExplorationByWorkflow(ctx, "wf-2")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, session.ID, loaded.ID)
}

func TestCreateExplorationSnapshotProducesDatedUniqueKeyAndUpdatesSession(t *testing.T) {
	now := time.Date(2026, 3, 26, 23, 15, 0, 0, time.UTC)
	store := newWorkflowStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "scan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := archaeology.Service{
		Store:    store,
		Now:      func() time.Time { return now },
		Learning: archaeolearning.Service{Store: store},
	}
	session, err := svc.EnsureExplorationSession(ctx, "wf-1", "/workspace/a", "rev-1")
	require.NoError(t, err)
	snapshot, err := svc.CreateExplorationSnapshot(ctx, session, archaeology.SnapshotInput{
		WorkflowID:      "wf-1",
		WorkspaceID:     "/workspace/a",
		BasedOnRevision: "rev-1",
		Summary:         "scan workspace",
	})
	require.NoError(t, err)
	require.Contains(t, snapshot.SnapshotKey, "20260326T231500")
	require.Equal(t, session.ID, snapshot.ExplorationID)

	updated, err := svc.EnsureExplorationSession(ctx, "wf-1", "/workspace/a", "rev-1")
	require.NoError(t, err)
	require.Equal(t, snapshot.ID, updated.LatestSnapshotID)
	require.Contains(t, updated.SnapshotIDs, snapshot.ID)

	view, err := svc.LoadExplorationView(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, view)
	require.NotNil(t, view.Session)
	require.Len(t, view.Snapshots, 1)
}

func TestMarkExplorationStaleSetsRecomputeFlags(t *testing.T) {
	store := newWorkflowStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "scan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := archaeology.Service{Store: store}
	session, err := svc.EnsureExplorationSession(ctx, "wf-1", "/workspace/a", "rev-1")
	require.NoError(t, err)

	stale, err := svc.MarkExplorationStale(ctx, session.ID, "workspace changed")
	require.NoError(t, err)
	require.Equal(t, archaeodomain.ExplorationStatusStale, stale.Status)
	require.True(t, stale.RecomputeRequired)
	require.Equal(t, "workspace changed", stale.StaleReason)
}

func TestEnsureExplorationSessionMarksRevisionDriftStale(t *testing.T) {
	store := newWorkflowStore(t)
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "scan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := archaeology.Service{Store: store}
	_, err := svc.EnsureExplorationSession(ctx, "wf-1", "/workspace/a", "rev-1")
	require.NoError(t, err)

	drifted, err := svc.EnsureExplorationSession(ctx, "wf-1", "/workspace/a", "rev-2")
	require.NoError(t, err)
	require.True(t, drifted.RecomputeRequired)
	require.Equal(t, archaeodomain.ExplorationStatusStale, drifted.Status)
	require.Contains(t, drifted.StaleReason, "revision changed")
}

func TestPrepareLivingPlanRefreshesSnapshotCandidates(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps:      map[string]*frameworkplan.PlanStep{},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, _ := openPatternStores(t)
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-candidate",
		Kind:         patterns.PatternKindStructural,
		Title:        "Candidate",
		Description:  "candidate pattern",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    now,
		UpdatedAt:    now,
	}))
	retrievalDB := openRetrievalDB(t)
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "compatibility",
		Definition: "preserve wire format",
		Class:      "policy",
	}, "workspace", "workspace_trusted")
	require.NoError(t, err)
	require.NoError(t, retrieval.RecordAnchorDrift(context.Background(), retrievalDB, anchor.AnchorID, "high", "drifted"))

	svc := archaeology.Service{
		Store: workflowStore,
		Plans: archaeoplans.Service{Store: store, WorkflowStore: workflowStore},
		Learning: archaeolearning.Service{
			Store:        workflowStore,
			PatternStore: patternStore,
			Retrieval:    archaeoretrieval.NewSQLStore(retrievalDB),
		},
		PersistPhase: func(context.Context, *core.Task, *core.Context, archaeodomain.EucloPhase, string, *frameworkplan.PlanStep) {
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}

	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{
		Context: map[string]any{
			"workspace":    "/tmp/ws",
			"workflow_id":  "wf-1",
			"corpus_scope": "workspace",
		},
	}, state, "wf-1")
	require.NoError(t, result.Err)

	patternRefs, ok := state.Get("euclo.exploration_candidate_pattern_refs")
	require.True(t, ok)
	require.Contains(t, patternRefs.([]string), "pattern-candidate")

	anchorRefs, ok := state.Get("euclo.exploration_candidate_anchor_refs")
	require.True(t, ok)
	require.Contains(t, anchorRefs.([]string), anchor.AnchorID)
	draftVersion, ok := state.Get("euclo.draft_plan_version")
	require.True(t, ok)
	require.Equal(t, 2, draftVersion)
	recomputeRequired, ok := state.Get("euclo.plan_recompute_required")
	require.True(t, ok)
	require.Equal(t, true, recomputeRequired)
}

func TestPrepareLivingPlanAllowsNonBlockingLearningToSurface(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps:      map[string]*frameworkplan.PlanStep{},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	learnSvc := archaeolearning.Service{
		Store: workflowStore,
	}
	_, err := learnSvc.Create(context.Background(), archaeolearning.CreateInput{
		WorkflowID:    "wf-1",
		ExplorationID: "explore-1",
		Kind:          archaeolearning.InteractionTensionReview,
		SubjectType:   archaeolearning.SubjectTension,
		SubjectID:     "tension-1",
		Title:         "Advisory learning",
		Blocking:      false,
	})
	require.NoError(t, err)

	var phasesSeen []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans:    archaeoplans.Service{Store: store},
		Learning: learnSvc,
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phasesSeen = append(phasesSeen, phase)
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}

	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{
		Context: map[string]any{
			"workspace":    "/tmp/ws",
			"workflow_id":  "wf-1",
			"corpus_scope": "workspace",
		},
	}, state, "wf-1")
	require.NoError(t, result.Err)
	require.Equal(t, []archaeodomain.EucloPhase{archaeodomain.PhaseSurfacing}, phasesSeen)
	rawFlag, ok := state.Get("euclo.has_nonblocking_learning")
	require.True(t, ok)
	flag, ok := rawFlag.(bool)
	require.True(t, ok)
	require.True(t, flag)
	raw, ok := state.Get("euclo.blocking_learning_ids")
	require.True(t, ok)
	require.Empty(t, raw.([]string))
}

func TestPrepareLivingPlanUsesProviderLifecycleAndCreatesPendingRequests(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps:      map[string]*frameworkplan.PlanStep{},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "review structure",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, _ := openPatternStores(t)
	retrievalDB := openRetrievalDB(t)
	anchor, err := retrieval.DeclareAnchor(context.Background(), retrievalDB, retrieval.AnchorDeclaration{
		Term:       "compatibility",
		Definition: "preserve compatibility",
		Class:      "policy",
	}, "workspace", "workspace_trusted")
	require.NoError(t, err)
	require.NoError(t, retrieval.RecordAnchorDrift(context.Background(), retrievalDB, anchor.AnchorID, "high", "drifted"))

	svc := archaeology.Service{
		Store: workflowStore,
		Plans: archaeoplans.Service{Store: store, WorkflowStore: workflowStore},
		Learning: archaeolearning.Service{
			Store:        workflowStore,
			PatternStore: patternStore,
			Retrieval:    archaeoretrieval.NewSQLStore(retrievalDB),
		},
		Requests: archaeorequests.Service{Store: workflowStore},
		Providers: providers.Bundle{
			PatternSurfacer: stubPatternSurfacer{records: []patterns.PatternRecord{{
				ID:           "pattern-provider",
				Kind:         patterns.PatternKindStructural,
				Title:        "Provider Pattern",
				Description:  "surfaced by provider",
				Status:       patterns.PatternStatusProposed,
				CorpusScope:  "workspace",
				CorpusSource: "workspace",
				CreatedAt:    now,
				UpdatedAt:    now,
			}}},
			TensionAnalyzer: stubTensionAnalyzer{records: []archaeodomain.Tension{{
				ID:                 "provider-tension",
				SourceRef:          "gap-provider",
				Kind:               "intent_gap",
				Description:        "provider detected tension",
				Status:             archaeodomain.TensionUnresolved,
				RelatedPlanStepIDs: []string{"resolve_tensions"},
			}}},
			ProspectiveAnalyzer: stubProspectiveAnalyzer{records: []patterns.PatternRecord{{
				ID:           "pattern-prospective",
				Kind:         patterns.PatternKindStructural,
				Title:        "Future Pattern",
				Description:  "prospective",
				Status:       patterns.PatternStatusProposed,
				CorpusScope:  "workspace",
				CorpusSource: "workspace",
				CreatedAt:    now,
				UpdatedAt:    now,
			}}},
			ConvergenceReviewer: stubConvergenceReviewer{failure: &frameworkplan.ConvergenceFailure{
				Description:        "draft still has unresolved tensions",
				UnresolvedTensions: []string{"provider-tension"},
			}},
		},
		PersistPhase: func(context.Context, *core.Task, *core.Context, archaeodomain.EucloPhase, string, *frameworkplan.PlanStep) {
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}

	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{
		Instruction: "review structure",
		Context: map[string]any{
			"workspace":    "/tmp/ws",
			"workflow_id":  "wf-1",
			"corpus_scope": "workspace",
			"symbol_scope": "pkg/service.go",
		},
	}, state, "wf-1")
	require.NoError(t, result.Err)

	requestsSvc := archaeorequests.Service{Store: workflowStore}
	requests, err := requestsSvc.ListByWorkflow(context.Background(), "wf-1")
	require.NoError(t, err)
	require.Len(t, requests, 4)
	require.ElementsMatch(t, []archaeodomain.RequestKind{
		archaeodomain.RequestPatternSurfacing,
		archaeodomain.RequestTensionAnalysis,
		archaeodomain.RequestProspectiveAnalysis,
		archaeodomain.RequestConvergenceReview,
	}, []archaeodomain.RequestKind{requests[0].Kind, requests[1].Kind, requests[2].Kind, requests[3].Kind})
	for _, record := range requests {
		require.Equal(t, archaeodomain.RequestStatusPending, record.Status)
		require.Nil(t, record.Result)
	}

	queueRaw, ok := state.Get("euclo.learning_queue")
	require.True(t, ok)
	queue := queueRaw.([]archaeolearning.Interaction)
	require.NotEmpty(t, queue)
	tensions, err := (archaeotensions.Service{Store: workflowStore}).ListByWorkflow(context.Background(), "wf-1")
	require.NoError(t, err)
	require.Empty(t, tensions)
	_, ok = state.Get("euclo.plan_formation_convergence_failure")
	require.False(t, ok)
}

func TestPrepareLivingPlanCreatesPendingRequestsWithoutProviders(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps:      map[string]*frameworkplan.PlanStep{},
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	workflowStore := newWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "review structure",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	svc := archaeology.Service{
		Store:    workflowStore,
		Plans:    archaeoplans.Service{Store: store, WorkflowStore: workflowStore},
		Learning: archaeolearning.Service{Store: workflowStore},
		Requests: archaeorequests.Service{Store: workflowStore},
		PersistPhase: func(context.Context, *core.Task, *core.Context, archaeodomain.EucloPhase, string, *frameworkplan.PlanStep) {
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}

	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{
		Instruction: "review structure",
		Context: map[string]any{
			"workspace":    "/tmp/ws",
			"workflow_id":  "wf-1",
			"corpus_scope": "workspace",
			"symbol_scope": "pkg/service.go",
		},
	}, state, "wf-1")
	require.NoError(t, result.Err)

	requestsSvc := archaeorequests.Service{Store: workflowStore}
	requests, err := requestsSvc.Pending(context.Background(), "wf-1")
	require.NoError(t, err)
	require.Len(t, requests, 3)
	require.ElementsMatch(t, []archaeodomain.RequestKind{
		archaeodomain.RequestPatternSurfacing,
		archaeodomain.RequestTensionAnalysis,
		archaeodomain.RequestProspectiveAnalysis,
	}, []archaeodomain.RequestKind{requests[0].Kind, requests[1].Kind, requests[2].Kind})
	for _, record := range requests {
		require.Equal(t, archaeodomain.RequestStatusPending, record.Status)
	}
}

func TestPrepareLivingPlanWithNoPlanStore(t *testing.T) {
	svc := archaeology.Service{}
	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{}, state, "")
	require.Nil(t, result.Plan)
	require.Nil(t, result.Step)
	require.Nil(t, result.Result)
	require.NoError(t, result.Err)
}

func TestPrepareLivingPlanWhenLoadActiveContextReturnsNil(t *testing.T) {
	store := &stubPlanStore{}
	var phases []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phases = append(phases, phase)
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{}, nil
		},
	}
	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), &core.Task{
		Context: map[string]any{"workspace": "/tmp/ws"},
	}, state, "wf-1")
	require.NoError(t, result.Err)
	require.Nil(t, result.Plan)
	require.Equal(t, []archaeodomain.EucloPhase{archaeodomain.PhaseArchaeology}, phases)
}

func TestPrepareLivingPlanWhenGateEvaluatorIsNil(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	var phases []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phases = append(phases, phase)
		},
		// EvaluateGate is nil
	}
	task := &core.Task{Context: map[string]any{"current_step_id": "step-1"}}
	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-1")
	require.Error(t, result.Err)
	require.Equal(t, []archaeodomain.EucloPhase{archaeodomain.PhaseBlocked}, phases)
}

func TestPrepareLivingPlanWhenGateReturnsShortCircuit(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	var phases []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phases = append(phases, phase)
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{
				Result: &core.Result{Success: true, Data: map[string]any{"skip": true}},
			}, nil
		},
	}
	task := &core.Task{Context: map[string]any{"current_step_id": "step-1"}}
	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-1")
	require.NoError(t, result.Err)
	require.NotNil(t, result.Result)
	require.True(t, result.Result.Success)
	// Should not be blocked
	require.NotEqual(t, archaeodomain.PhaseBlocked, phases)
}

func TestPrepareLivingPlanWhenGateReturnsConfidenceUpdated(t *testing.T) {
	now := time.Now().UTC()
	store := &stubPlanStore{
		plan: &frameworkplan.LivingPlan{
			ID:         "plan-1",
			WorkflowID: "wf-1",
			Steps: map[string]*frameworkplan.PlanStep{
				"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	var phases []archaeodomain.EucloPhase
	svc := archaeology.Service{
		Plans: archaeoplans.Service{Store: store},
		PersistPhase: func(_ context.Context, _ *core.Task, _ *core.Context, phase archaeodomain.EucloPhase, _ string, _ *frameworkplan.PlanStep) {
			phases = append(phases, phase)
		},
		EvaluateGate: func(context.Context, *core.Task, *core.Context, *frameworkplan.LivingPlan, *frameworkplan.PlanStep) (archaeoexec.PreflightOutcome, error) {
			return archaeoexec.PreflightOutcome{
				ConfidenceUpdated: true,
			}, nil
		},
	}
	task := &core.Task{Context: map[string]any{"current_step_id": "step-1"}}
	state := core.NewContext()
	result := svc.PrepareLivingPlan(context.Background(), task, state, "wf-1")
	require.NoError(t, result.Err)
	require.Nil(t, result.Result)
	require.NotNil(t, result.Step)
	// phase not blocked, but we didn't record any phase because no blocking
	// The current step should be set in state
	require.Equal(t, "step-1", state.GetString("euclo.current_plan_step_id"))
}

func newWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func openPatternStores(t *testing.T) (*patterns.SQLitePatternStore, *patterns.SQLiteCommentStore) {
	t.Helper()
	db, err := patterns.OpenSQLite(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	patternStore, err := patterns.NewSQLitePatternStore(db)
	require.NoError(t, err)
	commentStore, err := patterns.NewSQLiteCommentStore(db)
	require.NoError(t, err)
	return patternStore, commentStore
}

func openRetrievalDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	require.NoError(t, retrieval.EnsureSchema(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

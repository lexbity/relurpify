package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestSQLiteWorkflowStateStoreCreateAndListWorkflow(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	version, err := store.SchemaVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, workflowStateSchemaVersion, version)

	err = store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Implement change",
		Status:      WorkflowRunStatusPending,
		Metadata:    map[string]any{"source": "test"},
	})
	require.NoError(t, err)

	workflow, ok, err := store.GetWorkflow(ctx, "wf-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "task-1", workflow.TaskID)
	require.Equal(t, "test", workflow.Metadata["source"])
	require.EqualValues(t, 1, workflow.Version)

	workflows, err := store.ListWorkflows(ctx, 10)
	require.NoError(t, err)
	require.Len(t, workflows, 1)
	require.Equal(t, "wf-1", workflows[0].WorkflowID)
}

func TestSQLiteWorkflowStateStorePersistsPlanAndSelectsReadySteps(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-plan",
		TaskID:      "task-plan",
		TaskType:    core.TaskTypePlanning,
		Instruction: "Plan and execute",
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-plan",
		WorkflowID: "wf-plan",
		Status:     WorkflowRunStatusRunning,
	}))

	plan := core.Plan{
		Goal: "update docs",
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "read files"},
			{ID: "step-2", Description: "edit files"},
			{ID: "step-3", Description: "verify"},
		},
		Dependencies: map[string][]string{
			"step-2": {"step-1"},
			"step-3": {"step-2"},
		},
	}
	require.NoError(t, store.SavePlan(ctx, WorkflowPlanRecord{
		PlanID:     "plan-1",
		WorkflowID: "wf-plan",
		RunID:      "run-plan",
		Plan:       plan,
		IsActive:   true,
	}))

	activePlan, ok, err := store.GetActivePlan(ctx, "wf-plan")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "plan-1", activePlan.PlanID)

	ready, err := store.ListReadySteps(ctx, "wf-plan")
	require.NoError(t, err)
	require.Len(t, ready, 1)
	require.Equal(t, "step-1", ready[0].StepID)

	require.NoError(t, store.UpdateStepStatus(ctx, "wf-plan", "step-1", StepStatusCompleted, "read done"))

	ready, err = store.ListReadySteps(ctx, "wf-plan")
	require.NoError(t, err)
	require.Len(t, ready, 1)
	require.Equal(t, "step-2", ready[0].StepID)
}

func TestSQLiteWorkflowStateStoreStoresArtifactsAndKnowledgeInStepSlice(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-slice",
		TaskID:      "task-slice",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Carry step outputs forward",
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-slice",
		WorkflowID: "wf-slice",
		Status:     WorkflowRunStatusRunning,
	}))
	plan := core.Plan{
		Goal: "test projections",
		Steps: []core.PlanStep{
			{ID: "step-a", Description: "prepare"},
			{ID: "step-b", Description: "use previous result"},
		},
		Dependencies: map[string][]string{
			"step-b": {"step-a"},
		},
	}
	require.NoError(t, store.SavePlan(ctx, WorkflowPlanRecord{
		PlanID:     "plan-slice",
		WorkflowID: "wf-slice",
		RunID:      "run-slice",
		Plan:       plan,
		IsActive:   true,
	}))

	finishedAt := time.Now().UTC()
	require.NoError(t, store.CreateStepRun(ctx, StepRunRecord{
		StepRunID:      "step-run-a1",
		WorkflowID:     "wf-slice",
		RunID:          "run-slice",
		StepID:         "step-a",
		Attempt:        1,
		Status:         StepStatusCompleted,
		Summary:        "prepared context",
		ResultData:     map[string]any{"summary": "prepared context"},
		VerificationOK: true,
		StartedAt:      finishedAt.Add(-time.Minute),
		FinishedAt:     &finishedAt,
	}))
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-slice", "step-a", StepStatusCompleted, "prepared context"))
	require.NoError(t, store.UpsertArtifact(ctx, StepArtifactRecord{
		ArtifactID:        "artifact-a1",
		WorkflowID:        "wf-slice",
		StepRunID:         "step-run-a1",
		Kind:              "tool_output",
		ContentType:       "text/plain",
		StorageKind:       ArtifactStorageInline,
		SummaryText:       "summary for prompt",
		SummaryMetadata:   map[string]any{"source": "tool"},
		InlineRawText:     "raw output",
		RawSizeBytes:      int64(len("raw output")),
		CompressionMethod: "simple",
	}))
	require.NoError(t, store.PutKnowledge(ctx, KnowledgeRecord{
		RecordID:   "fact-a1",
		WorkflowID: "wf-slice",
		StepRunID:  "step-run-a1",
		StepID:     "step-a",
		Kind:       KnowledgeKindFact,
		Title:      "Prepared state",
		Content:    "The file scan completed successfully.",
	}))
	require.NoError(t, store.PutKnowledge(ctx, KnowledgeRecord{
		RecordID:   "issue-a1",
		WorkflowID: "wf-slice",
		StepRunID:  "step-run-a1",
		StepID:     "step-a",
		Kind:       KnowledgeKindIssue,
		Title:      "Open issue",
		Content:    "Need to preserve comments.",
		Status:     "open",
	}))
	require.NoError(t, store.PutKnowledge(ctx, KnowledgeRecord{
		RecordID:   "decision-a1",
		WorkflowID: "wf-slice",
		StepRunID:  "step-run-a1",
		StepID:     "step-a",
		Kind:       KnowledgeKindDecision,
		Title:      "Use small patch",
		Content:    "Edits should be narrow.",
	}))
	require.NoError(t, store.AppendEvent(ctx, WorkflowEventRecord{
		EventID:    "event-a1",
		WorkflowID: "wf-slice",
		RunID:      "run-slice",
		StepID:     "step-a",
		EventType:  "step_completed",
		Message:    "step a completed",
	}))

	slice, ok, err := store.LoadStepSlice(ctx, "wf-slice", "step-b", 10)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "step-b", slice.Step.StepID)
	require.Len(t, slice.DependencySteps, 1)
	require.Equal(t, "step-a", slice.DependencySteps[0].StepID)
	require.Len(t, slice.DependencyRuns, 1)
	require.Equal(t, "step-run-a1", slice.DependencyRuns[0].StepRunID)
	require.Len(t, slice.Artifacts, 1)
	require.Equal(t, "raw output", slice.Artifacts[0].InlineRawText)
	require.Len(t, slice.Facts, 1)
	require.Len(t, slice.Issues, 1)
	require.Len(t, slice.Decisions, 1)
	require.Len(t, slice.RecentEvents, 1)
}

func TestSQLiteWorkflowStateStoreStoresWorkflowArtifacts(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-workflow-artifact",
		TaskID:      "wf-workflow-artifact",
		TaskType:    core.TaskTypePlanning,
		Instruction: "persist workflow artifact",
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-workflow-artifact",
		WorkflowID: "wf-workflow-artifact",
		Status:     WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, WorkflowArtifactRecord{
		ArtifactID:        "workflow-artifact-1",
		WorkflowID:        "wf-workflow-artifact",
		RunID:             "run-workflow-artifact",
		Kind:              "planner_output",
		ContentType:       "application/json",
		StorageKind:       ArtifactStorageInline,
		SummaryText:       "planner summary",
		SummaryMetadata:   map[string]any{"phase": "planning"},
		InlineRawText:     `{"goal":"ship"}`,
		RawSizeBytes:      int64(len(`{"goal":"ship"}`)),
		CompressionMethod: "none",
	}))

	artifacts, err := store.ListWorkflowArtifacts(ctx, "wf-workflow-artifact", "run-workflow-artifact")
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	require.Equal(t, "planner_output", artifacts[0].Kind)
	require.Equal(t, `{"goal":"ship"}`, artifacts[0].InlineRawText)
	require.Equal(t, "planning", artifacts[0].SummaryMetadata["phase"])
}

func TestSQLiteWorkflowStateStoreStoresStageResultsAndFindsLatestValid(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-stage-results",
		TaskID:      "task-stage-results",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "persist stage attempts",
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-stage-results",
		WorkflowID: "wf-stage-results",
		Status:     WorkflowRunStatusRunning,
	}))

	started := time.Now().UTC().Add(-time.Minute)
	finished := time.Now().UTC()
	require.NoError(t, store.SaveStageResult(ctx, WorkflowStageResultRecord{
		ResultID:         "stage-result-1",
		WorkflowID:       "wf-stage-results",
		RunID:            "run-stage-results",
		StageName:        "analyze",
		StageIndex:       1,
		ContractName:     "issue-list",
		ContractVersion:  "v1",
		PromptText:       "analyze prompt",
		ResponseJSON:     `{"text":"bad"}`,
		DecodedOutput:    map[string]any{"issues": []any{}},
		ValidationOK:     false,
		ErrorText:        "missing issues",
		RetryAttempt:     0,
		TransitionKind:   "retry",
		TransitionReason: "missing issues",
		StartedAt:        started,
		FinishedAt:       finished,
	}))
	require.NoError(t, store.SaveStageResult(ctx, WorkflowStageResultRecord{
		ResultID:        "stage-result-2",
		WorkflowID:      "wf-stage-results",
		RunID:           "run-stage-results",
		StageName:       "analyze",
		StageIndex:      1,
		ContractName:    "issue-list",
		ContractVersion: "v1",
		PromptText:      "analyze prompt retry",
		ResponseJSON:    `{"text":"good"}`,
		DecodedOutput:   map[string]any{"issues": []any{map[string]any{"title": "bug"}}},
		ValidationOK:    true,
		RetryAttempt:    1,
		TransitionKind:  "next",
		StartedAt:       finished,
		FinishedAt:      finished.Add(time.Second),
	}))

	results, err := store.ListStageResults(ctx, "wf-stage-results", "run-stage-results")
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "stage-result-1", results[0].ResultID)
	require.Equal(t, "stage-result-2", results[1].ResultID)
	require.False(t, results[0].ValidationOK)
	require.True(t, results[1].ValidationOK)

	latest, ok, err := store.GetLatestValidStageResult(ctx, "wf-stage-results", "run-stage-results", "analyze")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "stage-result-2", latest.ResultID)
	require.Equal(t, 1, latest.RetryAttempt)
	decoded, ok := latest.DecodedOutput.(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, decoded["issues"])
}

func TestSQLiteWorkflowStateStoreListsStageResultsOrderedByStageAndRetry(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-stage-order",
		TaskID:      "task-stage-order",
		TaskType:    core.TaskTypePlanning,
		Instruction: "order stage results",
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-stage-order",
		WorkflowID: "wf-stage-order",
		Status:     WorkflowRunStatusRunning,
	}))

	now := time.Now().UTC()
	require.NoError(t, store.SaveStageResult(ctx, WorkflowStageResultRecord{
		ResultID:        "r2",
		WorkflowID:      "wf-stage-order",
		RunID:           "run-stage-order",
		StageName:       "analyze",
		StageIndex:      2,
		ContractName:    "issue-list",
		ContractVersion: "v1",
		RetryAttempt:    0,
		StartedAt:       now,
		FinishedAt:      now,
	}))
	require.NoError(t, store.SaveStageResult(ctx, WorkflowStageResultRecord{
		ResultID:        "r1b",
		WorkflowID:      "wf-stage-order",
		RunID:           "run-stage-order",
		StageName:       "explore",
		StageIndex:      1,
		ContractName:    "file-list",
		ContractVersion: "v1",
		RetryAttempt:    1,
		StartedAt:       now,
		FinishedAt:      now.Add(time.Second),
	}))
	require.NoError(t, store.SaveStageResult(ctx, WorkflowStageResultRecord{
		ResultID:        "r1a",
		WorkflowID:      "wf-stage-order",
		RunID:           "run-stage-order",
		StageName:       "explore",
		StageIndex:      1,
		ContractName:    "file-list",
		ContractVersion: "v1",
		RetryAttempt:    0,
		StartedAt:       now,
		FinishedAt:      now,
	}))

	results, err := store.ListStageResults(ctx, "wf-stage-order", "run-stage-order")
	require.NoError(t, err)
	require.Len(t, results, 3)
	require.Equal(t, []string{"r1a", "r1b", "r2"}, []string{results[0].ResultID, results[1].ResultID, results[2].ResultID})
}

func TestSQLiteWorkflowStateStoreInvalidatesDependents(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-invalidate",
		TaskID:      "task-invalidate",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "rerun upstream step",
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-invalidate",
		WorkflowID: "wf-invalidate",
		Status:     WorkflowRunStatusRunning,
	}))
	plan := core.Plan{
		Goal: "propagate invalidation",
		Steps: []core.PlanStep{
			{ID: "a", Description: "upstream"},
			{ID: "b", Description: "middle"},
			{ID: "c", Description: "downstream"},
		},
		Dependencies: map[string][]string{
			"b": {"a"},
			"c": {"b"},
		},
	}
	require.NoError(t, store.SavePlan(ctx, WorkflowPlanRecord{
		PlanID:     "plan-invalidate",
		WorkflowID: "wf-invalidate",
		RunID:      "run-invalidate",
		Plan:       plan,
		IsActive:   true,
	}))
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-invalidate", "a", StepStatusCompleted, "done a"))
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-invalidate", "b", StepStatusCompleted, "done b"))
	require.NoError(t, store.UpdateStepStatus(ctx, "wf-invalidate", "c", StepStatusCompleted, "done c"))

	invalidations, err := store.InvalidateDependents(ctx, "wf-invalidate", "a", "rerun upstream")
	require.NoError(t, err)
	require.Len(t, invalidations, 2)

	listed, err := store.ListInvalidations(ctx, "wf-invalidate")
	require.NoError(t, err)
	require.Len(t, listed, 2)

	steps, err := store.ListSteps(ctx, "wf-invalidate")
	require.NoError(t, err)
	statusByID := map[string]StepStatus{}
	for _, step := range steps {
		statusByID[step.StepID] = step.Status
	}
	require.Equal(t, StepStatusCompleted, statusByID["a"])
	require.Equal(t, StepStatusInvalidated, statusByID["b"])
	require.Equal(t, StepStatusInvalidated, statusByID["c"])
}

func TestSQLiteWorkflowStateStoreWorkflowVersionAndRunStatus(t *testing.T) {
	store := newTestWorkflowStateStore(t)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, WorkflowRecord{
		WorkflowID:  "wf-version",
		TaskID:      "task-version",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "versioned workflow",
		Status:      WorkflowRunStatusPending,
	}))
	require.NoError(t, store.CreateRun(ctx, WorkflowRunRecord{
		RunID:      "run-version",
		WorkflowID: "wf-version",
		Status:     WorkflowRunStatusRunning,
	}))

	workflow, ok, err := store.GetWorkflow(ctx, "wf-version")
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 1, workflow.Version)

	nextVersion, err := store.UpdateWorkflowStatus(ctx, "wf-version", workflow.Version, WorkflowRunStatusRunning, "step-1")
	require.NoError(t, err)
	require.EqualValues(t, 2, nextVersion)

	_, err = store.UpdateWorkflowStatus(ctx, "wf-version", workflow.Version, WorkflowRunStatusCompleted, "")
	require.ErrorIs(t, err, sql.ErrNoRows)

	finishedAt := time.Now().UTC()
	require.NoError(t, store.UpdateRunStatus(ctx, "run-version", WorkflowRunStatusCompleted, &finishedAt))
	run, ok, err := store.GetRun(ctx, "run-version")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, WorkflowRunStatusCompleted, run.Status)
	require.NotNil(t, run.FinishedAt)
}

func newTestWorkflowStateStore(t *testing.T) *SQLiteWorkflowStateStore {
	t.Helper()
	store, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow_state.db"))
	require.NoError(t, err)
	return store
}

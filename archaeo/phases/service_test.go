package phases_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/archaeo/phases"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestServiceEnsureAndLoadPersistsPhaseState(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "phase me",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	svc := phases.Service{
		Store: store,
		Now:   func() time.Time { return now },
	}

	state, err := svc.Ensure(ctx, "wf-1", archaeodomain.PhaseArchaeology)
	require.NoError(t, err)
	require.Equal(t, archaeodomain.PhaseArchaeology, state.CurrentPhase)

	loaded, ok, err := svc.Load(ctx, "wf-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, archaeodomain.PhaseArchaeology, loaded.CurrentPhase)
	require.Equal(t, now, loaded.EnteredAt)
}

func TestServiceTransitionValidatesAndAppendsEvent(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "phase me",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	now := time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC)
	svc := phases.Service{
		Store: store,
		Now:   func() time.Time { return now },
	}
	_, err := svc.Ensure(ctx, "wf-2", archaeodomain.PhaseArchaeology)
	require.NoError(t, err)

	version := 3
	state, err := svc.Transition(ctx, "wf-2", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{
		To:                archaeodomain.PhasePlanFormation,
		ActivePlanID:      "plan-1",
		ActivePlanVersion: &version,
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.PhasePlanFormation, state.CurrentPhase)
	require.Equal(t, "plan-1", state.ActivePlanID)
	require.NotNil(t, state.ActivePlanVersion)
	require.Equal(t, 3, *state.ActivePlanVersion)

	events, err := store.ListEvents(ctx, "wf-2", 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "archaeo.workflow_phase_transitioned", events[0].EventType)
}

func TestServiceRejectsInvalidTransition(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-3",
		TaskID:      "task-3",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "phase me",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := phases.Service{Store: store}
	_, err := svc.Ensure(ctx, "wf-3", archaeodomain.PhaseArchaeology)
	require.NoError(t, err)

	_, err = svc.Transition(ctx, "wf-3", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{
		To: archaeodomain.PhaseVerification,
	})
	require.Error(t, err)
}

func TestServiceRecordStateBuildsTransitionFromTaskAndState(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-4",
		TaskID:      "task-4",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "phase me",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := phases.Service{Store: store}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-4")
	state.Set("euclo.living_plan", &frameworkplan.LivingPlan{
		ID:      "plan-4",
		Version: 7,
	})
	broker := guidance.NewGuidanceBroker(time.Minute)
	_, err := broker.SubmitAsync(guidance.GuidanceRequest{
		Kind:  guidance.GuidanceConfidence,
		Title: "phase test",
		Choices: []guidance.GuidanceChoice{
			{ID: "proceed", Label: "Proceed", IsDefault: true},
		},
	})
	require.NoError(t, err)
	pending := broker.PendingRequests()
	require.Len(t, pending, 1)
	_, err = svc.Ensure(ctx, "wf-4", archaeodomain.PhaseArchaeology)
	require.NoError(t, err)
	_, err = svc.Transition(ctx, "wf-4", archaeodomain.PhaseArchaeology, archaeodomain.PhaseTransition{
		To: archaeodomain.PhasePlanFormation,
	})
	require.NoError(t, err)

	persisted, err := svc.RecordState(ctx, &core.Task{}, state, broker, archaeodomain.PhaseExecution, "", &frameworkplan.PlanStep{ID: "step-1"})
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, archaeodomain.PhaseExecution, persisted.CurrentPhase)
	require.Equal(t, "plan-4", persisted.ActivePlanID)
	require.NotNil(t, persisted.ActivePlanVersion)
	require.Equal(t, 7, *persisted.ActivePlanVersion)
	require.Equal(t, []string{pending[0].ID}, persisted.PendingGuidance)
}

func TestServiceSyncPendingLearningPersistsQueue(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-5",
		TaskID:      "task-5",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "phase me",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	svc := phases.Service{Store: store}
	state, err := svc.SyncPendingLearning(ctx, "wf-5", []string{"learn-2", "learn-1"})
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, archaeodomain.PhaseArchaeology, state.CurrentPhase)
	require.Equal(t, []string{"learn-1", "learn-2"}, state.PendingLearning)

	loaded, ok, err := svc.Load(ctx, "wf-5")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []string{"learn-1", "learn-2"}, loaded.PendingLearning)
}

func newWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

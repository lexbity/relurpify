package requests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestServiceRequestLifecycle(t *testing.T) {
	ctx := context.Background()
	store := openWorkflowStore(t, "wf-requests-lifecycle")
	now := time.Date(2026, 3, 27, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string { return prefix + "-1" },
	}

	version := 2
	record, err := svc.Create(ctx, CreateInput{
		WorkflowID:      "wf-requests-lifecycle",
		ExplorationID:   "explore-1",
		PlanID:          "plan-1",
		PlanVersion:     &version,
		Kind:            archaeodomain.RequestPlanReformation,
		Title:           "Reform active plan",
		Description:     "Recompute after drift.",
		RequestedBy:     "test",
		SubjectRefs:     []string{"tension-1"},
		Input:           map[string]any{"reason": "drift"},
		BasedOnRevision: "rev-1",
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusPending, record.Status)

	record, err = svc.Dispatch(ctx, record.WorkflowID, record.ID, map[string]any{"provider": "relurpic"})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusDispatched, record.Status)

	record, err = svc.Start(ctx, record.WorkflowID, record.ID, map[string]any{"dispatch_id": "disp-1"})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusRunning, record.Status)
	require.NotNil(t, record.StartedAt)

	record, err = svc.Complete(ctx, CompleteInput{
		WorkflowID: record.WorkflowID,
		RequestID:  record.ID,
		Result: archaeodomain.RequestResult{
			Kind:    "plan_version",
			RefID:   "plan-1:v3",
			Summary: "Created successor draft",
		},
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusCompleted, record.Status)
	require.NotNil(t, record.Result)
	require.Equal(t, "plan-1:v3", record.Result.RefID)
	require.NotNil(t, record.CompletedAt)

	loaded, ok, err := svc.Load(ctx, record.WorkflowID, record.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, archaeodomain.RequestStatusCompleted, loaded.Status)
	require.Equal(t, "plan-1:v3", loaded.Result.RefID)

	pending, err := svc.Pending(ctx, record.WorkflowID)
	require.NoError(t, err)
	require.Empty(t, pending)

	log := &archaeoevents.WorkflowLog{Store: store}
	events, err := log.Read(ctx, record.WorkflowID, 0, 0, false)
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, archaeoevents.EventRequestCreated, events[0].Type)
	require.Equal(t, archaeoevents.EventRequestDispatched, events[1].Type)
	require.Equal(t, archaeoevents.EventRequestStarted, events[2].Type)
	require.Equal(t, archaeoevents.EventRequestCompleted, events[3].Type)
}

func TestServiceFailAndPending(t *testing.T) {
	ctx := context.Background()
	store := openWorkflowStore(t, "wf-requests-failed")
	now := time.Date(2026, 3, 27, 19, 0, 0, 0, time.UTC)
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string { return prefix + "-1" },
	}

	record, err := svc.Create(ctx, CreateInput{
		WorkflowID: "wf-requests-failed",
		Kind:       archaeodomain.RequestTensionAnalysis,
		Title:      "Analyze tensions",
	})
	require.NoError(t, err)

	pending, err := svc.Pending(ctx, record.WorkflowID)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	record, err = svc.Fail(ctx, record.WorkflowID, record.ID, "provider unavailable", true)
	require.NoError(t, err)
	require.Equal(t, archaeodomain.RequestStatusFailed, record.Status)
	require.Equal(t, 1, record.RetryCount)
	require.Equal(t, "provider unavailable", record.ErrorText)

	pending, err = svc.Pending(ctx, record.WorkflowID)
	require.NoError(t, err)
	require.Empty(t, pending)
}

func openWorkflowStore(t *testing.T, workflowID string) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "request workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	return store
}

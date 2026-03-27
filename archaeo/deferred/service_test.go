package deferred

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateAllowsMultipleAmbiguities(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-deferred")
	now := time.Date(2026, 3, 27, 21, 0, 0, 0, time.UTC)
	nextID := 0
	svc := Service{
		Store: store,
		Now:   func() time.Time { return now },
		NewID: func(prefix string) string {
			nextID++
			return prefix + "-" + strconv.Itoa(nextID)
		},
	}

	first, err := svc.CreateOrUpdate(ctx, CreateInput{
		WorkspaceID:  "/workspace/deferred",
		WorkflowID:   "wf-deferred",
		PlanID:       "plan-1",
		RequestID:    "request-1",
		PlanVersion:  intPtr(1),
		AmbiguityKey: "step-1:type-choice",
		Title:        "Need type choice",
	})
	require.NoError(t, err)
	second, err := svc.CreateOrUpdate(ctx, CreateInput{
		WorkspaceID:  "/workspace/deferred",
		WorkflowID:   "wf-deferred",
		PlanID:       "plan-1",
		RequestID:    "request-2",
		PlanVersion:  intPtr(1),
		AmbiguityKey: "step-2:api-choice",
		Title:        "Need API choice",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)

	records, err := svc.ListByWorkspace(ctx, "/workspace/deferred")
	require.NoError(t, err)
	require.Len(t, records, 2)

	updated, err := svc.CreateOrUpdate(ctx, CreateInput{
		WorkspaceID:        "/workspace/deferred",
		WorkflowID:         "wf-deferred",
		AmbiguityKey:       "step-1:type-choice",
		LinkedDraftVersion: intPtr(2),
		LinkedDraftPlanID:  "plan-1-v2",
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, updated.ID)
	require.Equal(t, archaeodomain.DeferredDraftFormed, updated.Status)
}

func newWorkflowStore(t *testing.T, workflowID string) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "deferred workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	return store
}

func intPtr(v int) *int { return &v }

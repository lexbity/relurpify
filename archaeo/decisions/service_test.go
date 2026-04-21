package decisions

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestDecisionLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-decision")
	now := time.Date(2026, 3, 27, 23, 0, 0, 0, time.UTC)
	svc := Service{Store: store, Now: func() time.Time { return now }}

	record, err := svc.Create(ctx, CreateInput{
		WorkspaceID:      "/workspace/decision",
		WorkflowID:       "wf-decision",
		Kind:             archaeodomain.DecisionKindStaleResult,
		RelatedRequestID: "request-1",
		Title:            "Need stale-result decision",
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.DecisionStatusOpen, record.Status)

	record, err = svc.Resolve(ctx, ResolveInput{
		WorkflowID:  "wf-decision",
		RecordID:    record.ID,
		Status:      archaeodomain.DecisionStatusResolved,
		CommentRefs: []string{"comment-1"},
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.DecisionStatusResolved, record.Status)
	require.NotNil(t, record.ResolvedAt)
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
		Instruction: "decision workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	return store
}

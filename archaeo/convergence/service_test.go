package convergence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceConvergenceHistoryAndCurrent(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-convergence")
	now := time.Date(2026, 3, 27, 22, 0, 0, 0, time.UTC)
	svc := Service{Store: store, Now: func() time.Time { return now }}

	record, err := svc.Create(ctx, CreateInput{
		WorkspaceID:        "/workspace/conv",
		WorkflowID:         "wf-convergence",
		Title:              "Resolve architecture ambiguity",
		Question:           "Is the current draft plan stable enough to act on?",
		DeferredDraftIDs:   []string{"deferred-1"},
		RelevantTensionIDs: []string{"tension-1"},
	})
	require.NoError(t, err)
	require.Equal(t, archaeodomain.ConvergenceResolutionOpen, record.Status)

	_, err = svc.Resolve(ctx, ResolveInput{
		WorkflowID: "wf-convergence",
		RecordID:   record.ID,
		Resolution: archaeodomain.ConvergenceResolution{
			Status:       archaeodomain.ConvergenceResolutionDeferred,
			ChosenOption: "defer",
			Summary:      "Need more implementation evidence",
		},
	})
	require.NoError(t, err)

	proj, err := svc.CurrentByWorkspace(ctx, "/workspace/conv")
	require.NoError(t, err)
	require.NotNil(t, proj)
	require.Len(t, proj.History, 1)
	require.NotNil(t, proj.Current)
	require.Equal(t, archaeodomain.ConvergenceResolutionDeferred, proj.Current.Status)
	require.Equal(t, 1, proj.DeferredCount)
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
		Instruction: "convergence workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}))
	return store
}

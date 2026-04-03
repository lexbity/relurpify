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

func TestCreateMissingRequiredFields(t *testing.T) {
	ctx := context.Background()
	svc := Service{Store: nil}
	record, err := svc.Create(ctx, CreateInput{})
	require.NoError(t, err)
	require.Nil(t, record)

	store := newWorkflowStore(t, "wf-empty")
	svcWithStore := Service{Store: store}
	record2, err := svcWithStore.Create(ctx, CreateInput{
		WorkspaceID: "ws",
		// WorkflowID missing
	})
	require.NoError(t, err)
	require.Nil(t, record2)

	record3, err := svcWithStore.Create(ctx, CreateInput{
		WorkflowID: "wf",
		// WorkspaceID missing
	})
	require.NoError(t, err)
	require.Nil(t, record3)
}

func TestLoadNotFound(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-load")
	svc := Service{Store: store}

	record, err := svc.Load(ctx, "wf-load", "non-existent-id")
	require.NoError(t, err)
	require.Nil(t, record)

	// nil store
	svcNil := Service{Store: nil}
	record2, err := svcNil.Load(ctx, "", "")
	require.NoError(t, err)
	require.Nil(t, record2)
}

func TestListByWorkspaceEmpty(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-list")
	svc := Service{Store: store}

	list, err := svc.ListByWorkspace(ctx, "non-existent-workspace")
	require.NoError(t, err)
	require.Empty(t, list)

	// nil store
	svcNil := Service{Store: nil}
	list2, err := svcNil.ListByWorkspace(ctx, "")
	require.NoError(t, err)
	require.Nil(t, list2)
}

func TestIDsByWorkspace(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-ids")
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	svc := Service{Store: store, Now: func() time.Time { return now }}

	rec1, err := svc.Create(ctx, CreateInput{
		WorkspaceID: "/workspace/ids",
		WorkflowID:  "wf-ids",
		Title:       "First convergence",
	})
	require.NoError(t, err)
	require.NotNil(t, rec1)

	// advance time to ensure distinct IDs
	now = now.Add(time.Second)
	rec2, err := svc.Create(ctx, CreateInput{
		WorkspaceID: "/workspace/ids",
		WorkflowID:  "wf-ids",
		Title:       "Second convergence",
	})
	require.NoError(t, err)
	require.NotNil(t, rec2)

	ids, err := svc.IDsByWorkspace(ctx, "/workspace/ids")
	require.NoError(t, err)
	require.Len(t, ids, 2)
	require.ElementsMatch(t, []string{rec1.ID, rec2.ID}, ids)

	// empty workspace
	idsEmpty, err := svc.IDsByWorkspace(ctx, "")
	require.NoError(t, err)
	require.Nil(t, idsEmpty)

	// nil store
	svcNil := Service{Store: nil}
	idsNil, err := svcNil.IDsByWorkspace(ctx, "/workspace/ids")
	require.NoError(t, err)
	require.Nil(t, idsNil)
}

func TestRebuildCurrent(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-rebuild")
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	svc := Service{Store: store, Now: func() time.Time { return now }}

	rec1, err := svc.Create(ctx, CreateInput{
		WorkspaceID: "/workspace/rebuild",
		WorkflowID:  "wf-rebuild",
		Title:       "Open convergence",
	})
	require.NoError(t, err)

	proj, err := svc.RebuildCurrent(ctx, "/workspace/rebuild")
	require.NoError(t, err)
	require.NotNil(t, proj)
	require.Equal(t, "/workspace/rebuild", proj.WorkspaceID)
	require.Len(t, proj.History, 1)
	require.NotNil(t, proj.Current)
	require.Equal(t, rec1.ID, proj.Current.ID)
	require.Equal(t, archaeodomain.ConvergenceResolutionOpen, proj.Current.Status)
	require.Equal(t, 1, proj.OpenCount)

	// resolve to deferred
	_, err = svc.Resolve(ctx, ResolveInput{
		WorkflowID: "wf-rebuild",
		RecordID:   rec1.ID,
		Resolution: archaeodomain.ConvergenceResolution{
			Status:       archaeodomain.ConvergenceResolutionDeferred,
			ChosenOption: "wait",
			Summary:      "Wait",
		},
	})
	require.NoError(t, err)

	proj2, err := svc.RebuildCurrent(ctx, "/workspace/rebuild")
	require.NoError(t, err)
	require.Equal(t, 1, proj2.DeferredCount)
	require.Equal(t, 0, proj2.OpenCount)
	require.Equal(t, archaeodomain.ConvergenceResolutionDeferred, proj2.Current.Status)

	// empty workspace
	projEmpty, err := svc.RebuildCurrent(ctx, "")
	require.NoError(t, err)
	require.Nil(t, projEmpty)

	// nil store
	svcNil := Service{Store: nil}
	projNil, err := svcNil.RebuildCurrent(ctx, "/workspace/rebuild")
	require.NoError(t, err)
	require.Nil(t, projNil)
}

func TestResolveMergesFields(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-merge")
	now := time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC)
	svc := Service{Store: store, Now: func() time.Time { return now }}

	rec, err := svc.Create(ctx, CreateInput{
		WorkspaceID:  "/workspace/merge",
		WorkflowID:   "wf-merge",
		Title:        "Test merge",
		CommentRefs:  []string{"comment-1"},
		Metadata:     map[string]any{"original": true},
	})
	require.NoError(t, err)

	resolvedRec, err := svc.Resolve(ctx, ResolveInput{
		WorkflowID: "wf-merge",
		RecordID:   rec.ID,
		Resolution: archaeodomain.ConvergenceResolution{
			Status:      archaeodomain.ConvergenceResolutionResolved,
			CommentRefs: []string{"comment-2"},
			Metadata:    map[string]any{"resolved": true},
			ChosenOption: "yes",
			Summary:     "Resolved",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resolvedRec)
	require.Equal(t, archaeodomain.ConvergenceResolutionResolved, resolvedRec.Status)
	require.Contains(t, resolvedRec.CommentRefs, "comment-1")
	require.Contains(t, resolvedRec.CommentRefs, "comment-2")
	require.Equal(t, true, resolvedRec.Metadata["original"])
	require.NotNil(t, resolvedRec.Resolution)
	require.Equal(t, true, resolvedRec.Resolution.Metadata["resolved"])
	require.Equal(t, true, resolvedRec.Resolution.Metadata["original"])
}

func TestMultipleConvergenceHistoryOrder(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-order")
	now := time.Date(2026, 4, 1, 16, 0, 0, 0, time.UTC)
	svc := Service{Store: store, Now: func() time.Time { return now }}

	// create first
	rec1, err := svc.Create(ctx, CreateInput{
		WorkspaceID: "/workspace/order",
		WorkflowID:  "wf-order",
		Title:       "First",
	})
	require.NoError(t, err)
	// advance time slightly
	now2 := now.Add(time.Second)
	svc.Now = func() time.Time { return now2 }
	rec2, err := svc.Create(ctx, CreateInput{
		WorkspaceID: "/workspace/order",
		WorkflowID:  "wf-order",
		Title:       "Second",
	})
	require.NoError(t, err)

	// list should be chronological
	list, err := svc.ListByWorkspace(ctx, "/workspace/order")
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, rec1.ID, list[0].ID)
	require.Equal(t, rec2.ID, list[1].ID)
	require.True(t, list[0].CreatedAt.Before(list[1].CreatedAt))

	// current should be the last (most recent)
	proj, err := svc.CurrentByWorkspace(ctx, "/workspace/order")
	require.NoError(t, err)
	require.NotNil(t, proj.Current)
	require.Equal(t, rec2.ID, proj.Current.ID)
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

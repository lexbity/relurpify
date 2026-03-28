package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestListWorkflowArtifactsByKindAndWorkspace(t *testing.T) {
	ctx := context.Background()
	store, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	now := time.Date(2026, 3, 27, 23, 30, 0, 0, time.UTC)
	for _, workflowID := range []string{"wf-a", "wf-b"} {
		require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
			WorkflowID:  workflowID,
			TaskID:      "task-" + workflowID,
			TaskType:    core.TaskTypeCodeGeneration,
			Instruction: workflowID,
			Status:      memory.WorkflowRunStatusRunning,
			CreatedAt:   now,
			UpdatedAt:   now,
		}))
	}

	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "artifact-1",
		WorkflowID:      "wf-a",
		Kind:            "bench-kind",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "artifact 1",
		SummaryMetadata: map[string]any{"workspace_id": "/workspace/a"},
		InlineRawText:   "{}",
		CreatedAt:       now,
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "artifact-2",
		WorkflowID:      "wf-a",
		Kind:            "bench-kind",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "artifact 2",
		SummaryMetadata: map[string]any{"workspace_id": "/workspace/b"},
		InlineRawText:   "{}",
		CreatedAt:       now.Add(time.Second),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "artifact-3",
		WorkflowID:      "wf-b",
		Kind:            "bench-kind",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "artifact 3",
		SummaryMetadata: map[string]any{"workspace_id": "/workspace/a"},
		InlineRawText:   "{}",
		CreatedAt:       now.Add(2 * time.Second),
	}))

	records, err := store.ListWorkflowArtifactsByKindAndWorkspace(ctx, "wf-a", "", "bench-kind", "/workspace/a")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "artifact-1", records[0].ArtifactID)

	records, err = store.ListWorkflowArtifactsByKindAndWorkspace(ctx, "", "", "bench-kind", "/workspace/a")
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, "artifact-1", records[0].ArtifactID)
	require.Equal(t, "artifact-3", records[1].ArtifactID)

	latest, ok, err := store.LatestWorkflowArtifactByKindAndWorkspace(ctx, "", "", "bench-kind", "/workspace/a")
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, latest)
	require.Equal(t, "artifact-3", latest.ArtifactID)
}

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

func TestUpdateWorkflowMetadata_MergesNewFields(t *testing.T) {
	ctx := context.Background()
	store, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	now := time.Now().UTC()
	// Create workflow with initial metadata
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "test",
		Status:      memory.WorkflowRunStatusRunning,
		Metadata:    map[string]any{"existing_key": "existing_value", "version": 1},
		CreatedAt:   now,
		UpdatedAt:   now,
	}))

	// Update with new fields
	err = store.UpdateWorkflowMetadata(ctx, "wf-1", map[string]any{"workspace": "/test/workspace", "mode": "code"})
	require.NoError(t, err)

	// Verify metadata merged correctly
	wf, ok, err := store.GetWorkflow(ctx, "wf-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "existing_value", wf.Metadata["existing_key"], "existing key should be preserved")
	require.Equal(t, float64(1), wf.Metadata["version"], "existing version should be preserved")
	require.Equal(t, "/test/workspace", wf.Metadata["workspace"], "new workspace should be added")
	require.Equal(t, "code", wf.Metadata["mode"], "new mode should be added")
}

func TestUpdateWorkflowMetadata_OverwritesExistingFields(t *testing.T) {
	ctx := context.Background()
	store, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	now := time.Now().UTC()
	// Create workflow with initial metadata including workspace
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "test",
		Status:      memory.WorkflowRunStatusRunning,
		Metadata:    map[string]any{"workspace": "/old/workspace", "mode": "chat"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}))

	// Update workspace to new value
	err = store.UpdateWorkflowMetadata(ctx, "wf-2", map[string]any{"workspace": "/new/workspace"})
	require.NoError(t, err)

	// Verify workspace was overwritten, mode preserved
	wf, ok, err := store.GetWorkflow(ctx, "wf-2")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "/new/workspace", wf.Metadata["workspace"], "workspace should be overwritten")
	require.Equal(t, "chat", wf.Metadata["mode"], "mode should be preserved")
}

func TestUpdateWorkflowMetadata_NoopOnUnknownWorkflow(t *testing.T) {
	ctx := context.Background()
	store, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	// Update metadata on non-existent workflow should not error (idempotent)
	err = store.UpdateWorkflowMetadata(ctx, "non-existent-wf", map[string]any{"workspace": "/test"})
	require.NoError(t, err, "should not error on unknown workflow")
}

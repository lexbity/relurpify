package storeutil

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

type delegatedWorkflowStore struct {
	*memorydb.SQLiteWorkflowStateStore
	artifactByID             func(context.Context, string) (*memory.WorkflowArtifactRecord, bool, error)
	listByKind               func(context.Context, string, string, string) ([]memory.WorkflowArtifactRecord, error)
	listByKindAndWorkspace   func(context.Context, string, string, string, string) ([]memory.WorkflowArtifactRecord, error)
	latestByKind             func(context.Context, string, string, string) (*memory.WorkflowArtifactRecord, bool, error)
	latestByKindAndWorkspace func(context.Context, string, string, string, string) (*memory.WorkflowArtifactRecord, bool, error)
}

func (s delegatedWorkflowStore) WorkflowArtifactByID(ctx context.Context, artifactID string) (*memory.WorkflowArtifactRecord, bool, error) {
	return s.artifactByID(ctx, artifactID)
}

func (s delegatedWorkflowStore) ListWorkflowArtifactsByKind(ctx context.Context, workflowID, runID, kind string) ([]memory.WorkflowArtifactRecord, error) {
	return s.listByKind(ctx, workflowID, runID, kind)
}

func (s delegatedWorkflowStore) ListWorkflowArtifactsByKindAndWorkspace(ctx context.Context, workflowID, runID, kind, workspaceID string) ([]memory.WorkflowArtifactRecord, error) {
	return s.listByKindAndWorkspace(ctx, workflowID, runID, kind, workspaceID)
}

func (s delegatedWorkflowStore) LatestWorkflowArtifactByKind(ctx context.Context, workflowID, runID, kind string) (*memory.WorkflowArtifactRecord, bool, error) {
	return s.latestByKind(ctx, workflowID, runID, kind)
}

func (s delegatedWorkflowStore) LatestWorkflowArtifactByKindAndWorkspace(ctx context.Context, workflowID, runID, kind, workspaceID string) (*memory.WorkflowArtifactRecord, bool, error) {
	return s.latestByKindAndWorkspace(ctx, workflowID, runID, kind, workspaceID)
}

type fallbackWorkflowStore struct {
	memory.WorkflowStateStore
	workflows []memory.WorkflowRecord
	artifacts []memory.WorkflowArtifactRecord
}

func (s fallbackWorkflowStore) ListWorkflows(context.Context, int) ([]memory.WorkflowRecord, error) {
	return append([]memory.WorkflowRecord(nil), s.workflows...), nil
}

func (s fallbackWorkflowStore) ListWorkflowArtifacts(_ context.Context, workflowID, _ string) ([]memory.WorkflowArtifactRecord, error) {
	out := make([]memory.WorkflowArtifactRecord, 0, len(s.artifacts))
	for _, artifact := range s.artifacts {
		if artifact.WorkflowID == workflowID {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func TestWorkflowArtifactHelpersValidateInputs(t *testing.T) {
	ctx := context.Background()

	artifact, ok, err := WorkflowArtifactByID(ctx, nil, "artifact-1")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, artifact)

	records, err := ListWorkflowArtifactsByKind(ctx, nil, "wf-1", "", "kind-a")
	require.NoError(t, err)
	require.Nil(t, records)

	records, err = ListWorkflowArtifactsByKindAndWorkspace(ctx, nil, "wf-1", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.Nil(t, records)

	artifact, ok, err = LatestWorkflowArtifactByKind(ctx, nil, "wf-1", "", "kind-a")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, artifact)

	artifact, ok, err = LatestWorkflowArtifactByKindAndWorkspace(ctx, nil, "wf-1", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, artifact)
}

func TestWorkflowArtifactHelpersDelegateToSpecializedStoreMethods(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	custom := delegatedWorkflowStore{
		SQLiteWorkflowStateStore: store,
		artifactByID: func(context.Context, string) (*memory.WorkflowArtifactRecord, bool, error) {
			return &memory.WorkflowArtifactRecord{ArtifactID: "delegated-id"}, true, nil
		},
		listByKind: func(context.Context, string, string, string) ([]memory.WorkflowArtifactRecord, error) {
			return []memory.WorkflowArtifactRecord{{ArtifactID: "list-a"}, {ArtifactID: "list-b"}}, nil
		},
		listByKindAndWorkspace: func(context.Context, string, string, string, string) ([]memory.WorkflowArtifactRecord, error) {
			return []memory.WorkflowArtifactRecord{{ArtifactID: "workspace-a"}}, nil
		},
		latestByKind: func(context.Context, string, string, string) (*memory.WorkflowArtifactRecord, bool, error) {
			return &memory.WorkflowArtifactRecord{ArtifactID: "latest-kind"}, true, nil
		},
		latestByKindAndWorkspace: func(context.Context, string, string, string, string) (*memory.WorkflowArtifactRecord, bool, error) {
			return &memory.WorkflowArtifactRecord{ArtifactID: "latest-workspace"}, true, nil
		},
	}

	artifact, ok, err := WorkflowArtifactByID(ctx, custom, "artifact-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "delegated-id", artifact.ArtifactID)

	records, err := ListWorkflowArtifactsByKind(ctx, custom, "wf-1", "", "kind-a")
	require.NoError(t, err)
	require.Len(t, records, 2)

	records, err = ListWorkflowArtifactsByKindAndWorkspace(ctx, custom, "wf-1", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "workspace-a", records[0].ArtifactID)

	artifact, ok, err = LatestWorkflowArtifactByKind(ctx, custom, "wf-1", "", "kind-a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "latest-kind", artifact.ArtifactID)

	artifact, ok, err = LatestWorkflowArtifactByKindAndWorkspace(ctx, custom, "wf-1", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "latest-workspace", artifact.ArtifactID)
}

func TestWorkflowArtifactHelpersFallbackAndFiltering(t *testing.T) {
	ctx := context.Background()

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	store := fallbackWorkflowStore{
		workflows: []memory.WorkflowRecord{
			{
				WorkflowID:  "wf-1",
				TaskID:      "task-1",
				TaskType:    core.TaskTypeAnalysis,
				Instruction: "first",
				Status:      memory.WorkflowRunStatusRunning,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				WorkflowID:  "wf-2",
				TaskID:      "task-2",
				TaskType:    core.TaskTypeAnalysis,
				Instruction: "second",
				Status:      memory.WorkflowRunStatusRunning,
				CreatedAt:   now.Add(time.Minute),
				UpdatedAt:   now.Add(time.Minute),
			},
		},
		artifacts: []memory.WorkflowArtifactRecord{
			{
				ArtifactID:      "artifact-1",
				WorkflowID:      "wf-1",
				Kind:            "kind-a",
				StorageKind:     memory.ArtifactStorageInline,
				SummaryMetadata: map[string]any{"workspace_id": "ws-1"},
				InlineRawText:   `{"artifact_id":"artifact-1"}`,
				CreatedAt:       now,
			},
			{
				ArtifactID:      "artifact-2",
				WorkflowID:      "wf-1",
				Kind:            "kind-b",
				StorageKind:     memory.ArtifactStorageInline,
				SummaryMetadata: map[string]any{"workspace_id": "ws-1"},
				InlineRawText:   `{"artifact_id":"artifact-2"}`,
				CreatedAt:       now.Add(time.Minute),
			},
			{
				ArtifactID:      "artifact-3",
				WorkflowID:      "wf-2",
				Kind:            "kind-a",
				StorageKind:     memory.ArtifactStorageInline,
				SummaryMetadata: map[string]any{"workspace_id": "ws-2"},
				InlineRawText:   `{"artifact_id":"artifact-3"}`,
				CreatedAt:       now.Add(2 * time.Minute),
			},
			{
				ArtifactID:      "artifact-4",
				WorkflowID:      "wf-2",
				Kind:            "kind-a",
				StorageKind:     memory.ArtifactStorageInline,
				SummaryMetadata: map[string]any{"workspace_id": "ws-1"},
				InlineRawText:   `{"artifact_id":"artifact-4"}`,
				CreatedAt:       now.Add(3 * time.Minute),
			},
		},
	}
	_, ok, err := WorkflowArtifactByID(ctx, store, "artifact-1")
	require.NoError(t, err)
	require.False(t, ok)

	kindRecords, err := ListWorkflowArtifactsByKind(ctx, store, "wf-1", "", "kind-a")
	require.NoError(t, err)
	require.Empty(t, kindRecords)

	workspaceRecords, err := ListWorkflowArtifactsByKindAndWorkspace(ctx, store, "", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.Empty(t, workspaceRecords)

	workspaceRecords, err = ListWorkflowArtifactsByKindAndWorkspace(ctx, store, "wf-1", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.Empty(t, workspaceRecords)

	latestKind, ok, err := LatestWorkflowArtifactByKind(ctx, store, "wf-1", "", "kind-a")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, latestKind)

	latestWorkspace, ok, err := LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, latestWorkspace)

	latestWorkspace, ok, err = LatestWorkflowArtifactByKindAndWorkspace(ctx, store, "wf-1", "", "kind-a", "ws-1")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, latestWorkspace)
}

func TestWorkflowArtifactWorkspaceHelpers(t *testing.T) {
	artifacts := []memory.WorkflowArtifactRecord{
		{ArtifactID: "artifact-1", SummaryMetadata: map[string]any{"workspace_id": "ws-1"}},
		{ArtifactID: "artifact-2", SummaryMetadata: map[string]any{"workspace_id": "  ws-1  "}},
		{ArtifactID: "artifact-3", SummaryMetadata: map[string]any{"workspace_id": 1}},
		{ArtifactID: "artifact-4"},
	}

	require.Equal(t, "ws-1", artifactWorkspaceID(artifacts[0]))
	require.Equal(t, "ws-1", artifactWorkspaceID(artifacts[1]))
	require.Equal(t, "", artifactWorkspaceID(artifacts[2]))
	require.Equal(t, "", artifactWorkspaceID(artifacts[3]))

	filtered := filterWorkflowArtifactsByWorkspace(artifacts, "ws-1")
	require.Len(t, filtered, 2)
	require.Equal(t, []string{"artifact-1", "artifact-2"}, []string{filtered[0].ArtifactID, filtered[1].ArtifactID})

	require.Nil(t, filterWorkflowArtifactsByWorkspace(nil, "ws-1"))
	require.Nil(t, filterWorkflowArtifactsByWorkspace(artifacts, "  "))
	require.Nil(t, filterWorkflowArtifactsByWorkspace([]memory.WorkflowArtifactRecord{}, "ws-1"))
}

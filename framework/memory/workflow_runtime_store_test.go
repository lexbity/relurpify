package memory_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestCompositeRuntimeStoreQueriesWorkflowCheckpointAndMemoryRecords(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()
	checkpoints := db.NewSQLiteCheckpointStoreWithEvents(workflowStore.DB(), workflowStore, "wf-1", "run-1")
	store := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, checkpoints)

	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		Instruction: "ship phase 7",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  now,
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "artifact-1",
		WorkflowID:    "wf-1",
		RunID:         "run-1",
		Kind:          "summary",
		ContentType:   "text/plain",
		StorageKind:   memory.ArtifactStorageInline,
		SummaryText:   "workflow summary",
		InlineRawText: "phase 7 summary",
		CreatedAt:     now,
	}))
	require.NoError(t, store.AppendEvent(ctx, memory.WorkflowEventRecord{
		EventID:    "event-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		EventType:  "graph.preflight",
		Message:    "preflight passed",
		CreatedAt:  now,
	}))
	require.NoError(t, store.PutDeclarative(ctx, memory.DeclarativeMemoryRecord{
		RecordID:   "decl-1",
		Scope:      memory.MemoryScopeProject,
		Kind:       memory.DeclarativeMemoryKindDecision,
		Summary:    "prefer unified runtime storage",
		WorkflowID: "wf-1",
		TaskID:     "task-1",
		CreatedAt:  now,
	}))
	require.NoError(t, store.Save(&graph.GraphCheckpoint{
		CheckpointID:    "cp-1",
		TaskID:          "task-1",
		CompletedNodeID: "persist",
		NextNodeID:      "done",
		GraphHash:       "hash",
		CreatedAt:       now,
		Context:         core.NewContext(),
	}))

	payload, err := store.QueryWorkflowRuntime(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Equal(t, "wf-1", payload["workflow_id"])
	require.NotNil(t, payload["workflow"])
	require.NotNil(t, payload["run"])
	require.Len(t, payload["workflow_artifacts"].([]memory.WorkflowArtifactRecord), 1)
	require.Len(t, payload["events"].([]memory.WorkflowEventRecord), 2)
	require.Len(t, payload["declarative_memory"].([]memory.DeclarativeMemoryRecord), 1)

	checkpoint, err := store.Load("task-1", "cp-1")
	require.NoError(t, err)
	require.Equal(t, "done", checkpoint.NextNodeID)
}

func TestCompositeRuntimeStoreDelegatesGenericMemoryMethods(t *testing.T) {
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()

	store := memory.NewCompositeRuntimeStore(nil, runtimeStore, nil)
	ctx := context.Background()
	require.NoError(t, store.Remember(ctx, "fact-1", map[string]interface{}{"summary": "remembered"}, memory.MemoryScopeProject))

	record, ok, err := store.Recall(ctx, "fact-1", memory.MemoryScopeProject)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "remembered", record.Value["summary"])

	results, err := store.Search(ctx, "remembered", memory.MemoryScopeProject)
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.NoError(t, store.Forget(ctx, "fact-1", memory.MemoryScopeProject))
	_, ok, err = store.Recall(ctx, "fact-1", memory.MemoryScopeProject)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCompositeRuntimeStoreHandlesNilSubstores(t *testing.T) {
	store := memory.NewCompositeRuntimeStore(nil, nil, nil)
	payload, err := store.QueryWorkflowRuntime(context.Background(), "wf-missing", "run-missing")
	require.NoError(t, err)
	require.Equal(t, "wf-missing", payload["workflow_id"])
	require.Equal(t, "run-missing", payload["run_id"])

	require.NoError(t, store.Remember(context.Background(), "ignored", map[string]interface{}{"summary": "x"}, memory.MemoryScopeProject))
}

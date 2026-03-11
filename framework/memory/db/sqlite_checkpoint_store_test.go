package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestSQLiteCheckpointStoreRoundTripAndEvents(t *testing.T) {
	ctx := context.Background()
	workflowStore, err := NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	now := time.Date(2026, 3, 11, 15, 0, 0, 0, time.UTC)
	require.NoError(t, workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypePlanning,
		Instruction: "persist checkpoints",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, workflowStore.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  now,
	}))

	store := NewSQLiteCheckpointStoreWithEvents(workflowStore.db, workflowStore, "wf-1", "run-1")
	graphCtx := core.NewContext()
	graphCtx.Set("checkpoint.state", "warm")
	checkpoint := &graph.GraphCheckpoint{
		CheckpointID:    "cp-sqlite",
		TaskID:          "task-1",
		CompletedNodeID: "verify",
		NextNodeID:      "done",
		GraphHash:       "hash-1",
		VisitCounts:     map[string]int{"verify": 1},
		ExecutionPath:   []string{"plan", "verify"},
		LastTransition: &graph.NodeTransitionRecord{
			CompletedNodeID:  "verify",
			NextNodeID:       "done",
			TransitionReason: "test",
			CompletedAt:      now,
		},
		LastResultSummary: &graph.CheckpointResultSummary{
			NodeID:  "verify",
			Success: true,
		},
		Context:   graphCtx,
		Metadata:  map[string]any{"phase": "seven"},
		CreatedAt: now,
	}

	require.NoError(t, store.Save(checkpoint))

	loaded, err := store.Load("task-1", "cp-sqlite")
	require.NoError(t, err)
	require.Equal(t, "done", loaded.NextNodeID)
	require.Equal(t, 1, loaded.VisitCounts["verify"])
	require.Equal(t, "warm", loaded.Context.GetString("checkpoint.state"))
	require.Equal(t, "seven", loaded.Metadata["phase"])

	ids, err := store.List("task-1")
	require.NoError(t, err)
	require.Equal(t, []string{"cp-sqlite"}, ids)

	events, err := workflowStore.ListEvents(ctx, "wf-1", 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "graph.checkpoint", events[0].EventType)
	require.Equal(t, "cp-sqlite", events[0].Metadata["checkpoint_id"])
}

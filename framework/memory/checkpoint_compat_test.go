package memory_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestFallbackCheckpointStoreLoadsLegacyFileCheckpoint(t *testing.T) {
	primary := memory.NewCheckpointStore(filepath.Join(t.TempDir(), "primary"))
	fallback := memory.NewCheckpointStore(filepath.Join(t.TempDir(), "fallback"))
	checkpoint := &graph.GraphCheckpoint{
		CheckpointID:    "cp-file",
		TaskID:          "task-1",
		CompletedNodeID: "persist",
		NextNodeID:      "done",
		GraphHash:       "hash",
		CreatedAt:       time.Date(2026, 3, 11, 13, 0, 0, 0, time.UTC),
		Context:         core.NewContext(),
	}
	require.NoError(t, fallback.Save(checkpoint))

	store := &memory.FallbackCheckpointStore{
		Primary:  primary,
		Fallback: fallback,
	}

	loaded, err := store.Load("task-1", "cp-file")
	require.NoError(t, err)
	require.Equal(t, "done", loaded.NextNodeID)

	ids, err := store.List("task-1")
	require.NoError(t, err)
	require.Equal(t, []string{"cp-file"}, ids)
}

func TestWorkflowSnapshotCheckpointAdapterReadsLegacySnapshot(t *testing.T) {
	store, err := memory.NewFileWorkflowStore(filepath.Join(t.TempDir(), "workflow"))
	require.NoError(t, err)

	state := core.NewContext()
	state.Set("task.phase", "resume")
	require.NoError(t, store.Save(context.Background(), &memory.WorkflowSnapshot{
		ID: "legacy-1",
		Task: &core.Task{
			ID: "task-legacy",
		},
		Graph: &graph.GraphSnapshot{
			NextNodeID: "resume-node",
			State:      state.Snapshot(),
		},
		Status:    memory.WorkflowStatusRunning,
		Metadata:  map[string]any{"source": "legacy"},
		UpdatedAt: time.Date(2026, 3, 11, 14, 0, 0, 0, time.UTC),
	}))

	adapter := &memory.WorkflowSnapshotCheckpointAdapter{Store: store}
	checkpoint, err := adapter.Load("task-legacy", "legacy-1")
	require.NoError(t, err)
	require.Equal(t, "resume-node", checkpoint.NextNodeID)
	require.Equal(t, "legacy", checkpoint.Metadata["source"])
	require.Equal(t, "resume", checkpoint.Context.GetString("task.phase"))

	ids, err := adapter.List("task-legacy")
	require.NoError(t, err)
	require.Equal(t, []string{"legacy-1"}, ids)
}

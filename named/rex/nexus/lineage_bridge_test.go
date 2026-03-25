package nexus

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestLineageBridgeHandleFrameworkEventUpdatesBindingAndAppendsWorkflowEvent(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		StartedAt:  time.Now().UTC(),
	}))

	bridge := &LineageBridge{WorkflowStore: store}
	require.NoError(t, bridge.persistBinding(ctx, "wf-1", "run-1", LineageBinding{
		LineageID: "lineage-1",
		AttemptID: "run-1",
		RuntimeID: "rex",
		State:     string(core.AttemptStateRunning),
		UpdatedAt: time.Now().UTC(),
	}))

	payload, err := json.Marshal(map[string]any{
		"lineage_id":  "lineage-1",
		"old_attempt": "run-1",
		"new_attempt": "attempt-2",
	})
	require.NoError(t, err)
	require.NoError(t, bridge.HandleFrameworkEvent(ctx, core.FrameworkEvent{
		Seq:       42,
		Timestamp: time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
		Type:      core.FrameworkEventFMPResumeCommitted,
		Payload:   payload,
	}))

	binding, err := bridge.readBinding(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Equal(t, string(core.AttemptStateCommittedRemote), binding.State)

	events, err := store.ListEvents(ctx, "wf-1", 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, core.FrameworkEventFMPResumeCommitted, events[0].EventType)
}

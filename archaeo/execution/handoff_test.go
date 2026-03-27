package execution_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/archaeo/execution"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestHandoffRecorderPersistsVersionedExecutionHandoff(t *testing.T) {
	now := time.Date(2026, 3, 27, 3, 0, 0, 0, time.UTC)
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-handoff",
		TaskID:      "task-handoff",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "handoff",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	recorder := execution.HandoffRecorder{
		Store: store,
		Now:   func() time.Time { return now },
	}
	state := core.NewContext()
	state.Set("euclo.active_exploration_id", "explore-1")
	state.Set("euclo.active_exploration_snapshot_id", "snapshot-1")
	state.Set("euclo.based_on_revision", "rev-1")
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-handoff",
		Version:    2,
	}
	step := &frameworkplan.PlanStep{ID: "step-1"}

	record, err := recorder.Record(ctx, &core.Task{}, state, plan, step)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, 2, record.PlanVersion)
	require.Equal(t, "plan-1:v2", record.HandoffRef)
	require.Equal(t, "plan-1:v2", state.GetString("euclo.execution_handoff_ref"))

	artifacts, err := store.ListWorkflowArtifacts(ctx, "wf-handoff", "")
	require.NoError(t, err)
	found := false
	for _, artifact := range artifacts {
		if artifact.Kind == "archaeo_execution_handoff" {
			found = true
			break
		}
	}
	require.True(t, found)

	events, err := store.ListEvents(ctx, "wf-handoff", 16)
	require.NoError(t, err)
	found = false
	for _, event := range events {
		if event.EventType == "archaeo.execution_handoff_recorded" {
			found = true
			require.Equal(t, "plan-1:v2", event.Message)
			break
		}
	}
	require.True(t, found)
}

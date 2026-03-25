package nexus

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestSnapshotStoreQueryWorkflowRuntimeSummarizesRexArtifacts(t *testing.T) {
	t.Parallel()

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "write a patch",
		Status:      memory.WorkflowRunStatusRunning,
		Metadata:    map[string]any{"session_id": "sess-1"},
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "rex",
		AgentMode:  "managed",
		StartedAt:  time.Now().UTC(),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "run-1:task-request",
		WorkflowID:    "wf-1",
		RunID:         "run-1",
		Kind:          "rex.task_request",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: `{"task":{"instruction":"write a patch","type":"code_generation","context":{"session_id":"sess-1"}},"state":{"session_id":"sess-1"}}`,
		CreatedAt:     time.Now().UTC(),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "run-1:proof",
		WorkflowID:    "wf-1",
		RunID:         "run-1",
		Kind:          "rex.proof_surface",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		InlineRawText: `{"verification_status":"pass"}`,
		CreatedAt:     time.Now().UTC(),
	}))

	payload, err := (SnapshotStore{WorkflowStore: store}).QueryWorkflowRuntime(ctx, "wf-1", "run-1")
	require.NoError(t, err)
	require.Equal(t, "write a patch", payload["task"].(map[string]any)["instruction"])
	require.Equal(t, "pass", payload["proof_surface"].(map[string]any)["verification_status"])
	summaries := payload["artifact_summaries"].([]map[string]any)
	require.Len(t, summaries, 2)
	_, hasRaw := summaries[0]["inline_raw_text"]
	require.False(t, hasRaw)
}

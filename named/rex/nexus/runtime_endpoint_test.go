package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"

	// "codeburg.org/lexbit/relurpify/framework/memory/db" // TODO: package does not exist
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"github.com/stretchr/testify/require"
)

func TestRuntimeEndpointCreateAttemptSchedulesRexExecution(t *testing.T) {
	t.Parallel()

	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()

	var scheduled struct {
		workflowID string
		runID      string
		task       *core.Task
		env        *contextdata.Envelope
	}
	endpoint := &RuntimeEndpoint{
		DescriptorValue: core.RuntimeDescriptor{RuntimeID: "rex"},
		WorkflowStore:   workflowStore,
		Schedule: func(_ context.Context, workflowID, runID string, task *core.Task, env *contextdata.Envelope) error {
			scheduled.workflowID = workflowID
			scheduled.runID = runID
			scheduled.task = task
			scheduled.env = env
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC) },
	}
	payload, err := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":          "task-1",
			"type":        string(core.TaskTypeCodeGeneration),
			"instruction": "resume work",
			"context":     map[string]any{"workflow_id": "wf-1"},
		},
		"state": map[string]any{"session_id": "sess-1"},
	})
	require.NoError(t, err)

	attempt, err := endpoint.CreateAttempt(context.Background(), core.LineageRecord{LineageID: "lineage-1"}, core.HandoffAccept{
		OfferID:                      "offer-1",
		ProvisionalAttemptID:         "attempt-1",
		AcceptedContextClass:         "workflow-runtime",
		AcceptedCapabilityProjection: core.CapabilityEnvelope{AllowedCapabilityIDs: []string{string(core.CapabilityExecute)}},
	}, &fwfmp.PortableContextPackage{
		Manifest:         core.ContextManifest{ContextID: "ctx-1"},
		ExecutionPayload: payload,
	})
	require.NoError(t, err)
	require.Equal(t, "attempt-1", attempt.AttemptID)
	require.Equal(t, "attempt-1", scheduled.runID)
	require.NotNil(t, scheduled.task)
	require.Equal(t, "resume work", scheduled.task.Instruction)
	val, _ := scheduled.env.GetWorkingValue("fmp.lineage_id")
	require.Equal(t, "lineage-1", fmt.Sprint(val))
	receipt, err := endpoint.IssueReceipt(context.Background(), core.LineageRecord{LineageID: "lineage-1"}, *attempt, nil)
	require.NoError(t, err)
	require.Equal(t, []string{string(core.CapabilityExecute)}, receipt.CapabilityProjectionApplied.AllowedCapabilityIDs)
}

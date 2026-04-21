package events

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestWorkflowLogReadAndSnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.CreateWorkflow(ctx, testWorkflowRecord("wf-events")))
	require.NoError(t, AppendWorkflowEvent(ctx, store, "wf-events", EventWorkflowPhaseTransitioned, "execution", map[string]any{"phase": "execution"}, NowUTC()))
	require.NoError(t, AppendWorkflowEvent(ctx, store, "wf-events", EventLearningInteractionRequested, "confirm pattern", map[string]any{"interaction_id": "learn-1"}, NowUTC()))

	log := &WorkflowLog{Store: store}
	events, err := log.Read(ctx, "wf-events", 0, 0, false)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.EqualValues(t, 1, events[0].Seq)
	require.Equal(t, EventWorkflowPhaseTransitioned, events[0].Type)
	require.EqualValues(t, 2, events[1].Seq)
	require.Equal(t, EventLearningInteractionRequested, events[1].Type)

	require.NoError(t, log.TakeSnapshot(ctx, "wf-events", 2, []byte(`{"last":2}`)))
	seq, data, err := log.LoadSnapshot(ctx, "wf-events")
	require.NoError(t, err)
	require.EqualValues(t, 2, seq)
	require.JSONEq(t, `{"last":2}`, string(data))
}

func TestAppendAndReadMutationEvents(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.CreateWorkflow(ctx, testWorkflowRecord("wf-mutations")))
	planVersion := 2
	mutation := archaeodomain.MutationEvent{
		ID:            "mutation-1",
		WorkflowID:    "wf-mutations",
		ExplorationID: "explore-1",
		PlanID:        "plan-1",
		PlanVersion:   &planVersion,
		StepID:        "step-7",
		Category:      archaeodomain.MutationStepInvalidation,
		SourceKind:    "anchor",
		SourceRef:     "anchor-1",
		Description:   "required anchor drifted",
		BlastRadius: archaeodomain.BlastRadius{
			Scope:              archaeodomain.BlastRadiusStep,
			AffectedStepIDs:    []string{"step-7"},
			AffectedSymbolIDs:  []string{"sym-1"},
			AffectedPatternIDs: []string{"pattern-1"},
			AffectedAnchorRefs: []string{"anchor-1"},
			AffectedNodeIDs:    []string{"node-1"},
			EstimatedCount:     1,
		},
		Impact:              archaeodomain.ImpactHandoffInvalidating,
		Disposition:         archaeodomain.DispositionInvalidateStep,
		Blocking:            true,
		BasedOnRevision:     "rev-1",
		SemanticSnapshotRef: "snapshot-1",
		Metadata:            map[string]any{"producer": "test"},
		CreatedAt:           NowUTC(),
	}

	require.NoError(t, AppendMutationEvent(ctx, store, mutation))

	events, err := ReadMutationEvents(ctx, store, "wf-mutations")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, mutation.ID, events[0].ID)
	require.Equal(t, mutation.WorkflowID, events[0].WorkflowID)
	require.Equal(t, mutation.Category, events[0].Category)
	require.Equal(t, mutation.BlastRadius.Scope, events[0].BlastRadius.Scope)
	require.Equal(t, mutation.BlastRadius.AffectedStepIDs, events[0].BlastRadius.AffectedStepIDs)
	require.Equal(t, mutation.Impact, events[0].Impact)
	require.Equal(t, mutation.Disposition, events[0].Disposition)
	require.True(t, events[0].Blocking)
	require.Equal(t, "test", events[0].Metadata["producer"])
	require.NotNil(t, events[0].PlanVersion)
	require.Equal(t, planVersion, *events[0].PlanVersion)

	log := &WorkflowLog{Store: store}
	timeline, err := log.Read(ctx, "wf-mutations", 0, 0, false)
	require.NoError(t, err)
	require.Len(t, timeline, 1)
	require.Equal(t, EventMutationRecorded, timeline[0].Type)
}

func TestAppendRequestEvent(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.CreateWorkflow(ctx, testWorkflowRecord("wf-requests")))
	planVersion := 5
	request := archaeodomain.RequestRecord{
		ID:            "req-1",
		WorkflowID:    "wf-requests",
		ExplorationID: "explore-1",
		PlanID:        "plan-1",
		PlanVersion:   &planVersion,
		Kind:          archaeodomain.RequestPlanReformation,
		Status:        archaeodomain.RequestStatusDispatched,
		Title:         "Reform active plan",
		RequestedBy:   "test",
		SubjectRefs:   []string{"tension-1"},
		RequestedAt:   NowUTC(),
		UpdatedAt:     NowUTC(),
		Result: &archaeodomain.RequestResult{
			Kind:    "draft_plan",
			RefID:   "plan-1:v6",
			Summary: "Created successor draft",
		},
	}

	require.NoError(t, AppendRequestEvent(ctx, store, request, EventRequestDispatched, request.Title, map[string]any{"dispatch_id": "dispatch-1"}, NowUTC()))

	log := &WorkflowLog{Store: store}
	timeline, err := log.Read(ctx, "wf-requests", 0, 0, false)
	require.NoError(t, err)
	require.Len(t, timeline, 1)
	require.Equal(t, EventRequestDispatched, timeline[0].Type)
	records, err := log.ReadRecords(ctx, "wf-requests")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "req-1", records[0].Metadata["request_id"])
	require.Equal(t, string(archaeodomain.RequestPlanReformation), records[0].Metadata["request_kind"])
	require.Equal(t, string(archaeodomain.RequestStatusDispatched), records[0].Metadata["request_status"])
	require.Equal(t, "dispatch-1", records[0].Metadata["dispatch_id"])
	require.Equal(t, "plan-1:v6", records[0].Metadata["result_ref_id"])
}

func testWorkflowRecord(workflowID string) memory.WorkflowRecord {
	return memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "test workflow",
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

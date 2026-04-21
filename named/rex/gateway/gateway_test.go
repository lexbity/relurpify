package gateway

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/named/rex/events"
)

func newGatewayStore(t *testing.T) *db.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	return store
}

func TestIdentityForEquivalentEventsStable(t *testing.T) {
	gw := DefaultGateway{}
	event := events.CanonicalEvent{Type: "job.started.v1", ActorID: "node-1", Partition: "tenant-a", IdempotencyKey: "idem-1"}
	if first, second := gw.IdentityFor(event), gw.IdentityFor(event); first != second {
		t.Fatalf("identity mismatch %q %q", first, second)
	}
}

func TestResolveDuplicateStartCollapsesToSignalForExistingWorkflow(t *testing.T) {
	store := newGatewayStore(t)
	ctx := context.Background()
	event := events.CanonicalEvent{
		ID:             "evt-1",
		Type:           events.TypeTaskRequested,
		IngressOrigin:  events.OriginPeer,
		ActorID:        "actor-1",
		Partition:      "tenant-a",
		IdempotencyKey: "idem-1",
		Payload: map[string]any{
			"instruction": "start managed work",
		},
	}
	gw := DefaultGateway{Store: store}
	workflowID := gw.IdentityFor(event)
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-1",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "start managed work",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	decision, err := gw.Resolve(ctx, event)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Decision != SignalDecisionSignal {
		t.Fatalf("expected signal decision, got %+v", decision)
	}
	if decision.WorkflowID != workflowID {
		t.Fatalf("unexpected workflow identity: %+v", decision)
	}
}

func TestResolveValidCallbackAcceptsMatchingExpectedWaitState(t *testing.T) {
	store := newGatewayStore(t)
	ctx := context.Background()
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "await callback",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-2",
		WorkflowID: "wf-2",
		Status:     memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	gw := DefaultGateway{Store: store}
	decision, err := gw.Resolve(ctx, events.CanonicalEvent{
		ID:            "evt-2",
		Type:          events.TypeCallbackReceived,
		IngressOrigin: events.OriginPeer,
		Payload: map[string]any{
			"workflow_id":       "wf-2",
			"run_id":            "run-2",
			"expected_callback": "cb-123",
			"callback_key":      "cb-123",
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if decision.Decision != SignalDecisionSignal {
		t.Fatalf("expected signal decision, got %+v", decision)
	}
}

func TestResolveRejectsStaleSignalsWithoutMutatingWorkflow(t *testing.T) {
	store := newGatewayStore(t)
	ctx := context.Background()
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-3",
		TaskID:      "task-3",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "finished workflow",
		Status:      memory.WorkflowRunStatusCompleted,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	gw := DefaultGateway{Store: store}
	decision, err := gw.Resolve(ctx, events.CanonicalEvent{
		ID:            "evt-3",
		Type:          events.TypeWorkflowSignal,
		IngressOrigin: events.OriginPeer,
		Payload: map[string]any{
			"workflow_id":     "wf-3",
			"expected_signal": "resume",
			"signal":          "resume",
		},
	})
	if err == nil {
		t.Fatalf("expected stale signal rejection")
	}
	if decision.Decision != SignalDecisionReject {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	workflow, ok, err := store.GetWorkflow(ctx, "wf-3")
	if err != nil || !ok {
		t.Fatalf("GetWorkflow ok=%v err=%v", ok, err)
	}
	if workflow.Status != memory.WorkflowRunStatusCompleted {
		t.Fatalf("workflow should remain completed: %+v", workflow)
	}
}

package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	rexmanifest "codeburg.org/lexbit/relurpify/named/rex/config"
	"codeburg.org/lexbit/relurpify/named/rex/store"
)

func TestManagerStartsStopsAndQueues(t *testing.T) {
	memStore := memory.NewWorkingMemoryStore()
	manager := New(rexmanifest.Default(), memStore)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	if !manager.Enqueue(WorkItem{WorkflowID: "wf-1", RunID: "run-1"}) {
		t.Fatalf("enqueue failed")
	}
	time.Sleep(20 * time.Millisecond)
	manager.Stop()
	health, _, _ := manager.Snapshot()
	if health == "" {
		t.Fatalf("empty health")
	}
}

func TestManagerRecoveryScanFindsRunningWorkflows(t *testing.T) {
	workflowStore, err := store.NewSQLiteWorkflowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStore: %v", err)
	}
	ctx := context.Background()
	if err := workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-running",
		TaskID:      "task-running",
		TaskType:    string(core.TaskTypePlan),
		Instruction: "resume me",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := workflowStore.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "wf-running:run",
		WorkflowID: "wf-running",
		Status:     memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	manager := New(rexmanifest.Default(), workflowStore)
	manager.scanRecoveries(ctx)
	details := manager.Details()
	if details.RecoveryCount == 0 {
		t.Fatalf("expected recoveries, got %+v", details)
	}
}

func TestManagerBeginExecutionMarksDegradedOnFailure(t *testing.T) {
	manager := New(rexmanifest.Default(), memory.NewWorkingMemoryStore())
	finish := manager.BeginExecution("wf-err", "run-err")
	finish(context.DeadlineExceeded)
	details := manager.Details()
	if details.Health != HealthDegraded {
		t.Fatalf("expected degraded health: %+v", details)
	}
	if details.LastError == "" {
		t.Fatalf("expected last error: %+v", details)
	}
}

func TestManagerScanRecoveriesRecordsErrors(t *testing.T) {
	manager := New(rexmanifest.Default(), memory.NewWorkingMemoryStore())
	manager.mem = nil
	manager.scanRecoveries(context.Background())
	if manager.Details().Health != HealthHealthy {
		t.Fatalf("expected healthy when no workflow store")
	}
	manager.recordError(errors.New("boom"))
	if manager.Details().LastError != "boom" {
		t.Fatalf("expected error recording")
	}
}

package runtime

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/core"
	rexconfig "github.com/lexcodex/relurpify/named/rex/config"
)

func TestManagerStartsAndStops(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	manager := New(rexconfig.Default(), memStore)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	manager.Stop()
	health, _, _ := manager.Snapshot()
	if health == "" {
		t.Fatalf("empty health")
	}
}

func TestManagerEnqueueDegradesWhenFull(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	cfg := rexconfig.Default()
	cfg.QueueCapacity = 1
	manager := New(cfg, memStore)
	if !manager.Enqueue(WorkItem{}) {
		t.Fatalf("first enqueue failed")
	}
	if manager.Enqueue(WorkItem{}) {
		t.Fatalf("second enqueue unexpectedly succeeded")
	}
	health, _, _ := manager.Snapshot()
	if health != HealthDegraded {
		t.Fatalf("health = %q", health)
	}
}

func TestManagerRecoveryScanFindsRunningWorkflows(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRuntimeMemoryStore: %v", err)
	}
	composite := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, db.NewSQLiteCheckpointStore(workflowStore.DB()))
	ctx := context.Background()
	if err := workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-running",
		TaskID:      "task-running",
		TaskType:    core.TaskTypePlanning,
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

	cfg := rexconfig.Default()
	cfg.RecoveryScanPeriod = 10 * time.Millisecond
	manager := New(cfg, composite)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(runCtx)
	time.Sleep(30 * time.Millisecond)

	details := manager.Details()
	if details.RecoveryCount == 0 {
		t.Fatalf("expected recoveries, got %+v", details)
	}
	if details.Recoveries[0].WorkflowID != "wf-running" {
		t.Fatalf("unexpected recovery details: %+v", details.Recoveries)
	}
}

func TestManagerProcessesQueuedWorkAndTracksLastRun(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	cfg := rexconfig.Default()
	cfg.RecoveryScanPeriod = time.Hour
	manager := New(cfg, memStore)
	done := make(chan struct{})
	manager.SetWorker(func(ctx context.Context, item WorkItem) error {
		time.Sleep(10 * time.Millisecond)
		close(done)
		return nil
	})
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(runCtx)
	if !manager.Enqueue(WorkItem{WorkflowID: "wf-1", RunID: "run-1"}) {
		t.Fatalf("enqueue failed")
	}
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for worker")
	}
	details := manager.Details()
	if details.LastWorkflowID != "wf-1" || details.LastRunID != "run-1" {
		t.Fatalf("unexpected runtime details: %+v", details)
	}
	if details.ActiveWork != 0 {
		t.Fatalf("active work should be 0, got %+v", details)
	}
}

func TestManagerBeginExecutionMarksDegradedOnFailure(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	manager := New(rexconfig.Default(), memStore)
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

func TestManagerWorkerFailureSetsDegradedHealth(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	cfg := rexconfig.Default()
	cfg.RecoveryScanPeriod = time.Hour
	manager := New(cfg, memStore)
	var calls atomic.Int32
	manager.SetWorker(func(ctx context.Context, item WorkItem) error {
		calls.Add(1)
		return context.Canceled
	})
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(runCtx)
	if !manager.Enqueue(WorkItem{WorkflowID: "wf-fail", RunID: "run-fail"}) {
		t.Fatalf("enqueue failed")
	}
	time.Sleep(30 * time.Millisecond)
	if calls.Load() == 0 {
		t.Fatalf("worker was not called")
	}
	details := manager.Details()
	if details.Health != HealthDegraded {
		t.Fatalf("expected degraded health: %+v", details)
	}
}

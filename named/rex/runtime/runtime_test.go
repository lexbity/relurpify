package runtime

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	rexconfig "codeburg.org/lexbit/relurpify/named/rex/config"
)

type stubPartitionDetector struct{ partitioned bool }

func (s stubPartitionDetector) IsPartitioned() bool { return s.partitioned }

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

func TestManagerDoesNotExecuteSameWorkflowConcurrently(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	cfg := rexconfig.Default()
	cfg.WorkerCount = 2
	cfg.RecoveryScanPeriod = time.Hour
	manager := New(cfg, memStore)
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32
	firstStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{}, 1)
	manager.SetWorker(func(ctx context.Context, item WorkItem) error {
		current := inFlight.Add(1)
		for {
			observed := maxInFlight.Load()
			if current <= observed || maxInFlight.CompareAndSwap(observed, current) {
				break
			}
		}
		if item.RunID == "run-1" {
			firstStarted <- struct{}{}
			<-releaseFirst
		} else {
			secondStarted <- struct{}{}
		}
		inFlight.Add(-1)
		return nil
	})
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(runCtx)
	if !manager.Enqueue(WorkItem{WorkflowID: "wf-same", RunID: "run-1"}) {
		t.Fatalf("first enqueue failed")
	}
	if !manager.Enqueue(WorkItem{WorkflowID: "wf-same", RunID: "run-2"}) {
		t.Fatalf("second enqueue failed")
	}
	select {
	case <-firstStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for first execution")
	}
	select {
	case <-secondStarted:
		t.Fatalf("second workflow item started before first completed")
	default:
	}
	close(releaseFirst)
	select {
	case <-secondStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for second execution")
	}
	if got := maxInFlight.Load(); got != 1 {
		t.Fatalf("expected max concurrency of 1, got %d", got)
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
	done := make(chan struct{})
	manager.SetWorker(func(ctx context.Context, item WorkItem) error {
		defer close(done)
		calls.Add(1)
		return context.Canceled
	})
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(runCtx)
	if !manager.Enqueue(WorkItem{WorkflowID: "wf-fail", RunID: "run-fail"}) {
		t.Fatalf("enqueue failed")
	}
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for worker completion")
	}
	if calls.Load() == 0 {
		t.Fatalf("worker was not called")
	}
	details := manager.Details()
	if details.Health != HealthDegraded {
		t.Fatalf("expected degraded health: %+v", details)
	}
}

func TestManagerStopWaitsForWorkerExit(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	cfg := rexconfig.Default()
	cfg.RecoveryScanPeriod = time.Hour
	manager := New(cfg, memStore)
	workerStarted := make(chan struct{})
	workerExited := make(chan struct{})
	manager.SetWorker(func(ctx context.Context, item WorkItem) error {
		close(workerStarted)
		<-ctx.Done()
		close(workerExited)
		return ctx.Err()
	})
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.Start(runCtx)
	if !manager.Enqueue(WorkItem{WorkflowID: "wf-stop", RunID: "run-stop"}) {
		t.Fatalf("enqueue failed")
	}
	select {
	case <-workerStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for worker start")
	}

	stopDone := make(chan struct{})
	go func() {
		manager.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for stop to return")
	}

	select {
	case <-workerExited:
	default:
		t.Fatalf("worker should have exited before stop returned")
	}
}

func TestManagerDetailsDegradeWhenPartitioned(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	manager := New(rexconfig.Default(), memStore)
	manager.SetPartitionDetector(stubPartitionDetector{partitioned: true})
	details := manager.Details()
	if details.Health != HealthDegraded {
		t.Fatalf("expected degraded health: %+v", details)
	}
	if !details.Partitioned {
		t.Fatalf("expected partitioned details: %+v", details)
	}
	if details.LastError == "" {
		t.Fatalf("expected partition error: %+v", details)
	}
}

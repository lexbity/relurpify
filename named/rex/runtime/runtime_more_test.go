package runtime

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/memory"
	rexmanifest "codeburg.org/lexbit/relurpify/named/rex/config"
)

func TestManagerExecuteItemBranchesAndMax(t *testing.T) {
	manager := New(rexmanifest.Default(), memory.NewWorkingMemoryStore())

	workerCalled := false
	if err := manager.executeItem(context.Background(), func(context.Context, WorkItem) error {
		workerCalled = true
		return nil
	}, WorkItem{WorkflowID: "wf-1", RunID: "run-1"}); err != nil {
		t.Fatalf("executeItem worker: %v", err)
	}
	if !workerCalled {
		t.Fatalf("expected worker to be called")
	}

	executed := false
	if err := manager.executeItem(context.Background(), nil, WorkItem{Execute: func(context.Context, WorkItem) error {
		executed = true
		return nil
	}}); err != nil {
		t.Fatalf("executeItem fallback: %v", err)
	}
	if !executed {
		t.Fatalf("expected item Execute to be called")
	}

	if max(3, 7) != 7 || max(9, 2) != 9 {
		t.Fatalf("unexpected max helper")
	}
}

func TestManagerBeginExecutionSuccessKeepsHealthHealthy(t *testing.T) {
	manager := New(rexmanifest.Default(), memory.NewWorkingMemoryStore())
	finish := manager.BeginExecution("wf-1", "run-1")
	finish(nil)
	details := manager.Details()
	if details.Health != HealthHealthy {
		t.Fatalf("expected healthy details: %+v", details)
	}
	if details.ActiveWork != 0 || details.LastWorkflowID != "wf-1" || details.LastRunID != "run-1" {
		t.Fatalf("unexpected details: %+v", details)
	}
}

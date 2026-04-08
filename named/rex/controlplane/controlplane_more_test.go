package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
)

type workflowListerStub struct {
	workflows []memory.WorkflowRecord
	runs      map[string]*memory.WorkflowRunRecord
}

func (s workflowListerStub) ListWorkflows(context.Context, int) ([]memory.WorkflowRecord, error) {
	return append([]memory.WorkflowRecord(nil), s.workflows...), nil
}

func (s workflowListerStub) GetRun(_ context.Context, runID string) (*memory.WorkflowRunRecord, bool, error) {
	if s.runs == nil {
		return nil, false, nil
	}
	run, ok := s.runs[runID]
	if !ok {
		return nil, false, nil
	}
	copy := *run
	return &copy, true, nil
}

func TestFairnessAndLoadControllersReleaseBranches(t *testing.T) {
	fairness := &FairnessController{Limits: map[string]int{"tenant-a": 1}}
	if !fairness.Admit(AdmissionRequest{TenantID: "tenant-a", Class: WorkloadBestEffort}) {
		t.Fatal("expected fairness admit")
	}
	fairness.Release(AdmissionRequest{TenantID: "tenant-a"})
	if !fairness.Admit(AdmissionRequest{TenantID: "tenant-a", Class: WorkloadBestEffort}) {
		t.Fatal("expected fairness admit after release")
	}
	fairness.Release(AdmissionRequest{TenantID: "tenant-a"})

	load := &LoadController{Capacity: 2, Fairness: fairness}
	if !load.Admit(AdmissionRequest{TenantID: "tenant-b", Class: WorkloadImportant}) {
		t.Fatal("expected load admit")
	}
	load.Release(AdmissionRequest{TenantID: "tenant-b"})
	if load.InFlight != 0 {
		t.Fatalf("expected in-flight reset, got %d", load.InFlight)
	}

	critical := load.Decide(AdmissionRequest{TenantID: "tenant-c", Class: WorkloadCritical})
	if !critical.Allowed || critical.Reason != "critical_bypass" {
		t.Fatalf("unexpected critical decision: %+v", critical)
	}
}

func TestValidationAndSLOHelpersCoverFailureBranches(t *testing.T) {
	if err := ValidateOperatorAction(OperatorAction{}); err == nil {
		t.Fatal("expected validation failure")
	}
	if got := errorString(nil); got != "" {
		t.Fatalf("unexpected errorString: %q", got)
	}
	if got := firstNonEmpty(" ", "first", "second"); got != "first" {
		t.Fatalf("unexpected firstNonEmpty: %q", got)
	}

	signals, err := CollectSLOSignals(context.Background(), workflowListerStub{
		workflows: []memory.WorkflowRecord{
			{WorkflowID: "wf-running", Status: memory.WorkflowRunStatusRunning},
			{WorkflowID: "wf-completed", Status: memory.WorkflowRunStatusCompleted},
			{WorkflowID: "wf-canceled", Status: memory.WorkflowRunStatusCanceled},
		},
		runs: map[string]*memory.WorkflowRunRecord{
			"wf-running:run": {RunID: "wf-running:run", AgentMode: "recover-managed"},
		},
	}, 10)
	if err != nil {
		t.Fatalf("CollectSLOSignals: %v", err)
	}
	if signals.TotalWorkflows != 3 || signals.CompletedWorkflows != 1 || signals.FailedWorkflows != 1 {
		t.Fatalf("unexpected SLO signals: %+v", signals)
	}
	if signals.RecoverySensitive < 2 {
		t.Fatalf("expected recovery-sensitive count to include workflow and run: %+v", signals)
	}

	metadata := BuildDRMetadata(memory.WorkflowRecord{WorkflowID: "wf-dr", Status: memory.WorkflowRunStatusRunning}, &memory.WorkflowRunRecord{
		RunID:          "wf-dr:run",
		WorkflowID:     "wf-dr",
		RuntimeVersion: "rex.v1",
		StartedAt:      time.Now().UTC(),
	})
	if !metadata.FailoverReady || metadata.RecoveryState != string(memory.WorkflowRunStatusRunning) {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}

	if !isPrivilegedRole("nexus:operator") || isPrivilegedRole("viewer") {
		t.Fatal("unexpected privilege role classification")
	}
}

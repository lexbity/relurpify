package controlplane

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	rexstore "codeburg.org/lexbit/relurpify/named/rex/store"
)

func TestFairnessControllerEnforcesTenantLimit(t *testing.T) {
	controller := &FairnessController{Limits: map[string]int{"tenant-a": 1}}
	if !controller.Admit(AdmissionRequest{TenantID: "tenant-a", Class: WorkloadBestEffort}) {
		t.Fatalf("first admit failed")
	}
	if controller.Admit(AdmissionRequest{TenantID: "tenant-a", Class: WorkloadBestEffort}) {
		t.Fatalf("second admit unexpectedly succeeded")
	}
}

func TestLoadControllerShedsBestEffortUnderOverloadButAllowsCritical(t *testing.T) {
	controller := &LoadController{Capacity: 1}
	first := controller.Decide(AdmissionRequest{TenantID: "tenant-a", Class: WorkloadImportant})
	if !first.Allowed {
		t.Fatalf("expected first admission")
	}
	second := controller.Decide(AdmissionRequest{TenantID: "tenant-b", Class: WorkloadBestEffort})
	if second.Allowed || second.Reason != "over_capacity" {
		t.Fatalf("expected overload rejection, got %+v", second)
	}
	critical := controller.Decide(AdmissionRequest{TenantID: "tenant-c", Class: WorkloadCritical})
	if !critical.Allowed {
		t.Fatalf("critical workload should bypass overload: %+v", critical)
	}
}

func TestAuthorizeOperatorActionRequiresPrivilegedRoleAndAudits(t *testing.T) {
	audit := &AuditLog{}
	if err := AuthorizeOperatorAction(OperatorAction{Action: "pause", Role: "viewer", TenantID: "tenant-a"}, audit); err == nil {
		t.Fatalf("expected authorization failure")
	}
	if err := AuthorizeOperatorAction(OperatorAction{Action: "pause", Role: "nexus:operator", TenantID: "tenant-a"}, audit); err != nil {
		t.Fatalf("unexpected authorization error: %v", err)
	}
	records := audit.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 audit records, got %+v", records)
	}
	if records[0].Allowed {
		t.Fatalf("expected first record denied: %+v", records[0])
	}
	if !records[1].Allowed {
		t.Fatalf("expected second record allowed: %+v", records[1])
	}
}

func TestCollectSLOSignalsAggregatesWorkflowClasses(t *testing.T) {
	store, err := rexstore.NewSQLiteWorkflowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStore: %v", err)
	}
	ctx := context.Background()
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-running",
		TaskID:      "task-running",
		TaskType:    string(core.TaskTypePlan),
		Instruction: "running workflow",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow running: %v", err)
	}
	if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:          "wf-running:run",
		WorkflowID:     "wf-running",
		Status:         memory.WorkflowRunStatusRunning,
		AgentMode:      "recover-managed",
		RuntimeVersion: "rex.v1",
		StartedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateRun running: %v", err)
	}
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-failed",
		TaskID:      "task-failed",
		TaskType:    string(core.TaskTypeReview),
		Instruction: "failed workflow",
		Status:      memory.WorkflowRunStatusFailed,
	}); err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}
	signals, err := CollectSLOSignals(ctx, store, 10)
	if err != nil {
		t.Fatalf("CollectSLOSignals: %v", err)
	}
	if signals.RunningWorkflows != 1 || signals.FailedWorkflows != 1 {
		t.Fatalf("unexpected signal counts: %+v", signals)
	}
	if signals.RecoverySensitive == 0 {
		t.Fatalf("expected recovery-sensitive workflows: %+v", signals)
	}
	if len(signals.DegradedWorkflowIDs) != 1 || signals.DegradedWorkflowIDs[0] != "wf-failed" {
		t.Fatalf("unexpected degraded workflows: %+v", signals)
	}
}

func TestBuildDRMetadataEmitsFailoverSensitiveWorkflowState(t *testing.T) {
	startedAt := time.Now().UTC()
	workflow := memory.WorkflowRecord{
		WorkflowID: "wf-dr",
		Status:     memory.WorkflowRunStatusRunning,
	}
	run := &memory.WorkflowRunRecord{
		RunID:          "wf-dr:run",
		WorkflowID:     "wf-dr",
		RuntimeVersion: "rex.v1",
		StartedAt:      startedAt,
	}
	metadata := BuildDRMetadata(workflow, run)
	if !metadata.FailoverReady {
		t.Fatalf("expected failover readiness: %+v", metadata)
	}
	if metadata.RunID != "wf-dr:run" || metadata.RuntimeVersion != "rex.v1" {
		t.Fatalf("unexpected dr metadata: %+v", metadata)
	}
	if metadata.LastCheckpoint.IsZero() {
		t.Fatalf("expected last checkpoint timestamp: %+v", metadata)
	}
}

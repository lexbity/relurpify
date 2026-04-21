package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memdb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/named/rex"
	rexconfig "codeburg.org/lexbit/relurpify/named/rex/config"
	rexnexus "codeburg.org/lexbit/relurpify/named/rex/nexus"
	rexproof "codeburg.org/lexbit/relurpify/named/rex/proof"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
)

func TestRexRuntimeProviderRuntimeProjectionIncludesDRMetadata(t *testing.T) {
	ctx := context.Background()
	store, err := memdb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	defer store.Close()

	startedAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-dr",
		TaskID:      "task-dr",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "exercise dr metadata",
		Status:      memory.WorkflowRunStatusNeedsReplan,
		CreatedAt:   startedAt,
		UpdatedAt:   startedAt,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:          "run-dr",
		WorkflowID:     "wf-dr",
		Status:         memory.WorkflowRunStatusRunning,
		RuntimeVersion: "rex.v7",
		StartedAt:      startedAt,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	agent := &rex.Agent{
		Runtime:   rexruntime.New(rexconfig.Default(), nil),
		LastProof: rexproof.ProofSurface{VerificationStatus: "pass"},
	}
	finish := agent.Runtime.BeginExecution("wf-dr", "run-dr")
	finish(nil)
	provider := &RexRuntimeProvider{
		Agent:         agent,
		Adapter:       rexnexus.NewAdapter("rex", agent, nil),
		WorkflowStore: store,
	}

	projection := provider.RuntimeProjection()
	if !projection.FailoverReady {
		t.Fatalf("expected failover-ready projection: %+v", projection)
	}
	if projection.RecoveryState != string(memory.WorkflowRunStatusNeedsReplan) {
		t.Fatalf("unexpected recovery state: %+v", projection)
	}
	if projection.RuntimeVersion != "rex.v7" {
		t.Fatalf("unexpected runtime version: %+v", projection)
	}
	if !projection.LastCheckpoint.Equal(startedAt) {
		t.Fatalf("unexpected last checkpoint: %+v", projection)
	}
}

type unhealthyBridge struct {
	lastError string
}

func (b unhealthyBridge) Health() (bool, string) {
	return false, b.lastError
}

func TestRexRuntimeProviderRuntimeProjectionUsesEventBridgeHealth(t *testing.T) {
	provider := &RexRuntimeProvider{
		Agent:       &rex.Agent{Runtime: rexruntime.New(rexconfig.Default(), nil)},
		Adapter:     rexnexus.NewAdapter("rex", &rex.Agent{Runtime: rexruntime.New(rexconfig.Default(), nil)}, nil),
		EventBridge: unhealthyBridge{lastError: "bridge down"},
	}

	projection := provider.RuntimeProjection()
	if projection.Health != rexruntime.HealthDegraded {
		t.Fatalf("expected degraded projection: %+v", projection)
	}
	if projection.LastError != "bridge down" {
		t.Fatalf("unexpected last error: %+v", projection)
	}
}

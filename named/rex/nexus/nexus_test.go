package nexus

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	rexconfig "github.com/lexcodex/relurpify/named/rex/config"
	"github.com/lexcodex/relurpify/named/rex/proof"
	"github.com/lexcodex/relurpify/named/rex/runtime"
)

type stubManagedRuntime struct {
	projection   Projection
	capabilities []core.Capability
	result       *core.Result
	err          error
	invoked      bool
}

func (s *stubManagedRuntime) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	s.invoked = true
	if s.result == nil {
		s.result = &core.Result{Success: s.err == nil, Data: map[string]any{"rex.workflow_id": s.projection.WorkflowID}}
	}
	return s.result, s.err
}

func (s *stubManagedRuntime) RuntimeProjection() Projection { return s.projection }
func (s *stubManagedRuntime) Capabilities() []core.Capability {
	return append([]core.Capability{}, s.capabilities...)
}

func testWorkflowStore(t *testing.T) memory.WorkflowStateStore {
	t.Helper()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	return workflowStore
}

func TestBuildProjection(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	manager := runtime.New(rexconfig.Default(), memStore)
	finish := manager.BeginExecution("wf-1", "run-1")
	finish(nil)
	projection := BuildProjection(manager, proof.ProofSurface{RouteFamily: "react"})
	if projection.WorkflowID != "wf-1" || projection.RunID != "run-1" {
		t.Fatalf("unexpected projection: %+v", projection)
	}
}

func TestAdapterRegistrationExposesManagedRuntimeMetadata(t *testing.T) {
	store := testWorkflowStore(t)
	managed := &stubManagedRuntime{
		projection:   Projection{WorkflowID: "wf-1", RunID: "run-1"},
		capabilities: []core.Capability{core.CapabilityPlan, core.CapabilityExecute},
	}
	adapter := NewAdapter("rex", managed, store)
	registration := adapter.Registration()
	if !registration.Managed || registration.Name != "rex" {
		t.Fatalf("unexpected registration: %+v", registration)
	}
	if len(registration.Capabilities) == 0 {
		t.Fatalf("expected capabilities: %+v", registration)
	}
}

func TestAdapterInvokeUsesRuntimeExecutionPath(t *testing.T) {
	store := testWorkflowStore(t)
	managed := &stubManagedRuntime{
		projection: Projection{WorkflowID: "wf-1", RunID: "run-1"},
		result:     &core.Result{Success: true, Data: map[string]any{"rex.workflow_id": "wf-1"}},
	}
	adapter := NewAdapter("rex", managed, store)
	result, err := adapter.Invoke(context.Background(), &core.Task{
		ID:          "task-1",
		Instruction: "review current state",
		Type:        core.TaskTypeReview,
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": false},
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !managed.invoked {
		t.Fatalf("expected runtime invocation")
	}
	if result == nil || result.Data["rex.workflow_id"] == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAdapterAdminSnapshotProjectsWorkflowState(t *testing.T) {
	store := testWorkflowStore(t)
	ctx := context.Background()
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-2",
		TaskID:      "task-2",
		TaskType:    core.TaskTypeReview,
		Instruction: "review workflow projection",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-2",
		WorkflowID: "wf-2",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "rex",
		AgentMode:  "managed",
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	managed := &stubManagedRuntime{
		projection: Projection{
			Health:     runtime.HealthHealthy,
			WorkflowID: "wf-2",
			RunID:      "run-2",
		},
		capabilities: []core.Capability{core.CapabilityPlan},
	}
	adapter := NewAdapter("rex", managed, store)
	snapshot, err := adapter.AdminSnapshot(context.Background())
	if err != nil {
		t.Fatalf("AdminSnapshot: %v", err)
	}
	if snapshot.Runtime.WorkflowID == "" || snapshot.Runtime.RunID == "" {
		t.Fatalf("missing runtime identifiers: %+v", snapshot)
	}
	if len(snapshot.WorkflowRefURI) == 0 {
		t.Fatalf("expected workflow refs: %+v", snapshot)
	}
	if snapshot.HotState["workflow_id"] == "" {
		t.Fatalf("expected hot state payload: %+v", snapshot.HotState)
	}
	if snapshot.WarmState["workflow_id"] == "" {
		t.Fatalf("expected warm state payload: %+v", snapshot.WarmState)
	}
}

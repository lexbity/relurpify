package restore

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

type restoreFakeProvider struct {
	descriptor     core.ProviderDescriptor
	snapshot       core.ProviderSnapshot
	sessions       []core.ProviderSessionSnapshot
	restored       []core.ProviderSnapshot
	sessionRestore []core.ProviderSessionSnapshot
}

func (p *restoreFakeProvider) Descriptor() core.ProviderDescriptor { return p.descriptor }

func (p *restoreFakeProvider) Initialize(context.Context, core.ProviderRuntime) error { return nil }

func (p *restoreFakeProvider) RegisterCapabilities(context.Context, core.CapabilityRegistrar) error {
	return nil
}

func (p *restoreFakeProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return nil, nil
}

func (p *restoreFakeProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{Status: "ok"}, nil
}

func (p *restoreFakeProvider) Close(context.Context) error { return nil }

func (p *restoreFakeProvider) SnapshotProvider(context.Context) (*core.ProviderSnapshot, error) {
	s := p.snapshot
	return &s, nil
}

func (p *restoreFakeProvider) SnapshotSessions(context.Context) ([]core.ProviderSessionSnapshot, error) {
	return append([]core.ProviderSessionSnapshot(nil), p.sessions...), nil
}

func (p *restoreFakeProvider) RestoreProvider(_ context.Context, snapshot core.ProviderSnapshot) error {
	p.restored = append(p.restored, snapshot)
	return nil
}

func (p *restoreFakeProvider) RestoreSession(_ context.Context, snapshot core.ProviderSessionSnapshot) error {
	p.sessionRestore = append(p.sessionRestore, snapshot)
	return nil
}

type restoreBasicProvider struct {
	descriptor core.ProviderDescriptor
}

func (p *restoreBasicProvider) Descriptor() core.ProviderDescriptor { return p.descriptor }

func (p *restoreBasicProvider) Initialize(context.Context, core.ProviderRuntime) error { return nil }

func (p *restoreBasicProvider) RegisterCapabilities(context.Context, core.CapabilityRegistrar) error {
	return nil
}

func (p *restoreBasicProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return nil, nil
}

func (p *restoreBasicProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{Status: "ok"}, nil
}

func (p *restoreBasicProvider) Close(context.Context) error { return nil }

func TestResolveRuntimeSurfaces(t *testing.T) {
	if got := ResolveRuntimeSurfaces(nil); got.Workflow != nil {
		t.Fatalf("expected zero surfaces for nil store, got %#v", got)
	}

	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("open workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })

	composite := memory.NewCompositeRuntimeStore(workflowStore, nil, nil)
	surfaces := ResolveRuntimeSurfaces(composite)
	if surfaces.Workflow == nil {
		t.Fatalf("expected composite runtime surfaces, got %#v", surfaces)
	}
}

func TestEnsureWorkflowRun(t *testing.T) {
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-1")
	state.Set("euclo.run_id", "run-1")
	task := &core.Task{
		ID:          "task-1",
		Type:        core.TaskTypeCodeModification,
		Instruction: "update tests",
	}

	workflowID, runID, err := EnsureWorkflowRun(context.Background(), store, task, state)
	if err != nil {
		t.Fatalf("ensure workflow: %v", err)
	}
	if workflowID != "wf-1" || runID != "run-1" {
		t.Fatalf("unexpected ids: %q %q", workflowID, runID)
	}
	if got := state.GetString("euclo.workflow_id"); got != "wf-1" {
		t.Fatalf("expected state workflow id to persist, got %q", got)
	}

	workflow, ok, err := store.GetWorkflow(context.Background(), "wf-1")
	if err != nil || !ok {
		t.Fatalf("expected workflow to exist: %v %v", ok, err)
	}
	if workflow.TaskID != "task-1" || workflow.Instruction != "update tests" {
		t.Fatalf("unexpected workflow record: %#v", workflow)
	}
	run, ok, err := store.GetRun(context.Background(), "run-1")
	if err != nil || !ok {
		t.Fatalf("expected run to exist: %v %v", ok, err)
	}
	if run.WorkflowID != "wf-1" || run.AgentName != "euclo" {
		t.Fatalf("unexpected run record: %#v", run)
	}

	fallbackWorkflow, fallbackRun, err := EnsureWorkflowRun(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("nil store should not error: %v", err)
	}
	if fallbackWorkflow != "" || fallbackRun != "" {
		t.Fatalf("expected nil store to return empty ids, got %q %q", fallbackWorkflow, fallbackRun)
	}
}

func TestCapturePersistRestoreAndApplyProviderRuntimeState(t *testing.T) {
	state := core.NewContext()
	snapshotting := &restoreFakeProvider{
		descriptor: core.ProviderDescriptor{ID: "provider-a", Kind: core.ProviderKindBuiltin},
		snapshot: core.ProviderSnapshot{
			ProviderID: "provider-a",
			Descriptor: core.ProviderDescriptor{ID: "provider-a", Kind: core.ProviderKindBuiltin},
			Metadata:   map[string]any{"role": "primary"},
		},
		sessions: []core.ProviderSessionSnapshot{{
			Session:  core.ProviderSession{ID: "session-a", ProviderID: "provider-a"},
			Metadata: map[string]any{"state": "active"},
		}},
	}
	unsupported := &restoreBasicProvider{descriptor: core.ProviderDescriptor{ID: "provider-b", Kind: core.ProviderKindBuiltin}}

	captured := CaptureProviderRuntimeState(context.Background(), []core.Provider{snapshotting, unsupported}, state)
	if !captured.Restored || len(captured.Outcomes) == 0 {
		t.Fatalf("expected capture outcomes, got %#v", captured)
	}
	if _, ok := state.Get("euclo.provider_snapshots"); !ok {
		t.Fatal("expected provider snapshots on state")
	}
	if _, ok := state.Get("euclo.provider_session_snapshots"); !ok {
		t.Fatal("expected provider session snapshots on state")
	}

	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-restore",
		TaskID:      "task-restore",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "restore test",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "run-restore",
		WorkflowID: "wf-restore",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "euclo",
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	persisted, err := PersistProviderSnapshotState(context.Background(), store, "wf-restore", "run-restore", state, "task-restore")
	if err != nil {
		t.Fatalf("persist provider state: %v", err)
	}
	if len(persisted.ProviderSnapshotRefs) == 0 || len(persisted.SessionSnapshotRefs) == 0 {
		t.Fatalf("expected persisted refs, got %#v", persisted)
	}

	reloadedState := core.NewContext()
	restored, err := RestoreProviderSnapshotState(context.Background(), store, "wf-restore", "run-restore", reloadedState)
	if err != nil {
		t.Fatalf("restore provider state: %v", err)
	}
	if !restored.Restored || len(restored.ProviderSnapshotRefs) == 0 {
		t.Fatalf("expected restored refs, got %#v", restored)
	}

	applyState := core.NewContext()
	applyState.Set("euclo.provider_snapshots", []core.ProviderSnapshot{snapshotting.snapshot})
	applyState.Set("euclo.provider_session_snapshots", []core.ProviderSessionSnapshot{snapshotting.sessions[0]})
	applied, err := ApplyProviderRuntimeRestore(context.Background(), []core.Provider{snapshotting, unsupported}, applyState)
	if err != nil {
		t.Fatalf("apply restore: %v", err)
	}
	if !applied.Restored || len(snapshotting.restored) == 0 || len(snapshotting.sessionRestore) == 0 {
		t.Fatalf("expected provider restore callbacks to fire, got %#v", applied)
	}
	if !reflect.DeepEqual(applied.RestoredProviders, []string{"provider-a"}) {
		t.Fatalf("unexpected restored providers: %#v", applied.RestoredProviders)
	}
}

func TestEnsureWorkflowRun_WritesWorkspaceAndMode(t *testing.T) {
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	state := core.NewContext()
	state.Set("euclo.mode", "code")
	task := &core.Task{
		ID:          "task-ws",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "test with workspace",
		Context:     map[string]any{"workspace": "/test/workspace/path"},
	}

	workflowID, runID, err := EnsureWorkflowRun(context.Background(), store, task, state)
	if err != nil {
		t.Fatalf("ensure workflow: %v", err)
	}
	if workflowID == "" || runID == "" {
		t.Fatal("expected non-empty workflow and run IDs")
	}

	// Verify workflow metadata contains workspace and mode
	workflow, ok, err := store.GetWorkflow(context.Background(), workflowID)
	if err != nil || !ok {
		t.Fatalf("expected workflow to exist: %v %v", ok, err)
	}
	if got := workflow.Metadata["workspace"]; got != "/test/workspace/path" {
		t.Fatalf("expected workspace in metadata, got %v", got)
	}
	if got := workflow.Metadata["mode"]; got != "code" {
		t.Fatalf("expected mode in metadata, got %v", got)
	}
	// Verify agent key is preserved (set during creation)
	if got := workflow.Metadata["agent"]; got != "euclo" {
		t.Fatalf("expected agent=euclo in metadata, got %v", got)
	}
}

package restore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestRestoreProviderSnapshotState_NilInputs(t *testing.T) {
	state := core.NewContext()
	rs, err := RestoreProviderSnapshotState(context.Background(), nil, "wf", "run", state)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if rs.Restored || rs.WorkflowID != "" {
		t.Fatalf("expected empty outcome, got %+v", rs)
	}
	if _, err = RestoreProviderSnapshotState(context.Background(), nil, "wf", "run", nil); err != nil {
		t.Fatalf("nil state: %v", err)
	}
}

func TestRestoreProviderSnapshotState_EmptyListsStillUpdatesState(t *testing.T) {
	sqlStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = sqlStore.Close() })
	ctx := context.Background()
	if err := sqlStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-restore-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "test",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := sqlStore.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-restore-1",
		WorkflowID: "wf-restore-1",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "euclo",
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	state := core.NewContext()
	rs, err := RestoreProviderSnapshotState(ctx, sqlStore, "wf-restore-1", "run-restore-1", state)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if rs.Restored {
		t.Fatal("expected Restored false when snapshot tables are empty")
	}
	if _, ok := state.Get("euclo.provider_snapshots"); !ok {
		t.Fatal("expected euclo.provider_snapshots to be set")
	}
	if _, ok := state.Get("euclo.provider_restore"); !ok {
		t.Fatal("expected euclo.provider_restore to be set")
	}
}

func TestLoadPersistedArtifacts_SQLiteRoundTrip(t *testing.T) {
	sqlStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "artifacts.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = sqlStore.Close() })
	ctx := context.Background()
	if err := sqlStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-a",
		TaskID:      "task-a",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "test",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := sqlStore.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-a",
		WorkflowID: "wf-a",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "euclo",
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	artifacts := []euclotypes.Artifact{{
		ID: "art-1", Kind: euclotypes.ArtifactKindPlan, Summary: "saved",
		Payload: map[string]any{"step": 1}, ProducerID: "euclo:test", Status: "produced",
	}}
	if err := euclotypes.PersistWorkflowArtifacts(ctx, sqlStore, "wf-a", "run-a", artifacts); err != nil {
		t.Fatalf("persist: %v", err)
	}
	loaded, err := euclotypes.LoadPersistedArtifacts(ctx, sqlStore, "wf-a", "run-a")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != "art-1" {
		t.Fatalf("unexpected loaded: %#v", loaded)
	}
	payload, ok := loaded[0].Payload.(map[string]any)
	if !ok || payload["step"] != float64(1) {
		t.Fatalf("payload not round-tripped: %#v", loaded[0].Payload)
	}
}

func TestLoadPersistedArtifacts_ReaderError(t *testing.T) {
	store := errListArtifactsStore{err: errors.New("list failed")}
	_, err := euclotypes.LoadPersistedArtifacts(context.Background(), store, "wf-e", "run-e")
	if err == nil {
		t.Fatal("expected error from reader")
	}
}

func TestRestoreStateFromArtifacts_KnownAndUnknownKinds(t *testing.T) {
	state := core.NewContext()
	known := euclotypes.Artifact{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"x": 1}}
	unknown := euclotypes.Artifact{Kind: euclotypes.ArtifactKind("euclo.unknown_phase5_kind"), Payload: map[string]any{"y": 2}}
	euclotypes.RestoreStateFromArtifacts(state, []euclotypes.Artifact{known, unknown})
	if _, ok := state.Get("pipeline.plan"); !ok {
		t.Fatal("expected plan key from known kind")
	}
	raw, ok := state.Get("euclo.artifacts")
	if !ok {
		t.Fatal("expected euclo.artifacts")
	}
	arts := raw.([]euclotypes.Artifact)
	if len(arts) != 2 {
		t.Fatalf("expected both artifacts in slice, got %d", len(arts))
	}
}

type errListArtifactsStore struct {
	err error
}

func (e errListArtifactsStore) ListWorkflowArtifacts(context.Context, string, string) ([]memory.WorkflowArtifactRecord, error) {
	return nil, e.err
}

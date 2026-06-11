package persistence

import (
	"context"
	"os"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

func setupTestDB(t *testing.T) *graphdb.Engine {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := graphdb.Open(context.Background(), graphdb.Options{
		DataDir:          tmpDir,
		AOFFileName:      "test.aof",
		SnapshotFileName: "test.snapshot",
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	return db
}

func TestLifecycleRepository_CreateWorkflow(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	workflow := contextports.WorkflowRecord{
		WorkflowID: "wf-test-1",
		Metadata:   map[string]any{"key": "value"},
	}

	err := repo.CreateWorkflow(workflow)
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}

	// Verify it was created
	retrieved, err := repo.GetWorkflow("wf-test-1")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if retrieved.WorkflowID != "wf-test-1" {
		t.Errorf("expected WorkflowID wf-test-1, got %s", retrieved.WorkflowID)
	}
}

func TestLifecycleRepository_GetWorkflow(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	workflow := contextports.WorkflowRecord{
		WorkflowID: "wf-test-2",
		Metadata:   map[string]any{"key": "value"},
	}

	_ = repo.CreateWorkflow(workflow)

	retrieved, err := repo.GetWorkflow("wf-test-2")
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if retrieved.WorkflowID != "wf-test-2" {
		t.Errorf("expected WorkflowID wf-test-2, got %s", retrieved.WorkflowID)
	}
}

func TestLifecycleRepository_ListWorkflows(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-2"})

	workflows, err := repo.ListWorkflows("")
	if err != nil {
		t.Fatalf("ListWorkflows failed: %v", err)
	}
	if len(workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(workflows))
	}
}

func TestLifecycleRepository_CreateRun(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	// First create a workflow
	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})

	run := contextports.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     "running",
	}

	err := repo.CreateRun(run)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Verify it was created
	retrieved, err := repo.GetRun("run-1")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if retrieved.RunID != "run-1" {
		t.Errorf("expected RunID run-1, got %s", retrieved.RunID)
	}
}

func TestLifecycleRepository_ListRuns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-2", WorkflowID: "wf-1"})

	runs, err := repo.ListRuns("wf-1")
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runs))
	}
}

func TestLifecycleRepository_UpdateRunStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1", Status: "running"})

	err := repo.UpdateRunStatus("run-1", "completed")
	if err != nil {
		t.Fatalf("UpdateRunStatus failed: %v", err)
	}

	retrieved, _ := repo.GetRun("run-1")
	if retrieved.Status != "completed" {
		t.Errorf("expected status completed, got %s", retrieved.Status)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestLifecycleRepository_UpsertDelegation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	delegation := contextports.DelegationEntry{
		DelegationID:     "del-1",
		WorkflowID:       "wf-1",
		RunID:            "run-1",
		State:            "active",
		TargetProviderID: "provider-1",
	}

	err := repo.UpsertDelegation(delegation)
	if err != nil {
		t.Fatalf("UpsertDelegation failed: %v", err)
	}

	retrieved, err := repo.GetDelegation("del-1")
	if err != nil {
		t.Fatalf("GetDelegation failed: %v", err)
	}
	if retrieved.DelegationID != "del-1" {
		t.Errorf("expected DelegationID del-1, got %s", retrieved.DelegationID)
	}
}

func TestLifecycleRepository_ListDelegations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	_ = repo.UpsertDelegation(contextports.DelegationEntry{DelegationID: "del-1", WorkflowID: "wf-1", RunID: "run-1"})
	_ = repo.UpsertDelegation(contextports.DelegationEntry{DelegationID: "del-2", WorkflowID: "wf-1", RunID: "run-1"})

	delegations, err := repo.ListDelegations("wf-1")
	if err != nil {
		t.Fatalf("ListDelegations failed: %v", err)
	}
	if len(delegations) != 2 {
		t.Errorf("expected 2 delegations, got %d", len(delegations))
	}
}

func TestLifecycleRepository_AppendDelegationTransition(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.UpsertDelegation(contextports.DelegationEntry{DelegationID: "del-1", WorkflowID: "wf-1"})

	transition := contextports.DelegationTransitionEntry{
		DelegationID: "del-1",
		ToState:      "completed",
	}

	err := repo.AppendDelegationTransition(transition)
	if err != nil {
		t.Fatalf("AppendDelegationTransition failed: %v", err)
	}

	transitions, err := repo.ListDelegationTransitions("del-1")
	if err != nil {
		t.Fatalf("ListDelegationTransitions failed: %v", err)
	}
	if len(transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(transitions))
	}
}

func TestLifecycleRepository_AppendEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	event := contextports.WorkflowEventRecord{
		EventID:    "evt-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		EventType:  "test_event",
		Sequence:   1,
		Payload:    map[string]any{"msg": "test"},
	}

	err := repo.AppendEvent(event)
	if err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}

	events, err := repo.ListEventsByRun("run-1", 10)
	if err != nil {
		t.Fatalf("ListEventsByRun failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestLifecycleRepository_UpsertArtifact(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	artifact := contextports.WorkflowArtifactRecord{
		ArtifactID:  "art-1",
		WorkflowID:  "wf-1",
		RunID:       "run-1",
		StorageKind: "inline",
		ContentType: "text/plain",
	}

	err := repo.UpsertArtifact(artifact)
	if err != nil {
		t.Fatalf("UpsertArtifact failed: %v", err)
	}

	retrieved, err := repo.GetArtifact("art-1")
	if err != nil {
		t.Fatalf("GetArtifact failed: %v", err)
	}
	if retrieved.ArtifactID != "art-1" {
		t.Errorf("expected ArtifactID art-1, got %s", retrieved.ArtifactID)
	}
}

func TestLifecycleRepository_ListArtifacts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	_ = repo.UpsertArtifact(contextports.WorkflowArtifactRecord{ArtifactID: "art-1", WorkflowID: "wf-1", RunID: "run-1"})
	_ = repo.UpsertArtifact(contextports.WorkflowArtifactRecord{ArtifactID: "art-2", WorkflowID: "wf-1", RunID: "run-1"})

	artifacts, err := repo.ListArtifactsByRun("run-1")
	if err != nil {
		t.Fatalf("ListArtifactsByRun failed: %v", err)
	}
	if len(artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(artifacts))
	}
}

func TestLifecycleRepository_UpsertLineageBinding(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	binding := contextports.LineageBindingRecord{
		BindingID:    "lb-1",
		WorkflowID:   "wf-1",
		FromRunID:    "run-1",
		FromEntityID: "lineage-1",
		ToEntityID:   "attempt-1",
	}

	err := repo.UpsertLineageBinding(binding)
	if err != nil {
		t.Fatalf("UpsertLineageBinding failed: %v", err)
	}

	retrieved, err := repo.GetLineageBinding("lb-1")
	if err != nil {
		t.Fatalf("GetLineageBinding failed: %v", err)
	}
	if retrieved.BindingID != "lb-1" {
		t.Errorf("expected BindingID lb-1, got %s", retrieved.BindingID)
	}
}

func TestLifecycleRepository_FindLineageBindingByLineageID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	binding := contextports.LineageBindingRecord{
		BindingID:    "lb-1",
		WorkflowID:   "wf-1",
		FromRunID:    "run-1",
		FromEntityID: "lineage-123",
		ToEntityID:   "attempt-1",
	}

	_ = repo.UpsertLineageBinding(binding)

	bindings, err := repo.FindLineageBindingsByFrom("lineage-123")
	if err != nil {
		t.Fatalf("FindLineageBindingsByFrom failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].FromEntityID != "lineage-123" {
		t.Errorf("expected FromEntityID lineage-123, got %s", bindings[0].FromEntityID)
	}
}

func TestLifecycleRepository_FindLineageBindingByAttemptID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-1", WorkflowID: "wf-1"})

	binding := contextports.LineageBindingRecord{
		BindingID:    "lb-1",
		WorkflowID:   "wf-1",
		FromRunID:    "run-1",
		FromEntityID: "lineage-1",
		ToEntityID:   "attempt-456",
	}

	_ = repo.UpsertLineageBinding(binding)

	bindings, err := repo.FindLineageBindingsByTo("attempt-456")
	if err != nil {
		t.Fatalf("FindLineageBindingsByTo failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].ToEntityID != "attempt-456" {
		t.Errorf("expected ToEntityID attempt-456, got %s", bindings[0].ToEntityID)
	}
}

func TestLifecycleRepository_Close(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := graphdb.Open(context.Background(), graphdb.Options{
		DataDir:          tmpDir,
		AOFFileName:      "test.aof",
		SnapshotFileName: "test.snapshot",
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	repo := NewLifecycleRepository(db)
	err = repo.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify temp dir was cleaned up
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("temp dir should still exist after Close")
	}
}

func TestLifecycleRepository_IDGeneration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	// Test auto-generated IDs
	workflow := contextports.WorkflowRecord{} // No WorkflowID
	err := repo.CreateWorkflow(workflow)
	if err != nil {
		t.Fatalf("CreateWorkflow with auto ID failed: %v", err)
	}

	workflows, _ := repo.ListWorkflows("")
	if len(workflows) != 1 {
		t.Errorf("expected 1 workflow, got %d", len(workflows))
	}
	if workflows[0].WorkflowID == "" {
		t.Error("expected auto-generated WorkflowID to be non-empty")
	}
}

func TestLifecycleRepository_RoundTrip(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	// Create workflow
	workflow := contextports.WorkflowRecord{
		WorkflowID: "wf-roundtrip",
		Metadata:   map[string]any{"key1": "value1", "key2": 123},
	}
	_ = repo.CreateWorkflow(workflow)

	// Create run
	run := contextports.WorkflowRunRecord{
		RunID:      "run-roundtrip",
		WorkflowID: "wf-roundtrip",
		Status:     "running",
		Metadata:   map[string]any{"run_key": "run_value"},
	}
	_ = repo.CreateRun(run)

	// Create delegation
	delegation := contextports.DelegationEntry{
		DelegationID:     "del-roundtrip",
		WorkflowID:       "wf-roundtrip",
		RunID:            "run-roundtrip",
		State:            "active",
		TargetProviderID: "provider-1",
		Metadata:         map[string]any{"del_key": "del_value"},
	}
	_ = repo.UpsertDelegation(delegation)

	// Verify round-trip
	retrievedWorkflow, _ := repo.GetWorkflow("wf-roundtrip")
	if retrievedWorkflow.Metadata["key1"] != "value1" {
		t.Errorf("metadata round-trip failed: expected key1=value1, got %v", retrievedWorkflow.Metadata["key1"])
	}

	retrievedRun, _ := repo.GetRun("run-roundtrip")
	if retrievedRun.Status != "running" {
		t.Errorf("run round-trip failed: expected status running, got %s", retrievedRun.Status)
	}

	retrievedDelegation, _ := repo.GetDelegation("del-roundtrip")
	if retrievedDelegation.TargetProviderID != "provider-1" {
		t.Errorf("delegation round-trip failed: expected TargetProviderID provider-1, got %s", retrievedDelegation.TargetProviderID)
	}
}

func TestGraphdbIDGeneration(t *testing.T) {
	id1 := graphdb.GenerateID("test")
	id2 := graphdb.GenerateID("test")

	if id1 == id2 {
		t.Error("GenerateID should produce unique IDs")
	}
	if len(id1) == 0 {
		t.Error("GenerateID should produce non-empty ID")
	}
}

func TestGraphdbSequenceIDGeneration(t *testing.T) {
	id1 := graphdb.GenerateSequenceID("evt", 1)
	id2 := graphdb.GenerateSequenceID("evt", 2)

	if id1 == id2 {
		t.Error("GenerateSequenceID should produce different IDs for different sequences")
	}
	if id1 != "evt_0000000001" {
		t.Errorf("expected evt_0000000001, got %s", id1)
	}
	if id2 != "evt_0000000002" {
		t.Errorf("expected evt_0000000002, got %s", id2)
	}
}

func TestLifecycleRepository_EventLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close(context.Background())
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: "wf-1"})

	// Append 5 events
	for i := 0; i < 5; i++ {
		_ = repo.AppendEvent(contextports.WorkflowEventRecord{
			EventID:    graphdb.GenerateSequenceID("evt", uint64(i)),
			WorkflowID: "wf-1",
			EventType:  "test",
			Sequence:   int64(i),
		})
	}

	// List with limit 3
	events, err := repo.ListEvents("wf-1", 3)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events with limit, got %d", len(events))
	}

	// List without limit
	allEvents, err := repo.ListEvents("wf-1", 0)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(allEvents) != 5 {
		t.Errorf("expected 5 events without limit, got %d", len(allEvents))
	}
}

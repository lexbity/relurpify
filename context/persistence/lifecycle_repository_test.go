package persistence

import (
	"context"
	"os"
	"testing"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

const (
	Art1_lifecycle_repository_test         = "art-1"
	Attempt456_lifecycle_repository_test   = "attempt-456"
	Completed_lifecycle_repository_test    = "completed"
	Del1_lifecycle_repository_test         = "del-1"
	Evt_lifecycle_repository_test          = "evt"
	Key1_lifecycle_repository_test         = "key1"
	Lb1_lifecycle_repository_test          = "lb-1"
	Lineage123_lifecycle_repository_test   = "lineage-123"
	Provider1_lifecycle_repository_test    = "provider-1"
	Run1_lifecycle_repository_test         = "run-1"
	Runroundtrip_lifecycle_repository_test = "run-roundtrip"
	Running_lifecycle_repository_test      = "running"
	Test_lifecycle_repository_test         = "test"
	Wf1_lifecycle_repository_test          = "wf-1"
	Wfroundtrip_lifecycle_repository_test  = "wf-roundtrip"
	Wftest1_lifecycle_repository_test      = "wf-test-1"
	Wftest2_lifecycle_repository_test      = "wf-test-2"
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
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	workflow := contextports.WorkflowRecord{
		WorkflowID: Wftest1_lifecycle_repository_test,
		Metadata:   map[string]any{"key": "value"},
	}

	err := repo.CreateWorkflow(workflow)
	if err != nil {
		t.Fatalf("CreateWorkflow failed: %v", err)
	}

	// Verify it was created
	retrieved, err := repo.GetWorkflow(Wftest1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if retrieved.WorkflowID != Wftest1_lifecycle_repository_test {
		t.Errorf("expected WorkflowID wf-test-1, got %s", retrieved.WorkflowID)
	}
}

func TestLifecycleRepository_GetWorkflow(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	workflow := contextports.WorkflowRecord{
		WorkflowID: Wftest2_lifecycle_repository_test,
		Metadata:   map[string]any{"key": "value"},
	}

	_ = repo.CreateWorkflow(workflow)

	retrieved, err := repo.GetWorkflow(Wftest2_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("GetWorkflow failed: %v", err)
	}
	if retrieved.WorkflowID != Wftest2_lifecycle_repository_test {
		t.Errorf("expected WorkflowID wf-test-2, got %s", retrieved.WorkflowID)
	}
}

func TestLifecycleRepository_ListWorkflows(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
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
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	// First create a workflow
	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})

	run := contextports.WorkflowRunRecord{
		RunID:      Run1_lifecycle_repository_test,
		WorkflowID: Wf1_lifecycle_repository_test,
		Status:     Running_lifecycle_repository_test,
	}

	err := repo.CreateRun(run)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Verify it was created
	retrieved, err := repo.GetRun(Run1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if retrieved.RunID != Run1_lifecycle_repository_test {
		t.Errorf("expected RunID run-1, got %s", retrieved.RunID)
	}
}

func TestLifecycleRepository_ListRuns(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: "run-2", WorkflowID: Wf1_lifecycle_repository_test})

	runs, err := repo.ListRuns(Wf1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runs))
	}
}

func TestLifecycleRepository_UpdateRunStatus(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test, Status: Running_lifecycle_repository_test})

	err := repo.UpdateRunStatus(Run1_lifecycle_repository_test, Completed_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("UpdateRunStatus failed: %v", err)
	}

	retrieved, _ := repo.GetRun(Run1_lifecycle_repository_test)
	if retrieved.Status != Completed_lifecycle_repository_test {
		t.Errorf("expected status completed, got %s", retrieved.Status)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestLifecycleRepository_UpsertDelegation(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	delegation := contextports.DelegationEntry{
		DelegationID:     Del1_lifecycle_repository_test,
		WorkflowID:       Wf1_lifecycle_repository_test,
		RunID:            Run1_lifecycle_repository_test,
		State:            "active",
		TargetProviderID: Provider1_lifecycle_repository_test,
	}

	err := repo.UpsertDelegation(delegation)
	if err != nil {
		t.Fatalf("UpsertDelegation failed: %v", err)
	}

	retrieved, err := repo.GetDelegation(Del1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("GetDelegation failed: %v", err)
	}
	if retrieved.DelegationID != Del1_lifecycle_repository_test {
		t.Errorf("expected DelegationID del-1, got %s", retrieved.DelegationID)
	}
}

func TestLifecycleRepository_ListDelegations(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	_ = repo.UpsertDelegation(contextports.DelegationEntry{DelegationID: Del1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test, RunID: Run1_lifecycle_repository_test})
	_ = repo.UpsertDelegation(contextports.DelegationEntry{DelegationID: "del-2", WorkflowID: Wf1_lifecycle_repository_test, RunID: Run1_lifecycle_repository_test})

	delegations, err := repo.ListDelegations(Wf1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("ListDelegations failed: %v", err)
	}
	if len(delegations) != 2 {
		t.Errorf("expected 2 delegations, got %d", len(delegations))
	}
}

func TestLifecycleRepository_AppendDelegationTransition(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.UpsertDelegation(contextports.DelegationEntry{DelegationID: Del1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	transition := contextports.DelegationTransitionEntry{
		DelegationID: Del1_lifecycle_repository_test,
		ToState:      Completed_lifecycle_repository_test,
	}

	err := repo.AppendDelegationTransition(transition)
	if err != nil {
		t.Fatalf("AppendDelegationTransition failed: %v", err)
	}

	transitions, err := repo.ListDelegationTransitions(Del1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("ListDelegationTransitions failed: %v", err)
	}
	if len(transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(transitions))
	}
}

func TestLifecycleRepository_AppendEvent(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	event := contextports.WorkflowEventRecord{
		EventID:    "evt-1",
		WorkflowID: Wf1_lifecycle_repository_test,
		RunID:      Run1_lifecycle_repository_test,
		EventType:  "test_event",
		Sequence:   1,
		Payload:    map[string]any{"msg": Test_lifecycle_repository_test},
	}

	err := repo.AppendEvent(event)
	if err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}

	events, err := repo.ListEventsByRun(Run1_lifecycle_repository_test, 10)
	if err != nil {
		t.Fatalf("ListEventsByRun failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestLifecycleRepository_UpsertArtifact(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	artifact := contextports.WorkflowArtifactRecord{
		ArtifactID:  Art1_lifecycle_repository_test,
		WorkflowID:  Wf1_lifecycle_repository_test,
		RunID:       Run1_lifecycle_repository_test,
		StorageKind: "inline",
		ContentType: "text/plain",
	}

	err := repo.UpsertArtifact(artifact)
	if err != nil {
		t.Fatalf("UpsertArtifact failed: %v", err)
	}

	retrieved, err := repo.GetArtifact(Art1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("GetArtifact failed: %v", err)
	}
	if retrieved.ArtifactID != Art1_lifecycle_repository_test {
		t.Errorf("expected ArtifactID art-1, got %s", retrieved.ArtifactID)
	}
}

func TestLifecycleRepository_ListArtifacts(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	_ = repo.UpsertArtifact(contextports.WorkflowArtifactRecord{ArtifactID: Art1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test, RunID: Run1_lifecycle_repository_test})
	_ = repo.UpsertArtifact(contextports.WorkflowArtifactRecord{ArtifactID: "art-2", WorkflowID: Wf1_lifecycle_repository_test, RunID: Run1_lifecycle_repository_test})

	artifacts, err := repo.ListArtifactsByRun(Run1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("ListArtifactsByRun failed: %v", err)
	}
	if len(artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(artifacts))
	}
}

func TestLifecycleRepository_UpsertLineageBinding(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	binding := contextports.LineageBindingRecord{
		BindingID:    Lb1_lifecycle_repository_test,
		WorkflowID:   Wf1_lifecycle_repository_test,
		FromRunID:    Run1_lifecycle_repository_test,
		FromEntityID: "lineage-1",
		ToEntityID:   "attempt-1",
	}

	err := repo.UpsertLineageBinding(binding)
	if err != nil {
		t.Fatalf("UpsertLineageBinding failed: %v", err)
	}

	retrieved, err := repo.GetLineageBinding(Lb1_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("GetLineageBinding failed: %v", err)
	}
	if retrieved.BindingID != Lb1_lifecycle_repository_test {
		t.Errorf("expected BindingID lb-1, got %s", retrieved.BindingID)
	}
}

func TestLifecycleRepository_FindLineageBindingByLineageID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	binding := contextports.LineageBindingRecord{
		BindingID:    Lb1_lifecycle_repository_test,
		WorkflowID:   Wf1_lifecycle_repository_test,
		FromRunID:    Run1_lifecycle_repository_test,
		FromEntityID: Lineage123_lifecycle_repository_test,
		ToEntityID:   "attempt-1",
	}

	_ = repo.UpsertLineageBinding(binding)

	bindings, err := repo.FindLineageBindingsByFrom(Lineage123_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("FindLineageBindingsByFrom failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].FromEntityID != Lineage123_lifecycle_repository_test {
		t.Errorf("expected FromEntityID lineage-123, got %s", bindings[0].FromEntityID)
	}
}

func TestLifecycleRepository_FindLineageBindingByAttemptID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})
	_ = repo.CreateRun(contextports.WorkflowRunRecord{RunID: Run1_lifecycle_repository_test, WorkflowID: Wf1_lifecycle_repository_test})

	binding := contextports.LineageBindingRecord{
		BindingID:    Lb1_lifecycle_repository_test,
		WorkflowID:   Wf1_lifecycle_repository_test,
		FromRunID:    Run1_lifecycle_repository_test,
		FromEntityID: "lineage-1",
		ToEntityID:   Attempt456_lifecycle_repository_test,
	}

	_ = repo.UpsertLineageBinding(binding)

	bindings, err := repo.FindLineageBindingsByTo(Attempt456_lifecycle_repository_test)
	if err != nil {
		t.Fatalf("FindLineageBindingsByTo failed: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].ToEntityID != Attempt456_lifecycle_repository_test {
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
	defer func() { _ = db.Close(context.Background()) }()
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
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	// Create workflow
	workflow := contextports.WorkflowRecord{
		WorkflowID: Wfroundtrip_lifecycle_repository_test,
		Metadata:   map[string]any{Key1_lifecycle_repository_test: "value1", "key2": 123},
	}
	_ = repo.CreateWorkflow(workflow)

	// Create run
	run := contextports.WorkflowRunRecord{
		RunID:      Runroundtrip_lifecycle_repository_test,
		WorkflowID: Wfroundtrip_lifecycle_repository_test,
		Status:     Running_lifecycle_repository_test,
		Metadata:   map[string]any{"run_key": "run_value"},
	}
	_ = repo.CreateRun(run)

	// Create delegation
	delegation := contextports.DelegationEntry{
		DelegationID:     "del-roundtrip",
		WorkflowID:       Wfroundtrip_lifecycle_repository_test,
		RunID:            Runroundtrip_lifecycle_repository_test,
		State:            "active",
		TargetProviderID: Provider1_lifecycle_repository_test,
		Metadata:         map[string]any{"del_key": "del_value"},
	}
	_ = repo.UpsertDelegation(delegation)

	// Verify round-trip
	retrievedWorkflow, _ := repo.GetWorkflow(Wfroundtrip_lifecycle_repository_test)
	if retrievedWorkflow.Metadata[Key1_lifecycle_repository_test] != "value1" {
		t.Errorf("metadata round-trip failed: expected key1=value1, got %v", retrievedWorkflow.Metadata[Key1_lifecycle_repository_test])
	}

	retrievedRun, _ := repo.GetRun(Runroundtrip_lifecycle_repository_test)
	if retrievedRun.Status != Running_lifecycle_repository_test {
		t.Errorf("run round-trip failed: expected status running, got %s", retrievedRun.Status)
	}

	retrievedDelegation, _ := repo.GetDelegation("del-roundtrip")
	if retrievedDelegation.TargetProviderID != Provider1_lifecycle_repository_test {
		t.Errorf("delegation round-trip failed: expected TargetProviderID provider-1, got %s", retrievedDelegation.TargetProviderID)
	}
}

func TestGraphdbIDGeneration(t *testing.T) {
	id1 := graphdb.GenerateID(Test_lifecycle_repository_test)
	id2 := graphdb.GenerateID(Test_lifecycle_repository_test)

	if id1 == id2 {
		t.Error("GenerateID should produce unique IDs")
	}
	if len(id1) == 0 {
		t.Error("GenerateID should produce non-empty ID")
	}
}

func TestGraphdbSequenceIDGeneration(t *testing.T) {
	id1 := graphdb.GenerateSequenceID(Evt_lifecycle_repository_test, 1)
	id2 := graphdb.GenerateSequenceID(Evt_lifecycle_repository_test, 2)

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
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_ = repo.CreateWorkflow(contextports.WorkflowRecord{WorkflowID: Wf1_lifecycle_repository_test})

	// Append 5 events
	for i := 0; i < 5; i++ {
		_ = repo.AppendEvent(contextports.WorkflowEventRecord{
			EventID:    graphdb.GenerateSequenceID(Evt_lifecycle_repository_test, uint64(i)),
			WorkflowID: Wf1_lifecycle_repository_test,
			EventType:  Test_lifecycle_repository_test,
			Sequence:   int64(i),
		})
	}

	// List with limit 3
	events, err := repo.ListEvents(Wf1_lifecycle_repository_test, 3)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events with limit, got %d", len(events))
	}

	// List without limit
	allEvents, err := repo.ListEvents(Wf1_lifecycle_repository_test, 0)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(allEvents) != 5 {
		t.Errorf("expected 5 events without limit, got %d", len(allEvents))
	}
}

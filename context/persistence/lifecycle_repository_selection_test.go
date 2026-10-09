package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
)

const (
	Sel1_lifecycle_repository_selection_test         = "sel_wf-selection-1_0000000001"
	Selectionwf_lifecycle_repository_selection_test  = "wf-selection-1"
	Selectionwf2_lifecycle_repository_selection_test = "wf-selection-2"
	Selectionrun_lifecycle_repository_selection_test = "run-selection-1"
)

func newSelectionRecord(workflowID, runID string) contextports.SelectionDecisionRecord {
	return contextports.SelectionDecisionRecord{
		Schema:          contextports.SelectionDecisionSchema,
		WorkflowID:      workflowID,
		RunID:           runID,
		UtteranceDigest: "sha256:abc",
		TokenCount:      5,
		Family:          "query",
		Candidates: []contextports.SelectionDecisionCandidate{
			{RouteID: "euclo:cap.ast_query", Kind: "capability", Availability: "available", Score: 25, Components: map[string]int{"keyword": 25}},
		},
		Selected:       contextports.SelectionDecisionSelected{RouteID: "euclo:cap.ast_query", Kind: "capability"},
		DecidedBy:      "lattice:score",
		FallbackTaken:  false,
		ExecutionState: contextports.SelectionExecutionStateDispatched,
		CreatedAt:      time.Now().UTC(),
	}
}

func TestRecordSelectionDecision_WorkflowScopedID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	id, err := repo.RecordSelectionDecision(context.Background(), newSelectionRecord(Selectionwf_lifecycle_repository_selection_test, Selectionrun_lifecycle_repository_selection_test))
	if err != nil {
		t.Fatalf("RecordSelectionDecision failed: %v", err)
	}
	expectedPrefix := "sel_" + Selectionwf_lifecycle_repository_selection_test + "_"
	if !strings.HasPrefix(id, expectedPrefix) {
		t.Fatalf("decision id %q does not carry the workflow-scoped prefix %q", id, expectedPrefix)
	}

	// Two workflows must never collide: the same first decision in a different
	// workflow scopes its sequence separately.
	id2, err := repo.RecordSelectionDecision(context.Background(), newSelectionRecord(Selectionwf2_lifecycle_repository_selection_test, "run-selection-2"))
	if err != nil {
		t.Fatalf("second RecordSelectionDecision failed: %v", err)
	}
	if id == id2 {
		t.Fatalf("records for distinct workflows collided on %q", id)
	}
}

// The second record of the same workflow gets the next sequence.
func TestRecordSelectionDecision_MonotonicSequencePerWorkflow(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	first, err := repo.RecordSelectionDecision(context.Background(), newSelectionRecord(Selectionwf_lifecycle_repository_selection_test, ""))
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	second, err := repo.RecordSelectionDecision(context.Background(), newSelectionRecord(Selectionwf_lifecycle_repository_selection_test, ""))
	if err != nil {
		t.Fatalf("second record: %v", err)
	}
	if first == second {
		t.Fatalf("records of the same workflow must not share an id: %q", first)
	}
	if !(first < second) {
		t.Fatalf("expected monotonic sequence order %q < %q", second, first)
	}
}

func TestRecordSelectionDecision_NoRawUtteranceAndLinks(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	// The record struct has no utterance field by construction; prove the
	// stored properties are the redacted set.
	record := newSelectionRecord(Selectionwf_lifecycle_repository_selection_test, Selectionrun_lifecycle_repository_selection_test)
	id, err := repo.RecordSelectionDecision(context.Background(), record)
	if err != nil {
		t.Fatalf("RecordSelectionDecision failed: %v", err)
	}

	// Round-trip
	records, err := repo.ListSelectionDecisions(Selectionwf_lifecycle_repository_selection_test, 0)
	if err != nil {
		t.Fatalf("ListSelectionDecisions failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	got := records[0]
	if got.DecisionID != id {
		t.Fatalf("decoded id %q != %q", got.DecisionID, id)
	}
	if got.UtteranceDigest != "sha256:abc" {
		t.Fatalf("digest mismatch: %q", got.UtteranceDigest)
	}
	if got.WorkflowID != Selectionwf_lifecycle_repository_selection_test || got.RunID != Selectionrun_lifecycle_repository_selection_test {
		t.Fatalf("link fields lost: workflow=%q run=%q", got.WorkflowID, got.RunID)
	}

	// Workflow + run linkage must exist as typed edges.
	wfEdges := db.GetOutEdges(Selectionwf_lifecycle_repository_selection_test, graphdb.EdgeKindWorkflowHasSelectionDecision)
	if len(wfEdges) != 1 || wfEdges[0].TargetID != id {
		t.Fatalf("expected workflow→decision edge to %q, got %#v", id, wfEdges)
	}
	runEdges := db.GetOutEdges(Selectionrun_lifecycle_repository_selection_test, graphdb.EdgeKindRunHasSelectionDecision)
	if len(runEdges) != 1 || runEdges[0].TargetID != id {
		t.Fatalf("expected run→decision edge to %q, got %#v", id, runEdges)
	}
}

func TestUpdateSelectionExecutionState_Transitions(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	id, err := repo.RecordSelectionDecision(context.Background(), newSelectionRecord(Selectionwf_lifecycle_repository_selection_test, ""))
	if err != nil {
		t.Fatalf("RecordSelectionDecision failed: %v", err)
	}

	if err := repo.UpdateSelectionExecutionState(context.Background(), id, contextports.SelectionExecutionStateCompleted); err != nil {
		t.Fatalf("UpdateSelectionExecutionState failed: %v", err)
	}
	records, err := repo.ListSelectionDecisions(Selectionwf_lifecycle_repository_selection_test, 0)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].ExecutionState != contextports.SelectionExecutionStateCompleted {
		t.Fatalf("expected completed, got %q", records[0].ExecutionState)
	}

	if err := repo.UpdateSelectionExecutionState(context.Background(), id, contextports.SelectionExecutionStateFailed); err != nil {
		t.Fatalf("second update failed: %v", err)
	}
	records, _ = repo.ListSelectionDecisions(Selectionwf_lifecycle_repository_selection_test, 0)
	if records[0].ExecutionState != contextports.SelectionExecutionStateFailed {
		t.Fatalf("expected failed, got %q", records[0].ExecutionState)
	}

	// An unknown id is an error — a crash between record and completion still
	// leaves the "dispatched" record readable (D11 truthfulness).
	if err := repo.UpdateSelectionExecutionState(context.Background(), "sel_@nope", "completed"); err == nil {
		t.Fatal("expected error for unknown decision id")
	}
}

func TestListSelectionDecisions_EmptyWorkflowListsAll(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close(context.Background()) }()
	repo := NewLifecycleRepository(db)

	_, _ = repo.RecordSelectionDecision(context.Background(), newSelectionRecord(Selectionwf_lifecycle_repository_selection_test, ""))
	_, _ = repo.RecordSelectionDecision(context.Background(), newSelectionRecord(Selectionwf2_lifecycle_repository_selection_test, ""))

	all, err := repo.ListSelectionDecisions("", 0)
	if err != nil {
		t.Fatalf("ListSelectionDecisions('') failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d", len(all))
	}
	// Limit honors caps.
	limited, err := repo.ListSelectionDecisions("", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 limited record, got %d", len(limited))
	}
}

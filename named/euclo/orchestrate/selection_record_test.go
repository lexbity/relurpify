package orchestrate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/persistence"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	Selwf_selection_record_test  = "wf-selection-test"
	Selrun_selection_record_test = "run-selection-test"
)

// brokenLifecycleRepository is a minimal production-shaped broken store: every
// persistence call fails so selection must proceed without a record.
type brokenLifecycleRepository struct {
	contextports.LifecycleRepository
	failRecords bool
}

func (b *brokenLifecycleRepository) RecordSelectionDecision(_ context.Context, _ contextports.SelectionDecisionRecord) (string, error) {
	return "", errBrokenStore
}

var errBrokenStore = &selectionStoreError{"store is down"}

type selectionStoreError struct{ msg string }

func (e *selectionStoreError) Error() string { return e.msg }

// openTestLifecycle opens a temp graphdb-backed lifecycle repository.
func openTestLifecycle(t *testing.T) (*persistence.LifecycleRepository, *graphdb.Engine) {
	t.Helper()
	db, err := graphdb.Open(context.Background(), graphdb.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return persistence.NewLifecycleRepository(db), db
}

func TestBuildSelectionRecord_RedactionAndDigest(t *testing.T) {
	env := contextdata.NewEnvelope(Selwf_selection_record_test, Selrun_selection_record_test)
	req := RouteRequest{FamilyID: "query", Instruction: "  FIX  the debugger crash   "}
	report := &DryRunReport{
		Request:   req,
		DecidedBy: decidedByScore,
		Candidates: []CandidateRouteInfo{
			{
				RouteID:         "euclo.thoughtrecipe.debug_tdd_repair",
				RouteKind:       "thoughtrecipe",
				Availability:    RouteAvailable,
				RankScore:       55,
				Components:      map[string]int{compFamilyAffinity: 50, compKeyword: 5},
				MatchedKeywords: []string{"fix", "diagnose"},
			},
		},
		Tier2: euclotypes.Tier2Info{},
	}
	selected := report.Candidates[0]

	record := buildSelectionRecord(env, req, report, selected, false)

	if record.Schema != contextports.SelectionDecisionSchema {
		t.Fatalf("schema = %q, want %q", record.Schema, contextports.SelectionDecisionSchema)
	}
	if record.WorkflowID != Selwf_selection_record_test || record.RunID != Selrun_selection_record_test {
		t.Fatalf("link fields: workflow=%q run=%q", record.WorkflowID, record.RunID)
	}
	if !strings.HasPrefix(record.UtteranceDigest, "sha256:") {
		t.Fatalf("digest not prefixed: %q", record.UtteranceDigest)
	}
	// The digest must be a digest, not the utterance.
	if strings.Contains(strings.ToLower(record.UtteranceDigest), "debugger") {
		t.Fatalf("digest leaks utterance text: %q", record.UtteranceDigest)
	}
	if record.TokenCount == 0 {
		t.Fatal("token_count must be derived from the utterance vocabulary")
	}
	if record.Family != "query" {
		t.Fatalf("family = %q", record.Family)
	}
	if len(record.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(record.Candidates))
	}
	candidate := record.Candidates[0]
	if candidate.Score != 55 || candidate.Components["family_affinity"] != 50 {
		t.Fatalf("component split lost: %#v", candidate)
	}
	if record.Selected.RouteID != "euclo.thoughtrecipe.debug_tdd_repair" {
		t.Fatalf("selected = %#v", record.Selected)
	}
	if record.ExecutionState != contextports.SelectionExecutionStateDispatched {
		t.Fatalf("initial execution_state = %q, want dispatched", record.ExecutionState)
	}

	// Redaction is structural: the serialized record has no raw-utterance field.
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"utterance", "instruction", "text"} {
		if _, ok := asMap[forbidden]; ok {
			t.Fatalf("record leaks a raw-utterance-shaped field %q", forbidden)
		}
	}
}

func TestDispatchPersistsSelectionRecord(t *testing.T) {
	repo, _ := openTestLifecycle(t)
	recorder := NewSelectionRecorder(repo)

	reg := registry.NewRegistry()
	desc := testCapabilityDescriptor("euclo:cap.ast_query", 10, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(context.Background(), desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	env := contextdata.NewEnvelope(Selwf_selection_record_test, Selrun_selection_record_test)
	req := RouteRequest{CapabilityID: desc.ID, Instruction: "trace the regression"}

	result, err := Dispatch(context.Background(), env, req, SelectionDeps{Capabilities: reg, Recorder: recorder})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result == nil || result.RouteID != desc.ID {
		t.Fatalf("unexpected route result: %#v", result)
	}

	decisionID, ok := state.GetSelectionRecordID(env)
	if !ok || decisionID == "" {
		t.Fatalf("selection record id not written to envelope (ok=%v)", ok)
	}
	if !strings.HasPrefix(decisionID, "sel_"+Selwf_selection_record_test+"_") {
		t.Fatalf("record id is not workflow-scoped: %q", decisionID)
	}

	records, err := repo.ListSelectionDecisions(Selwf_selection_record_test, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 persisted record, got %d", len(records))
	}
	record := records[0]
	if record.DecisionID != decisionID {
		t.Fatalf("envelope id %q != persisted id %q", decisionID, record.DecisionID)
	}
	if record.Selected.RouteID != desc.ID {
		t.Fatalf("record selected route = %q, want %q", record.Selected.RouteID, desc.ID)
	}
	if record.ExecutionState != contextports.SelectionExecutionStateDispatched {
		t.Fatalf("post-dispatch execution_state = %q, want dispatched (executor has not run)", record.ExecutionState)
	}
}

func TestDispatchPersistFailureProceedsAndEmits(t *testing.T) {
	recorder := NewSelectionRecorder(&brokenLifecycleRepository{})
	sink := &telemetrySink{}
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	reg := registry.NewRegistry()
	desc := testCapabilityDescriptor("euclo:cap.ast_query", 10, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(context.Background(), desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	env := contextdata.NewEnvelope(Selwf_selection_record_test, Selrun_selection_record_test)
	result, err := Dispatch(ctx, env, RouteRequest{CapabilityID: desc.ID}, SelectionDeps{Capabilities: reg, Recorder: recorder})
	if err != nil {
		t.Fatalf("select must proceed even when record persistence fails: %v", err)
	}
	if result == nil || result.RouteID != desc.ID {
		t.Fatalf("unexpected result: %#v", result)
	}

	found := false
	for _, ev := range sink.snapshot() {
		if ev.Type == "euclo.route.selection_persist_failed" {
			found = true
			class, _ := ev.Metadata["error_class"].(string)
			if class == "" {
				t.Fatalf("persist-failed event missing error_class: %#v", ev.Metadata)
			}
		}
	}
	if !found {
		t.Fatal("expected euclo.route.selection_persist_failed to fire on store failure")
	}
}

func TestSelectionRecorderUpdateExecutionState(t *testing.T) {
	repo, _ := openTestLifecycle(t)
	recorder := NewSelectionRecorder(repo)

	env := contextdata.NewEnvelope(Selwf_selection_record_test, Selrun_selection_record_test)
	record := contextports.SelectionDecisionRecord{
		Schema:     contextports.SelectionDecisionSchema,
		WorkflowID: Selwf_selection_record_test,
		RunID:      Selrun_selection_record_test,
	}
	decisionID, err := repo.RecordSelectionDecision(context.Background(), record)
	if err != nil {
		t.Fatalf("seed record: %v", err)
	}
	state.SetSelectionRecordID(env, decisionID)

	// Completed path.
	recorder.updateExecutionState(context.Background(), env, true)
	records, _ := repo.ListSelectionDecisions(Selwf_selection_record_test, 0)
	if records[0].ExecutionState != contextports.SelectionExecutionStateCompleted {
		t.Fatalf("execution_state = %q, want completed", records[0].ExecutionState)
	}

	// Failed path.
	recorder.updateExecutionState(context.Background(), env, false)
	records, _ = repo.ListSelectionDecisions(Selwf_selection_record_test, 0)
	if records[0].ExecutionState != contextports.SelectionExecutionStateFailed {
		t.Fatalf("execution_state = %q, want failed", records[0].ExecutionState)
	}

	// A dispatch whose record never persisted is a no-op.
	other := contextdata.NewEnvelope("other-task", "other-run")
	state.SetSelectionRecordID(other, "")
	recorder.updateExecutionState(context.Background(), other, false) // must not panic
}

func TestDispatchWithoutRecorderNoRecord(t *testing.T) {
	reg := registry.NewRegistry()
	desc := testCapabilityDescriptor("euclo:cap.ast_query", 10, descriptor.AvailabilitySpec{Available: true})
	if err := reg.RegisterCapability(context.Background(), desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	env := contextdata.NewEnvelope(Selwf_selection_record_test, Selrun_selection_record_test)
	if _, err := Dispatch(context.Background(), env, RouteRequest{CapabilityID: desc.ID}, SelectionDeps{Capabilities: reg}); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if _, ok := state.GetSelectionRecordID(env); ok {
		t.Fatal("no recorder must mean no selection record id on the envelope")
	}
}

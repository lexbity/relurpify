package agenttest

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/persistence"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
)

// TestSelectionRecords is the AC-10 harness gate: every dispatch — through the
// same SelectionDeps/recorder wiring the PreparedRunExecutor composes — leaves
// a schema-valid, workflow-scoped Selection Decision Record with no raw
// utterance field. It runs without a model backend: dispatch precedes
// execution, so the record is written before any LLM is needed.
func TestSelectionRecords(t *testing.T) {
	db, err := graphdb.Open(context.Background(), graphdb.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	defer func() { _ = db.Close(context.Background()) }()
	repo := persistence.NewLifecycleRepository(db)
	recorder := orchestrate.NewSelectionRecorder(repo)

	reg := registry.NewRegistry()
	if err := reg.RegisterCapability(context.Background(), descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.ast_query",
		Name:          "euclo:cap.ast_query",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	workflowID := "wf-agenttest-ac10"
	for i, runID := range []string{"run-ac10-1", "run-ac10-2", "run-ac10-3"} {
		env := contextdata.NewEnvelope(workflowID, runID)
		req := orchestrate.RouteRequest{CapabilityID: "euclo:cap.ast_query", Instruction: "map the codebase traces"}
		result, err := orchestrate.Dispatch(context.Background(), env, req,
			orchestrate.SelectionDeps{Capabilities: reg, Recorder: recorder})
		if err != nil {
			t.Fatalf("dispatch %d failed: %v", i, err)
		}
		if result == nil || result.RouteID != "euclo:cap.ast_query" {
			t.Fatalf("dispatch %d routed to %#v", i, result)
		}
	}

	records, err := repo.ListSelectionDecisions(workflowID, 0)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected one record per dispatch (3), got %d", len(records))
	}

	for i, record := range records {
		if record.Schema != contextports.SelectionDecisionSchema {
			t.Errorf("record %d schema = %q, want %q", i, record.Schema, contextports.SelectionDecisionSchema)
		}
		if !strings.HasPrefix(record.DecisionID, "sel_"+workflowID+"_") {
			t.Errorf("record %d id %q is not workflow-scoped", i, record.DecisionID)
		}
		if record.WorkflowID != workflowID {
			t.Errorf("record %d workflow = %q", i, record.WorkflowID)
		}
		if record.UtteranceDigest == "" || !strings.HasPrefix(record.UtteranceDigest, "sha256:") {
			t.Errorf("record %d missing digest: %q", i, record.UtteranceDigest)
		}
		if record.Selected.RouteID != "euclo:cap.ast_query" {
			t.Errorf("record %d selected = %q", i, record.Selected.RouteID)
		}
		if record.ExecutionState != contextports.SelectionExecutionStateDispatched {
			t.Errorf("record %d execution_state = %q, want dispatched (no executor ran)", i, record.ExecutionState)
		}
	}
}

// TestSelectionRecordsSchemaRedaction locks the "no raw utterance field"
// surface: the persisted record set never contains user instruction text.
func TestSelectionRecordsSchemaRedaction(t *testing.T) {
	db, err := graphdb.Open(context.Background(), graphdb.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	defer func() { _ = db.Close(context.Background()) }()
	repo := persistence.NewLifecycleRepository(db)
	recorder := orchestrate.NewSelectionRecorder(repo)

	reg := registry.NewRegistry()
	if err := reg.RegisterCapability(context.Background(), descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.ast_query",
		Name:          "euclo:cap.ast_query",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	privatePhrase := "supersecret-utterance-that-must-never-persist"
	env := contextdata.NewEnvelope("wf-ac10-redaction", "run-ac10-redaction")
	if _, err := orchestrate.Dispatch(context.Background(), env, orchestrate.RouteRequest{
		CapabilityID: "euclo:cap.ast_query",
		Instruction:  "locate " + privatePhrase,
	}, orchestrate.SelectionDeps{Capabilities: reg, Recorder: recorder}); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	records, err := repo.ListSelectionDecisions("wf-ac10-redaction", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if strings.Contains(records[0].UtteranceDigest, "supersecret") {
		t.Fatalf("digest leaks raw utterance text: %q", records[0].UtteranceDigest)
	}
}

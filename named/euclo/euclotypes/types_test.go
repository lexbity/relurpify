package euclotypes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestCollectArtifactsFromState_IncludesWaiverArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.waiver", eucloruntime.ExecutionWaiver{
		WaiverID:  "waiver-1",
		Kind:      eucloruntime.WaiverKindVerification,
		Reason:    "operator approved degraded verification",
		GrantedBy: "operator",
	})

	artifacts := euclotypes.CollectArtifactsFromState(state)
	for _, artifact := range artifacts {
		if artifact.Kind == euclotypes.ArtifactKindWaiver {
			return
		}
	}
	t.Fatalf("expected waiver artifact in %#v", artifacts)
}

func TestCollectArtifactsFromState_IncludesVerificationPlanArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.verification_plan", map[string]any{
		"scope_kind": "explicit",
		"source":     "task_context",
	})

	artifacts := euclotypes.CollectArtifactsFromState(state)
	for _, artifact := range artifacts {
		if artifact.Kind == euclotypes.ArtifactKindVerificationPlan {
			return
		}
	}
	t.Fatalf("expected verification plan artifact in %#v", artifacts)
}

func TestCollectArtifactsFromState_IncludesTDDLifecycleArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.tdd.lifecycle", map[string]any{
		"current_phase": "green",
		"status":        "completed",
	})

	artifacts := euclotypes.CollectArtifactsFromState(state)
	for _, artifact := range artifacts {
		if artifact.Kind == euclotypes.ArtifactKindTDDLifecycle {
			return
		}
	}
	t.Fatalf("expected TDD lifecycle artifact in %#v", artifacts)
}

func TestAssembleFinalReport_IncludesWaiverAndAssuranceClass(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{
			ID:      "verification_plan",
			Kind:    euclotypes.ArtifactKindVerificationPlan,
			Summary: "verification scope selected",
			Payload: map[string]any{
				"scope_kind": "package_tests",
				"source":     "heuristic_go",
			},
		},
		{
			ID:      "tdd_lifecycle",
			Kind:    euclotypes.ArtifactKindTDDLifecycle,
			Summary: "TDD lifecycle complete",
			Payload: map[string]any{
				"current_phase": "complete",
				"status":        "completed",
				"phase_history": []map[string]any{
					{"phase": "red", "status": "completed"},
					{"phase": "green", "status": "completed"},
				},
			},
		},
		{
			ID:      "success_gate",
			Kind:    euclotypes.ArtifactKindSuccessGate,
			Summary: "completion gate evaluated",
			Payload: map[string]any{
				"allowed":            true,
				"reason":             "manual_verification_allowed",
				"assurance_class":    "operator_deferred",
				"degradation_mode":   "operator_waiver",
				"degradation_reason": "operator_waiver",
			},
		},
		{
			ID:      "waiver",
			Kind:    euclotypes.ArtifactKindWaiver,
			Summary: "operator waiver",
			Payload: map[string]any{
				"waiver_id":  "waiver-1",
				"kind":       "verification",
				"granted_by": "operator",
				"reason":     "continue without executable verification",
			},
		},
	}

	report := euclotypes.AssembleFinalReport(artifacts)
	if report["assurance_class"] != "operator_deferred" {
		t.Fatalf("expected assurance class in report, got %#v", report["assurance_class"])
	}
	if report["degradation_mode"] != "operator_waiver" {
		t.Fatalf("expected degradation mode in report, got %#v", report["degradation_mode"])
	}
	if report["degradation_reason"] != "operator_waiver" {
		t.Fatalf("expected degradation reason in report, got %#v", report["degradation_reason"])
	}
	if _, ok := report["verification_plan"].(map[string]any); !ok {
		t.Fatalf("expected verification plan payload in report, got %#v", report["verification_plan"])
	}
	if _, ok := report["tdd_lifecycle"].(map[string]any); !ok {
		t.Fatalf("expected TDD lifecycle payload in report, got %#v", report["tdd_lifecycle"])
	}
	if _, ok := report["waiver"].(map[string]any); !ok {
		t.Fatalf("expected waiver payload in report, got %#v", report["waiver"])
	}
}

// ---------------------------------------------------------------------------
// PersistWorkflowArtifacts
// ---------------------------------------------------------------------------

type stubArtifactStore struct {
	records []memory.WorkflowArtifactRecord
	err     error
}

func (s *stubArtifactStore) UpsertWorkflowArtifact(_ context.Context, r memory.WorkflowArtifactRecord) error {
	if s.err != nil {
		return s.err
	}
	s.records = append(s.records, r)
	return nil
}

func (s *stubArtifactStore) ListWorkflowArtifacts(_ context.Context, workflowID, _ string) ([]memory.WorkflowArtifactRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	var out []memory.WorkflowArtifactRecord
	for _, r := range s.records {
		if r.WorkflowID == workflowID {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestPersistWorkflowArtifacts_NilStoreReturnsNil(t *testing.T) {
	err := euclotypes.PersistWorkflowArtifacts(context.Background(), nil, "wf-1", "run-1", []euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"step": 1}},
	})
	if err != nil {
		t.Fatalf("expected nil error for nil store, got %v", err)
	}
}

func TestPersistWorkflowArtifacts_EmptyWorkflowIDReturnsNil(t *testing.T) {
	store := &stubArtifactStore{}
	err := euclotypes.PersistWorkflowArtifacts(context.Background(), store, "  ", "run-1", []euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindPlan},
	})
	if err != nil {
		t.Fatalf("expected nil error for empty workflow ID, got %v", err)
	}
	if len(store.records) != 0 {
		t.Fatal("expected no records written for empty workflow ID")
	}
}

func TestPersistWorkflowArtifacts_EmptyArtifactsReturnsNil(t *testing.T) {
	store := &stubArtifactStore{}
	err := euclotypes.PersistWorkflowArtifacts(context.Background(), store, "wf-1", "run-1", nil)
	if err != nil {
		t.Fatalf("expected nil error for empty artifacts, got %v", err)
	}
}

func TestPersistWorkflowArtifacts_WritesRecords(t *testing.T) {
	store := &stubArtifactStore{}
	artifacts := []euclotypes.Artifact{
		{
			ID:         "plan-1",
			Kind:       euclotypes.ArtifactKindPlan,
			Summary:    "a plan",
			ProducerID: "euclo:chat.implement",
			Status:     "produced",
			Payload:    map[string]any{"steps": []string{"step1"}},
		},
	}
	err := euclotypes.PersistWorkflowArtifacts(context.Background(), store, "wf-persist", "run-1", artifacts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(store.records))
	}
	r := store.records[0]
	if r.ArtifactID != "plan-1" {
		t.Fatalf("unexpected ArtifactID: %q", r.ArtifactID)
	}
	if r.WorkflowID != "wf-persist" {
		t.Fatalf("unexpected WorkflowID: %q", r.WorkflowID)
	}
	if r.Kind != string(euclotypes.ArtifactKindPlan) {
		t.Fatalf("unexpected Kind: %q", r.Kind)
	}
	if r.SummaryMetadata["producer_id"] != "euclo:chat.implement" {
		t.Fatalf("unexpected producer_id in metadata: %v", r.SummaryMetadata["producer_id"])
	}
	if r.SummaryMetadata["status"] != "produced" {
		t.Fatalf("unexpected status in metadata: %v", r.SummaryMetadata["status"])
	}
}

func TestPersistWorkflowArtifacts_StoreErrorPropagates(t *testing.T) {
	store := &stubArtifactStore{err: errors.New("disk full")}
	artifacts := []euclotypes.Artifact{
		{ID: "x", Kind: euclotypes.ArtifactKindPlan, Payload: "ok"},
	}
	err := euclotypes.PersistWorkflowArtifacts(context.Background(), store, "wf-1", "", artifacts)
	if err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

func TestPersistWorkflowArtifacts_FallbackArtifactIDFromKind(t *testing.T) {
	store := &stubArtifactStore{}
	artifacts := []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: nil}, // empty ID
	}
	err := euclotypes.PersistWorkflowArtifacts(context.Background(), store, "wf-1", "", artifacts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(store.records))
	}
	if store.records[0].ArtifactID == "" {
		t.Fatal("expected fallback ArtifactID from Kind, got empty string")
	}
}

// ---------------------------------------------------------------------------
// LoadPersistedArtifacts
// ---------------------------------------------------------------------------

func TestLoadPersistedArtifacts_NilStoreReturnsNil(t *testing.T) {
	artifacts, err := euclotypes.LoadPersistedArtifacts(context.Background(), nil, "wf-1", "run-1")
	if err != nil || artifacts != nil {
		t.Fatalf("expected nil,nil for nil store, got %v %v", artifacts, err)
	}
}

func TestLoadPersistedArtifacts_EmptyWorkflowIDReturnsNil(t *testing.T) {
	store := &stubArtifactStore{}
	artifacts, err := euclotypes.LoadPersistedArtifacts(context.Background(), store, "  ", "run-1")
	if err != nil || artifacts != nil {
		t.Fatalf("expected nil,nil for empty workflow ID, got %v %v", artifacts, err)
	}
}

func TestLoadPersistedArtifacts_RoundTrip(t *testing.T) {
	store := &stubArtifactStore{}
	original := []euclotypes.Artifact{
		{
			ID:         "plan-rt",
			Kind:       euclotypes.ArtifactKindPlan,
			Summary:    "round trip plan",
			ProducerID: "euclo:test",
			Status:     "produced",
			Payload:    map[string]any{"note": "hello"},
		},
	}
	if err := euclotypes.PersistWorkflowArtifacts(context.Background(), store, "wf-rt", "run-rt", original); err != nil {
		t.Fatalf("persist failed: %v", err)
	}
	loaded, err := euclotypes.LoadPersistedArtifacts(context.Background(), store, "wf-rt", "run-rt")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(loaded))
	}
	a := loaded[0]
	if a.ID != "plan-rt" {
		t.Fatalf("unexpected ID: %q", a.ID)
	}
	if a.Kind != euclotypes.ArtifactKindPlan {
		t.Fatalf("unexpected Kind: %q", a.Kind)
	}
	if a.ProducerID != "euclo:test" {
		t.Fatalf("unexpected ProducerID: %q", a.ProducerID)
	}
	if a.Status != "produced" {
		t.Fatalf("unexpected Status: %q", a.Status)
	}
}

func TestLoadPersistedArtifacts_StoreErrorPropagates(t *testing.T) {
	store := &stubArtifactStore{err: errors.New("io error")}
	_, err := euclotypes.LoadPersistedArtifacts(context.Background(), store, "wf-1", "run-1")
	if err == nil {
		t.Fatal("expected error from store")
	}
}

// ---------------------------------------------------------------------------
// RestoreStateFromArtifacts
// ---------------------------------------------------------------------------

func TestRestoreStateFromArtifacts_NilStateDoesNotPanic(t *testing.T) {
	euclotypes.RestoreStateFromArtifacts(nil, []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"x": 1}},
	})
}

func TestRestoreStateFromArtifacts_EmptyArtifactsDoesNothing(t *testing.T) {
	state := core.NewContext()
	euclotypes.RestoreStateFromArtifacts(state, nil)
	if _, ok := state.Get("euclo.artifacts"); ok {
		t.Fatal("expected no keys set for empty artifacts")
	}
}

func TestRestoreStateFromArtifacts_RestoresKnownArtifactKind(t *testing.T) {
	state := core.NewContext()
	payload := map[string]any{"scope_kind": "explicit"}
	artifacts := []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindVerificationPlan, Payload: payload},
	}
	euclotypes.RestoreStateFromArtifacts(state, artifacts)
	raw, ok := state.Get("euclo.verification_plan")
	if !ok {
		t.Fatal("expected euclo.verification_plan to be restored in state")
	}
	restored := raw.(map[string]any)
	if restored["scope_kind"] != "explicit" {
		t.Fatalf("unexpected restored value: %v", restored)
	}
}

func TestRestoreStateFromArtifacts_SetsArtifactsSlice(t *testing.T) {
	state := core.NewContext()
	artifacts := []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{}},
	}
	euclotypes.RestoreStateFromArtifacts(state, artifacts)
	if _, ok := state.Get("euclo.artifacts"); !ok {
		t.Fatal("expected euclo.artifacts to be set after restore")
	}
}

func TestRestoreStateFromArtifacts_IgnoresUnknownKind(t *testing.T) {
	state := core.NewContext()
	artifacts := []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKind("euclo.unknown_kind"), Payload: map[string]any{}},
	}
	euclotypes.RestoreStateFromArtifacts(state, artifacts)
	// Should still set euclo.artifacts even if the kind has no state key
	if _, ok := state.Get("euclo.artifacts"); !ok {
		t.Fatal("expected euclo.artifacts to be set even for unknown kind")
	}
}

// ---------------------------------------------------------------------------
// StateKeyForArtifactKind
// ---------------------------------------------------------------------------

func TestStateKeyForArtifactKind_KnownKinds(t *testing.T) {
	cases := []struct {
		kind euclotypes.ArtifactKind
		want string
	}{
		{euclotypes.ArtifactKindPlan, "pipeline.plan"},
		{euclotypes.ArtifactKindVerificationPlan, "euclo.verification_plan"},
		{euclotypes.ArtifactKindWaiver, "euclo.waiver"},
		{euclotypes.ArtifactKindTDDLifecycle, "euclo.tdd.lifecycle"},
		{euclotypes.ArtifactKindRootCause, "euclo.root_cause"},
		{euclotypes.ArtifactKindRegressionAnalysis, "euclo.regression_analysis"},
		{euclotypes.ArtifactKindExplore, "pipeline.explore"},
		{euclotypes.ArtifactKindSuccessGate, "euclo.success_gate"},
		{euclotypes.ArtifactKindLearningPromotion, "euclo.promoted_learning_interaction"},
	}
	for _, c := range cases {
		got := euclotypes.StateKeyForArtifactKind(c.kind)
		if got != c.want {
			t.Errorf("StateKeyForArtifactKind(%q) = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestStateKeyForArtifactKind_UnknownKindReturnsEmpty(t *testing.T) {
	got := euclotypes.StateKeyForArtifactKind("euclo.totally_unknown")
	if got != "" {
		t.Fatalf("expected empty string for unknown kind, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// CollectArtifactsFromState — additional coverage (nil state)
// ---------------------------------------------------------------------------

func TestCollectArtifactsFromState_NilStateReturnsNil(t *testing.T) {
	if got := euclotypes.CollectArtifactsFromState(nil); got != nil {
		t.Fatalf("expected nil for nil state, got %v", got)
	}
}

func TestCollectArtifactsFromState_EmptyStateReturnsEmpty(t *testing.T) {
	state := core.NewContext()
	if got := euclotypes.CollectArtifactsFromState(state); len(got) != 0 {
		t.Fatalf("expected empty for empty state, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// ArtifactState
// ---------------------------------------------------------------------------

func TestArtifactState_Has_ReturnsTrueWhenPresent(t *testing.T) {
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
	})
	if !state.Has(euclotypes.ArtifactKindPlan) {
		t.Fatal("expected Has to return true")
	}
}

func TestArtifactState_Has_ReturnsFalseWhenAbsent(t *testing.T) {
	state := euclotypes.NewArtifactState(nil)
	if state.Has(euclotypes.ArtifactKindPlan) {
		t.Fatal("expected Has to return false for empty state")
	}
}

func TestArtifactState_OfKind_ReturnsMatchingArtifacts(t *testing.T) {
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan, ID: "p1"},
		{Kind: euclotypes.ArtifactKindWaiver, ID: "w1"},
		{Kind: euclotypes.ArtifactKindPlan, ID: "p2"},
	})
	plans := state.OfKind(euclotypes.ArtifactKindPlan)
	if len(plans) != 2 {
		t.Fatalf("expected 2 plan artifacts, got %d", len(plans))
	}
}

func TestArtifactState_All_ReturnsAll(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
		{Kind: euclotypes.ArtifactKindWaiver},
	}
	state := euclotypes.NewArtifactState(artifacts)
	if len(state.All()) != 2 {
		t.Fatalf("expected 2, got %d", len(state.All()))
	}
}

func TestArtifactState_Len_ReturnsCount(t *testing.T) {
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
		{Kind: euclotypes.ArtifactKindPlan},
		{Kind: euclotypes.ArtifactKindPlan},
	})
	if state.Len() != 3 {
		t.Fatalf("expected Len=3, got %d", state.Len())
	}
}

// ---------------------------------------------------------------------------
// ArtifactContract
// ---------------------------------------------------------------------------

func TestArtifactContract_SatisfiedBy_EmptyContractAlwaysPasses(t *testing.T) {
	contract := euclotypes.ArtifactContract{}
	state := euclotypes.NewArtifactState(nil)
	if !contract.SatisfiedBy(state) {
		t.Fatal("expected empty contract to be satisfied by any state")
	}
}

func TestArtifactContract_SatisfiedBy_RequiredInputPresent(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindPlan, Required: true},
		},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
	})
	if !contract.SatisfiedBy(state) {
		t.Fatal("expected contract to be satisfied when required artifact is present")
	}
}

func TestArtifactContract_SatisfiedBy_RequiredInputAbsent(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindPlan, Required: true},
		},
	}
	state := euclotypes.NewArtifactState(nil)
	if contract.SatisfiedBy(state) {
		t.Fatal("expected contract to fail when required artifact is absent")
	}
}

func TestArtifactContract_SatisfiedBy_OptionalInputIgnored(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindPlan, Required: false},
		},
	}
	state := euclotypes.NewArtifactState(nil) // optional input missing — still OK
	if !contract.SatisfiedBy(state) {
		t.Fatal("expected optional input to not block contract satisfaction")
	}
}

func TestArtifactContract_SatisfiedBy_MinCountEnforced(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindPlan, Required: true, MinCount: 2},
		},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
	})
	if contract.SatisfiedBy(state) {
		t.Fatal("expected contract to fail when MinCount=2 but only 1 present")
	}
	state2 := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
		{Kind: euclotypes.ArtifactKindPlan},
	})
	if !contract.SatisfiedBy(state2) {
		t.Fatal("expected contract to be satisfied when MinCount=2 and 2 present")
	}
}

func TestArtifactContract_MissingInputs_ReturnsEmptyWhenSatisfied(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindPlan, Required: true},
		},
	}
	state := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindPlan}})
	if missing := contract.MissingInputs(state); len(missing) != 0 {
		t.Fatalf("expected no missing inputs, got %v", missing)
	}
}

func TestArtifactContract_MissingInputs_ReturnsMissingKinds(t *testing.T) {
	contract := euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindPlan, Required: true},
			{Kind: euclotypes.ArtifactKindWaiver, Required: true},
		},
	}
	state := euclotypes.NewArtifactState(nil)
	missing := contract.MissingInputs(state)
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing, got %v", missing)
	}
}

// ---------------------------------------------------------------------------
// ValidateArtifactProvenance
// ---------------------------------------------------------------------------

func TestValidateArtifactProvenance_NoWarningsForWellFormed(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindPlan, Status: "produced", ProducerID: "euclo:test"},
	}
	if warnings := euclotypes.ValidateArtifactProvenance(artifacts); len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestValidateArtifactProvenance_WarnsMissingProducerID(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindPlan, Status: "produced"},
	}
	warnings := euclotypes.ValidateArtifactProvenance(artifacts)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", warnings)
	}
}

func TestValidateArtifactProvenance_NoWarnForNonProducedStatus(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{ID: "a1", Kind: euclotypes.ArtifactKindPlan, Status: "pending"},
	}
	if warnings := euclotypes.ValidateArtifactProvenance(artifacts); len(warnings) != 0 {
		t.Fatalf("expected no warnings for pending status, got %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// ModeRegistry
// ---------------------------------------------------------------------------

func TestModeRegistry_NewRegistry_IsEmpty(t *testing.T) {
	r := euclotypes.NewModeRegistry()
	if list := r.List(); len(list) != 0 {
		t.Fatalf("expected empty registry, got %d descriptors", len(list))
	}
}

func TestModeRegistry_Register_AddsDescriptor(t *testing.T) {
	r := euclotypes.NewModeRegistry()
	if err := r.Register(euclotypes.ModeDescriptor{
		ModeID:                   "chat",
		DefaultExecutionProfiles: []string{"direct_edit"},
	}); err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}
	desc, ok := r.Lookup("chat")
	if !ok {
		t.Fatal("expected to find registered mode")
	}
	if desc.ModeID != "chat" {
		t.Fatalf("unexpected ModeID: %q", desc.ModeID)
	}
}

func TestModeRegistry_Register_NilRegistryReturnsError(t *testing.T) {
	var r *euclotypes.ModeRegistry
	if err := r.Register(euclotypes.ModeDescriptor{
		ModeID:                   "chat",
		DefaultExecutionProfiles: []string{"direct_edit"},
	}); err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestModeRegistry_Register_EmptyIDReturnsError(t *testing.T) {
	r := euclotypes.NewModeRegistry()
	if err := r.Register(euclotypes.ModeDescriptor{ModeID: ""}); err == nil {
		t.Fatal("expected error for empty mode ID")
	}
	if list := r.List(); len(list) != 0 {
		t.Fatalf("expected empty registry, got %d", len(list))
	}
}

func TestModeRegistry_Register_RequiresDefaultExecutionProfiles(t *testing.T) {
	r := euclotypes.NewModeRegistry()
	if err := r.Register(euclotypes.ModeDescriptor{ModeID: "chat"}); err == nil {
		t.Fatal("expected error when DefaultExecutionProfiles is empty")
	}
}

func TestModeRegistry_Lookup_NilRegistryReturnsFalse(t *testing.T) {
	var r *euclotypes.ModeRegistry
	_, ok := r.Lookup("chat")
	if ok {
		t.Fatal("expected false from nil registry")
	}
}

func TestModeRegistry_Lookup_UnknownIDReturnsFalse(t *testing.T) {
	r := euclotypes.NewModeRegistry()
	_, ok := r.Lookup("missing")
	if ok {
		t.Fatal("expected false for unknown mode")
	}
}

func TestModeRegistry_List_ReturnsSortedDescriptors(t *testing.T) {
	r := euclotypes.NewModeRegistry()
	profiles := []string{"p1"}
	_ = r.Register(euclotypes.ModeDescriptor{ModeID: "review", DefaultExecutionProfiles: profiles})
	_ = r.Register(euclotypes.ModeDescriptor{ModeID: "chat", DefaultExecutionProfiles: profiles})
	_ = r.Register(euclotypes.ModeDescriptor{ModeID: "debug", DefaultExecutionProfiles: profiles})
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 descriptors, got %d", len(list))
	}
	if list[0].ModeID != "chat" || list[1].ModeID != "debug" || list[2].ModeID != "review" {
		t.Fatalf("expected sorted order, got %v", list)
	}
}

func TestDefaultModeRegistry_IsNonEmpty(t *testing.T) {
	r := euclotypes.DefaultModeRegistry()
	if list := r.List(); len(list) == 0 {
		t.Fatal("expected default mode registry to have entries")
	}
}

func TestDefaultModeRegistry_ContainsExpectedModes(t *testing.T) {
	r := euclotypes.DefaultModeRegistry()
	for _, modeID := range []string{"code", "debug", "review", "planning", "tdd"} {
		if _, ok := r.Lookup(modeID); !ok {
			t.Errorf("expected mode %q in default registry", modeID)
		}
	}
}

// ---------------------------------------------------------------------------
// ExecutionProfileRegistry
// ---------------------------------------------------------------------------

func TestExecutionProfileRegistry_NewRegistry_IsEmpty(t *testing.T) {
	r := euclotypes.NewExecutionProfileRegistry()
	if list := r.List(); len(list) != 0 {
		t.Fatalf("expected empty registry, got %d", len(list))
	}
}

func TestExecutionProfileRegistry_Register_AddsDescriptor(t *testing.T) {
	r := euclotypes.NewExecutionProfileRegistry()
	if err := r.Register(euclotypes.ExecutionProfileDescriptor{
		ProfileID:      "direct_edit",
		SupportedModes: []string{"chat"},
	}); err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}
	desc, ok := r.Lookup("direct_edit")
	if !ok {
		t.Fatal("expected to find registered profile")
	}
	if desc.ProfileID != "direct_edit" {
		t.Fatalf("unexpected ProfileID: %q", desc.ProfileID)
	}
}

func TestExecutionProfileRegistry_Register_NilRegistryReturnsError(t *testing.T) {
	var r *euclotypes.ExecutionProfileRegistry
	if err := r.Register(euclotypes.ExecutionProfileDescriptor{
		ProfileID:      "x",
		SupportedModes: []string{"chat"},
	}); err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestExecutionProfileRegistry_Register_RequiresSupportedModes(t *testing.T) {
	r := euclotypes.NewExecutionProfileRegistry()
	if err := r.Register(euclotypes.ExecutionProfileDescriptor{ProfileID: "no-modes"}); err == nil {
		t.Fatal("expected error when SupportedModes is empty")
	}
}

func TestExecutionProfileRegistry_Lookup_NilRegistryReturnsFalse(t *testing.T) {
	var r *euclotypes.ExecutionProfileRegistry
	_, ok := r.Lookup("x")
	if ok {
		t.Fatal("expected false from nil registry")
	}
}

func TestExecutionProfileRegistry_List_ReturnsSortedDescriptors(t *testing.T) {
	r := euclotypes.NewExecutionProfileRegistry()
	modes := []string{"chat"}
	_ = r.Register(euclotypes.ExecutionProfileDescriptor{ProfileID: "z-profile", SupportedModes: modes})
	_ = r.Register(euclotypes.ExecutionProfileDescriptor{ProfileID: "a-profile", SupportedModes: modes})
	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].ProfileID != "a-profile" {
		t.Fatalf("expected sorted order, got %v", list)
	}
}

func TestDefaultExecutionProfileRegistry_IsNonEmpty(t *testing.T) {
	r := euclotypes.DefaultExecutionProfileRegistry()
	if list := r.List(); len(list) == 0 {
		t.Fatal("expected default profile registry to have entries")
	}
}

// ---------------------------------------------------------------------------
// ArtifactStateFromContext
// ---------------------------------------------------------------------------

func TestArtifactStateFromContext_NilStateReturnsEmptyState(t *testing.T) {
	state := euclotypes.ArtifactStateFromContext(nil)
	if state.Len() != 0 {
		t.Fatalf("expected empty state for nil context, got %d", state.Len())
	}
}

func TestArtifactStateFromContext_EmptyContextReturnsEmptyState(t *testing.T) {
	ctx := core.NewContext()
	state := euclotypes.ArtifactStateFromContext(ctx)
	if state.Len() != 0 {
		t.Fatalf("expected empty state for empty context, got %d", state.Len())
	}
}

func TestArtifactStateFromContext_ReadsArtifactsKey(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("euclo.artifacts", []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindPlan},
		{Kind: euclotypes.ArtifactKindWaiver},
	})
	state := euclotypes.ArtifactStateFromContext(ctx)
	if state.Len() != 2 {
		t.Fatalf("expected 2 artifacts, got %d", state.Len())
	}
}

// Suppress unused import warnings.
var _ = eucloruntime.ExecutionWaiver{}

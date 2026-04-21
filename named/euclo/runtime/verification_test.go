package runtime

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestNormalizeVerificationEvidence_AssignsFallbackProvenanceForStringPayload(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.verify", "verification looked okay")
	state.Set("euclo.run_id", "run-123")

	evidence := NormalizeVerificationEvidence(state)
	if evidence.Provenance != VerificationProvenanceFallback {
		t.Fatalf("expected fallback provenance, got %q", evidence.Provenance)
	}
	if evidence.RunID != "run-123" {
		t.Fatalf("expected run id to be backfilled, got %q", evidence.RunID)
	}
}

func TestNormalizeVerificationEvidence_AssignsReusedProvenanceForLatchedSummary(t *testing.T) {
	state := core.NewContext()
	state.Set("react.verification_latched_summary", "reused previous verification")
	state.Set("euclo.run_id", "run-456")

	evidence := NormalizeVerificationEvidence(state)
	if evidence.Provenance != VerificationProvenanceReused {
		t.Fatalf("expected reused provenance, got %q", evidence.Provenance)
	}
	if evidence.RunID != "run-456" {
		t.Fatalf("expected run id from state, got %q", evidence.RunID)
	}
}

func TestEvaluateSuccessGate_RejectsFallbackEvidenceForFreshEdits(t *testing.T) {
	policy := VerificationPolicy{
		RequiresVerification:  true,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: true,
	}
	evidence := VerificationEvidence{
		Status:          "pass",
		Provenance:      VerificationProvenanceFallback,
		EvidencePresent: true,
		Checks: []VerificationCheckRecord{
			{Name: "test", Status: "pass", Provenance: VerificationProvenanceExecuted},
		},
	}
	editRecord := &EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "main.go", Status: "applied"}},
	}

	result := EvaluateSuccessGate(policy, evidence, editRecord, nil)
	if result.Allowed {
		t.Fatal("expected fallback evidence to be rejected for fresh edits")
	}
	if result.Reason != "verification_fallback_rejected" {
		t.Fatalf("unexpected reason %q", result.Reason)
	}
	if result.AssuranceClass != AssuranceClassUnverifiedSuccess {
		t.Fatalf("unexpected assurance class %q", result.AssuranceClass)
	}
}

func TestEvaluateSuccessGate_RejectsReusedEvidenceForFreshEdits(t *testing.T) {
	policy := VerificationPolicy{
		RequiresVerification:  true,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: true,
	}
	evidence := VerificationEvidence{
		Status:          "pass",
		Provenance:      VerificationProvenanceReused,
		EvidencePresent: true,
		Checks: []VerificationCheckRecord{
			{Name: "test", Status: "pass", Provenance: VerificationProvenanceExecuted},
		},
	}
	editRecord := &EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "main.go", Status: "applied"}},
	}

	result := EvaluateSuccessGate(policy, evidence, editRecord, nil)
	if result.Allowed {
		t.Fatal("expected reused evidence to be rejected for fresh edits")
	}
	if result.Reason != "verification_reused_rejected" {
		t.Fatalf("unexpected reason %q", result.Reason)
	}
}

func TestDetectAutomaticVerificationDegradation_ForMissingVerificationTools(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.envelope", TaskEnvelope{
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{
			HasExecuteTools:      false,
			HasVerificationTools: false,
		},
	})
	mode, reason, degraded := DetectAutomaticVerificationDegradation(VerificationPolicy{RequiresVerification: true}, state, VerificationEvidence{})
	if !degraded {
		t.Fatal("expected automatic degradation to be detected")
	}
	if mode != "automatic" || reason != "verification_tools_unavailable" {
		t.Fatalf("unexpected degradation %q %q", mode, reason)
	}
}

func TestResolveVerificationPolicy_DebugProfileHonorsVerificationRequired(t *testing.T) {
	policy := ResolveVerificationPolicy(
		ModeResolution{ModeID: "debug"},
		ExecutionProfileSelection{ProfileID: "reproduce_localize_patch", VerificationRequired: true},
	)
	if !policy.RequiresVerification {
		t.Fatal("expected debug profile with verification_required to require verification")
	}
	if !policy.RequiresExecutedCheck {
		t.Fatal("expected debug profile with verification_required to require executed checks")
	}
}

func TestResolveVerificationPolicy_DebugModeWithoutVerificationRequiredDoesNotRequireVerification(t *testing.T) {
	policy := ResolveVerificationPolicy(
		ModeResolution{ModeID: "debug"},
		ExecutionProfileSelection{ProfileID: "trace_execute_analyze", VerificationRequired: false},
	)
	if policy.RequiresVerification {
		t.Fatal("expected debug profile without verification_required to skip verification")
	}
	if policy.RequiresExecutedCheck {
		t.Fatal("expected debug profile without verification_required to skip executed checks")
	}
}

func TestEvaluateSuccessGate_RejectsTDDWithoutRedEvidence(t *testing.T) {
	policy := VerificationPolicy{
		ProfileID:             "test_driven_generation",
		RequiresVerification:  true,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: true,
	}
	evidence := VerificationEvidence{
		Status:          "pass",
		Provenance:      VerificationProvenanceExecuted,
		EvidencePresent: true,
		RunID:           "run-1",
		Checks: []VerificationCheckRecord{
			{Name: "test", Status: "pass", Provenance: VerificationProvenanceExecuted, RunID: "run-1"},
		},
	}
	editRecord := &EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "main.go", Status: "applied"}},
	}
	state := core.NewContext()
	state.Set("euclo.tdd.green_evidence", map[string]any{"status": "pass", "run_id": "run-1"})

	result := EvaluateSuccessGate(policy, evidence, editRecord, state)
	if result.Allowed {
		t.Fatal("expected TDD run without red evidence to be rejected")
	}
	if result.Reason != "tdd_red_missing" {
		t.Fatalf("unexpected reason %q", result.Reason)
	}
	if result.AssuranceClass != AssuranceClassTDDIncomplete {
		t.Fatalf("unexpected assurance class %q", result.AssuranceClass)
	}
}

func TestEvaluateSuccessGate_AcceptsTDDWithRedAndGreenEvidence(t *testing.T) {
	policy := VerificationPolicy{
		ProfileID:             "test_driven_generation",
		RequiresVerification:  true,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: true,
	}
	evidence := VerificationEvidence{
		Status:          "pass",
		Provenance:      VerificationProvenanceExecuted,
		EvidencePresent: true,
		RunID:           "run-1",
		Checks: []VerificationCheckRecord{
			{Name: "test", Status: "pass", Provenance: VerificationProvenanceExecuted, RunID: "run-1"},
		},
	}
	editRecord := &EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "main.go", Status: "applied"}},
	}
	state := core.NewContext()
	state.Set("euclo.tdd.lifecycle", map[string]any{
		"current_phase":      "complete",
		"status":             "completed",
		"requested_refactor": false,
		"phase_history": []map[string]any{
			{"phase": "red", "status": "completed", "run_id": "run-1"},
			{"phase": "green", "status": "completed", "run_id": "run-1"},
			{"phase": "complete", "status": "completed", "run_id": "run-1"},
		},
	})
	state.Set("euclo.tdd.red_evidence", map[string]any{"status": "fail", "run_id": "run-1"})
	state.Set("euclo.tdd.green_evidence", map[string]any{"status": "pass", "run_id": "run-1"})

	result := EvaluateSuccessGate(policy, evidence, editRecord, state)
	if !result.Allowed {
		t.Fatalf("expected TDD run with red and green evidence to pass, got %#v", result)
	}
}

func TestEvaluateSuccessGate_RejectsTDDWithoutLifecycle(t *testing.T) {
	policy := VerificationPolicy{
		ProfileID:             "test_driven_generation",
		RequiresVerification:  true,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: true,
	}
	evidence := VerificationEvidence{
		Status:          "pass",
		Provenance:      VerificationProvenanceExecuted,
		EvidencePresent: true,
		RunID:           "run-1",
		Checks: []VerificationCheckRecord{
			{Name: "test", Status: "pass", Provenance: VerificationProvenanceExecuted, RunID: "run-1"},
		},
	}
	editRecord := &EditExecutionRecord{Executed: []EditOperationRecord{{Path: "main.go", Status: "applied"}}}
	state := core.NewContext()
	state.Set("euclo.tdd.red_evidence", map[string]any{"status": "fail", "run_id": "run-1"})
	state.Set("euclo.tdd.green_evidence", map[string]any{"status": "pass", "run_id": "run-1"})

	result := EvaluateSuccessGate(policy, evidence, editRecord, state)
	if result.Allowed {
		t.Fatal("expected TDD run without lifecycle to be rejected")
	}
	if result.Reason != "tdd_lifecycle_incomplete" {
		t.Fatalf("unexpected reason %q", result.Reason)
	}
}

func TestEvaluateSuccessGate_RejectsTDDWithoutRequestedRefactorEvidence(t *testing.T) {
	policy := VerificationPolicy{
		ProfileID:             "test_driven_generation",
		RequiresVerification:  true,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: true,
	}
	evidence := VerificationEvidence{
		Status:          "pass",
		Provenance:      VerificationProvenanceExecuted,
		EvidencePresent: true,
		RunID:           "run-1",
		Checks: []VerificationCheckRecord{
			{Name: "test", Status: "pass", Provenance: VerificationProvenanceExecuted, RunID: "run-1"},
		},
	}
	editRecord := &EditExecutionRecord{Executed: []EditOperationRecord{{Path: "main.go", Status: "applied"}}}
	state := core.NewContext()
	state.Set("euclo.tdd.lifecycle", map[string]any{
		"current_phase":      "complete",
		"status":             "completed",
		"requested_refactor": true,
		"phase_history": []map[string]any{
			{"phase": "red", "status": "completed", "run_id": "run-1"},
			{"phase": "green", "status": "completed", "run_id": "run-1"},
			{"phase": "complete", "status": "completed", "run_id": "run-1"},
		},
	})
	state.Set("euclo.tdd.red_evidence", map[string]any{"status": "fail", "run_id": "run-1"})
	state.Set("euclo.tdd.green_evidence", map[string]any{"status": "pass", "run_id": "run-1"})

	result := EvaluateSuccessGate(policy, evidence, editRecord, state)
	if result.Allowed {
		t.Fatal("expected TDD run without refactor evidence to be rejected")
	}
	if result.Reason != "tdd_refactor_missing" {
		t.Fatalf("unexpected reason %q", result.Reason)
	}
}

func TestVerificationHelperBranchesAndDegradationFallbacks(t *testing.T) {
	if !verificationBoolValue(true) || verificationBoolValue(false) {
		t.Fatal("unexpected bool parsing")
	}
	if !verificationBoolValue(" true ") || verificationBoolValue("no") {
		t.Fatal("unexpected string bool parsing")
	}
	if got := verificationEvidenceFromRaw(nil); got.Status != "not_verified" || got.Source != "pipeline.verify" {
		t.Fatalf("unexpected raw verification fallback: %#v", got)
	}

	typedHistory := phaseHistoryRecords([]map[string]any{
		{"phase": "red", "run_id": "run-1"},
		{"phase": "green", "run_id": "run-1"},
	})
	if len(typedHistory) != 2 {
		t.Fatalf("unexpected typed history: %#v", typedHistory)
	}
	mixedHistory := phaseHistoryRecords([]any{
		map[string]any{"phase": "complete", "run_id": "run-1"},
		"ignore",
	})
	if len(mixedHistory) != 1 {
		t.Fatalf("unexpected mixed history: %#v", mixedHistory)
	}

	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{
		"capability_snapshot": map[string]any{
			"has_execute_tools":      false,
			"has_verification_tools": false,
		},
	})
	mode, reason, degraded := DetectAutomaticVerificationDegradation(VerificationPolicy{RequiresVerification: true}, state, VerificationEvidence{})
	if !degraded || mode != "automatic" || reason != "verification_tools_unavailable" {
		t.Fatalf("unexpected tool degradation: %q %q %v", mode, reason, degraded)
	}

	state = core.NewContext()
	state.Set("euclo.verification_plan", map[string]any{"commands": []any{}})
	mode, reason, degraded = DetectAutomaticVerificationDegradation(VerificationPolicy{RequiresVerification: true}, state, VerificationEvidence{})
	if !degraded || mode != "automatic" || reason != "verification_plan_unavailable" {
		t.Fatalf("unexpected plan degradation: %q %q %v", mode, reason, degraded)
	}

	state = core.NewContext()
	state.Set("euclo.envelope", TaskEnvelope{
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{
			HasExecuteTools:      true,
			HasVerificationTools: true,
		},
	})
	mode, reason, degraded = DetectAutomaticVerificationDegradation(VerificationPolicy{RequiresVerification: true}, state, VerificationEvidence{
		EvidencePresent: true,
		Provenance:      VerificationProvenanceExecuted,
	})
	if degraded || mode != "" || reason != "" {
		t.Fatalf("unexpected non-degraded result: %q %q %v", mode, reason, degraded)
	}

	state = core.NewContext()
	state.Set("euclo.tdd.lifecycle", map[string]any{"requested_refactor": "yes"})
	if !tddLifecycleRequestedRefactor(state) {
		t.Fatal("expected string requested_refactor to be recognized")
	}
}

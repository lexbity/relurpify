package state

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// TestGetVerificationPolicy_EmptyContext verifies that GetVerificationPolicy on empty context returns zero value and false.
func TestGetVerificationPolicy_EmptyContext(t *testing.T) {
	ctx := core.NewContext()
	policy, ok := GetVerificationPolicy(ctx)
	if ok {
		t.Error("expected ok to be false on empty context")
	}
	if policy.PolicyID != "" {
		t.Errorf("expected zero value PolicyID, got %q", policy.PolicyID)
	}
}

// TestSetGetVerificationPolicy_RoundTrip verifies that SetVerificationPolicy then GetVerificationPolicy round-trips without loss.
func TestSetGetVerificationPolicy_RoundTrip(t *testing.T) {
	ctx := core.NewContext()
	expected := runtimepkg.VerificationPolicy{
		PolicyID:              "code/default",
		ModeID:                "code",
		ProfileID:             "default",
		RequiresVerification:  true,
		RequiresExecutedCheck: true,
	}

	SetVerificationPolicy(ctx, expected)
	got, ok := GetVerificationPolicy(ctx)
	if !ok {
		t.Fatal("expected ok to be true after setting")
	}
	if got.PolicyID != expected.PolicyID {
		t.Errorf("PolicyID: expected %q, got %q", expected.PolicyID, got.PolicyID)
	}
	if got.ModeID != expected.ModeID {
		t.Errorf("ModeID: expected %q, got %q", expected.ModeID, got.ModeID)
	}
	if got.RequiresVerification != expected.RequiresVerification {
		t.Errorf("RequiresVerification: expected %v, got %v", expected.RequiresVerification, got.RequiresVerification)
	}
}

// TestGetRecoveryTrace_FromMap verifies that GetRecoveryTrace can read from legacy map format.
func TestGetRecoveryTrace_FromMap(t *testing.T) {
	ctx := core.NewContext()
	legacyMap := map[string]any{
		"status":        "repair_exhausted",
		"attempt_count": 3,
		"max_attempts":  5,
		"reason":        "max attempts reached",
	}
	ctx.Set(KeyRecoveryTrace, legacyMap)

	trace, ok := GetRecoveryTrace(ctx)
	if !ok {
		t.Fatal("expected ok to be true")
	}
	if trace.Status != "repair_exhausted" {
		t.Errorf("Status: expected %q, got %q", "repair_exhausted", trace.Status)
	}
	if trace.AttemptCount != 3 {
		t.Errorf("AttemptCount: expected %d, got %d", 3, trace.AttemptCount)
	}
	if trace.MaxAttempts != 5 {
		t.Errorf("MaxAttempts: expected %d, got %d", 5, trace.MaxAttempts)
	}
	if trace.Reason != "max attempts reached" {
		t.Errorf("Reason: expected %q, got %q", "max attempts reached", trace.Reason)
	}
}

// TestGetRecoveryTrace_TypedStruct verifies that GetRecoveryTrace reads typed struct correctly.
func TestGetRecoveryTrace_TypedStruct(t *testing.T) {
	ctx := core.NewContext()
	expected := RecoveryTrace{
		Status:       "repaired",
		AttemptCount: 2,
		MaxAttempts:  3,
		Reason:       "successful repair",
	}
	ctx.Set(KeyRecoveryTrace, expected)

	got, ok := GetRecoveryTrace(ctx)
	if !ok {
		t.Fatal("expected ok to be true")
	}
	if got.Status != expected.Status {
		t.Errorf("Status: expected %q, got %q", expected.Status, got.Status)
	}
	if got.AttemptCount != expected.AttemptCount {
		t.Errorf("AttemptCount: expected %d, got %d", expected.AttemptCount, got.AttemptCount)
	}
}

// TestGetBehaviorTrace_TypeMismatch returns zero value and false on type mismatch, no panic.
func TestGetBehaviorTrace_TypeMismatch(t *testing.T) {
	ctx := core.NewContext()
	// Set an incompatible type
	ctx.Set(KeyBehaviorTrace, "not a trace")

	trace, ok := GetBehaviorTrace(ctx)
	if ok {
		t.Error("expected ok to be false on type mismatch")
	}
	if trace.PrimaryCapabilityID != "" {
		t.Errorf("expected empty PrimaryCapabilityID, got %q", trace.PrimaryCapabilityID)
	}
}

// TestNilContextAccessors verifies that nil context returns zero value and false on all getters.
func TestNilContextAccessors(t *testing.T) {
	tests := []struct {
		name string
		test func() (any, bool)
	}{
		{"GetVerificationPolicy", func() (any, bool) { v, ok := GetVerificationPolicy(nil); return v, ok }},
		{"GetVerification", func() (any, bool) { v, ok := GetVerification(nil); return v, ok }},
		{"GetSuccessGate", func() (any, bool) { v, ok := GetSuccessGate(nil); return v, ok }},
		{"GetAssuranceClass", func() (any, bool) { v, ok := GetAssuranceClass(nil); return v, ok }},
		{"GetRecoveryTrace", func() (any, bool) { v, ok := GetRecoveryTrace(nil); return v, ok }},
		{"GetBehaviorTrace", func() (any, bool) { v, ok := GetBehaviorTrace(nil); return v, ok }},
		{"GetArtifacts", func() (any, bool) { v, ok := GetArtifacts(nil); return v, ok }},
		{"GetActionLog", func() (any, bool) { v, ok := GetActionLog(nil); return v, ok }},
		{"GetProofSurface", func() (any, bool) { v, ok := GetProofSurface(nil); return v, ok }},
		{"GetFinalReport", func() (any, bool) { v, ok := GetFinalReport(nil); return v, ok }},
		{"GetSharedContextRuntime", func() (any, bool) { v, ok := GetSharedContextRuntime(nil); return v, ok }},
		{"GetSecurityRuntime", func() (any, bool) { v, ok := GetSecurityRuntime(nil); return v, ok }},
		{"GetUnitOfWork", func() (any, bool) { v, ok := GetUnitOfWork(nil); return v, ok }},
		{"GetUnitOfWorkHistory", func() (any, bool) { v, ok := GetUnitOfWorkHistory(nil); return v, ok }},
		{"GetEnvelope", func() (any, bool) { v, ok := GetEnvelope(nil); return v, ok }},
		{"GetClassification", func() (any, bool) { v, ok := GetClassification(nil); return v, ok }},
		{"GetMode", func() (any, bool) { v, ok := GetMode(nil); return v, ok }},
		{"GetExecutionProfile", func() (any, bool) { v, ok := GetExecutionProfile(nil); return v, ok }},
		{"GetRetrievalPolicy", func() (any, bool) { v, ok := GetRetrievalPolicy(nil); return v, ok }},
		{"GetPipelineExplore", func() (any, bool) { v, ok := GetPipelineExplore(nil); return v, ok }},
		{"GetPipelineAnalyze", func() (any, bool) { v, ok := GetPipelineAnalyze(nil); return v, ok }},
		{"GetPipelinePlan", func() (any, bool) { v, ok := GetPipelinePlan(nil); return v, ok }},
		{"GetPipelineCode", func() (any, bool) { v, ok := GetPipelineCode(nil); return v, ok }},
		{"GetPipelineVerify", func() (any, bool) { v, ok := GetPipelineVerify(nil); return v, ok }},
		{"GetPipelineFinalOutput", func() (any, bool) { v, ok := GetPipelineFinalOutput(nil); return v, ok }},
		{"GetPreClassifiedCapabilitySequence", func() (any, bool) { v, ok := GetPreClassifiedCapabilitySequence(nil); return v, ok }},
		{"GetCapabilitySequenceOperator", func() (any, bool) { v, ok := GetCapabilitySequenceOperator(nil); return v, ok }},
		{"GetClassificationSource", func() (any, bool) { v, ok := GetClassificationSource(nil); return v, ok }},
		{"GetClassificationMeta", func() (any, bool) { v, ok := GetClassificationMeta(nil); return v, ok }},
		{"GetWorkflowID", func() (any, bool) { v, ok := GetWorkflowID(nil); return v, ok }},
		{"GetReviewFindings", func() (any, bool) { v, ok := GetReviewFindings(nil); return v, ok }},
		{"GetRootCause", func() (any, bool) { v, ok := GetRootCause(nil); return v, ok }},
		{"GetRootCauseCandidates", func() (any, bool) { v, ok := GetRootCauseCandidates(nil); return v, ok }},
		{"GetRegressionAnalysis", func() (any, bool) { v, ok := GetRegressionAnalysis(nil); return v, ok }},
		{"GetPlanCandidates", func() (any, bool) { v, ok := GetPlanCandidates(nil); return v, ok }},
		{"GetVerificationSummary", func() (any, bool) { v, ok := GetVerificationSummary(nil); return v, ok }},
		{"GetEditExecution", func() (any, bool) { v, ok := GetEditExecution(nil); return v, ok }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.test()
			if ok {
				t.Errorf("%s: expected ok to be false with nil context", tt.name)
			}
		})
	}
}

// TestNilContextSetters verifies that nil context does not panic on setters.
func TestNilContextSetters(t *testing.T) {
	// All setters should be no-ops with nil context and not panic
	SetVerificationPolicy(nil, runtimepkg.VerificationPolicy{})
	SetVerification(nil, runtimepkg.VerificationEvidence{})
	SetSuccessGate(nil, runtimepkg.SuccessGateResult{})
	SetAssuranceClass(nil, "")
	SetRecoveryTrace(nil, RecoveryTrace{})
	SetBehaviorTrace(nil, Trace{})
	SetArtifacts(nil, nil)
	SetActionLog(nil, nil)
	SetProofSurface(nil, runtimepkg.ProofSurface{})
	SetFinalReport(nil, nil)
	SetSharedContextRuntime(nil, runtimepkg.SharedContextRuntimeState{})
	SetSecurityRuntime(nil, runtimepkg.SecurityRuntimeState{})
	SetUnitOfWork(nil, runtimepkg.UnitOfWork{})
	SetUnitOfWorkHistory(nil, nil)
	SetEnvelope(nil, runtimepkg.TaskEnvelope{})
	SetClassification(nil, runtimepkg.TaskClassification{})
	SetMode(nil, "")
	SetExecutionProfile(nil, "")
	SetRetrievalPolicy(nil, runtimepkg.RetrievalPolicy{})
	SetPipelineExplore(nil, nil)
	SetPipelineAnalyze(nil, nil)
	SetPipelinePlan(nil, nil)
	SetPipelineCode(nil, nil)
	SetPipelineVerify(nil, nil)
	SetPipelineFinalOutput(nil, nil)
	SetPreClassifiedCapabilitySequence(nil, nil)
	SetCapabilitySequenceOperator(nil, "")
	SetClassificationSource(nil, "")
	SetClassificationMeta(nil, "")
	SetWorkflowID(nil, "")
	SetReviewFindings(nil, nil)
	SetRootCause(nil, nil)
	SetRootCauseCandidates(nil, nil)
	SetRegressionAnalysis(nil, nil)
	SetPlanCandidates(nil, nil)
	SetVerificationSummary(nil, nil)
	SetEditExecution(nil, runtimepkg.EditExecutionRecord{})
}

// TestSettersWithNilContextNoPanic specifically tests that setters don't panic with nil context.
func TestSettersWithNilContextNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("setter panicked with nil context: %v", r)
		}
	}()

	// This test just ensures no panic occurs
	SetVerificationPolicy(nil, runtimepkg.VerificationPolicy{PolicyID: "test"})
}

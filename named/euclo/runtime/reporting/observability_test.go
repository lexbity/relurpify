package reporting

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

func TestBuildProofSurface_CarriesAssuranceAndWaiver(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode_resolution", eucloruntime.ModeResolution{ModeID: "code"})
	state.Set("euclo.execution_profile_selection", euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"})
	state.Set("euclo.verification", eucloruntime.VerificationEvidence{
		Status:     "pass",
		Provenance: eucloruntime.VerificationProvenanceExecuted,
	})
	state.Set("euclo.success_gate", eucloruntime.SuccessGateResult{
		Allowed:           true,
		Reason:            "manual_verification_allowed",
		AssuranceClass:    eucloruntime.AssuranceClassOperatorDeferred,
		WaiverApplied:     true,
		DegradationMode:   "operator_waiver",
		DegradationReason: "operator_waiver",
	})
	state.Set("euclo.recovery_trace", map[string]any{
		"status":        "repaired",
		"attempt_count": 1,
	})

	proof := BuildProofSurface(state, nil)
	if proof.VerificationProvenance != "executed" {
		t.Fatalf("expected verification provenance, got %q", proof.VerificationProvenance)
	}
	if proof.AssuranceClass != "operator_deferred" {
		t.Fatalf("expected assurance class, got %q", proof.AssuranceClass)
	}
	if !proof.WaiverApplied {
		t.Fatal("expected waiver applied to be true")
	}
	if proof.DegradationMode != "operator_waiver" {
		t.Fatalf("expected degradation mode, got %q", proof.DegradationMode)
	}
	if proof.DegradationReason != "operator_waiver" {
		t.Fatalf("expected degradation reason, got %q", proof.DegradationReason)
	}
	if proof.RecoveryStatus != "repaired" {
		t.Fatalf("expected recovery status, got %q", proof.RecoveryStatus)
	}
	if proof.RecoveryAttempts != 1 {
		t.Fatalf("expected recovery attempts, got %d", proof.RecoveryAttempts)
	}
}

func TestEmitObservabilityTelemetry_NilTelemetryIsSafe(t *testing.T) {
	EmitObservabilityTelemetry(nil, &core.Task{ID: "nil-tel"}, nil, eucloruntime.ProofSurface{})
}

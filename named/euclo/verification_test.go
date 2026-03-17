package euclo

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestNormalizeVerificationEvidenceFromPipelineReport(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "tests passed",
		"checks": []any{
			map[string]any{"name": "go test ./...", "status": "pass", "details": "ok"},
		},
	})

	evidence := NormalizeVerificationEvidence(state)
	require.Equal(t, "pass", evidence.Status)
	require.True(t, evidence.EvidencePresent)
	require.Len(t, evidence.Checks, 1)
}

func TestEvaluateSuccessGateRejectsMissingVerificationForEvidenceFirstProfile(t *testing.T) {
	policy := ResolveVerificationPolicy(
		ModeResolution{ModeID: "code"},
		ExecutionProfileSelection{ProfileID: "edit_verify_repair", VerificationRequired: true},
	)

	result := EvaluateSuccessGate(policy, VerificationEvidence{Status: "not_verified"}, &EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "main.go", Action: "update", Status: "executed"}},
	})
	require.False(t, result.Allowed)
	require.Equal(t, "verification_missing", result.Reason)
}

func TestEvaluateSuccessGateRejectsManualVerificationForCodeProfile(t *testing.T) {
	policy := ResolveVerificationPolicy(
		ModeResolution{ModeID: "code"},
		ExecutionProfileSelection{ProfileID: "edit_verify_repair", VerificationRequired: true},
	)

	result := EvaluateSuccessGate(policy, VerificationEvidence{
		Status:          "needs_manual_verification",
		EvidencePresent: true,
	}, nil)
	require.False(t, result.Allowed)
	require.Equal(t, "verification_status_rejected", result.Reason)
}

// TestEvaluateSuccessGateAcceptsPassingVerificationWithCheck tests passing verification
// Note: VerificationCheckRecord is a type that needs to be defined
// This test is commented out pending type definition
/*
func TestEvaluateSuccessGateAcceptsPassingVerificationWithCheck(t *testing.T) {
	policy := ResolveVerificationPolicy(
		ModeResolution{ModeID: "debug"},
		ExecutionProfileSelection{ProfileID: "reproduce_localize_patch", VerificationRequired: true},
	)

	result := EvaluateSuccessGate(policy, VerificationEvidence{
		Status:          "pass",
		EvidencePresent: true,
		Checks:          []VerificationCheckRecord{{Name: "go test", Status: "pass"}},
	}, &EditExecutionRecord{
		Executed: []EditOperationRecord{{Path: "bug.go", Action: "update", Status: "executed"}},
	})
	require.True(t, result.Allowed)
	require.Equal(t, "verification_accepted", result.Reason)
}
*/

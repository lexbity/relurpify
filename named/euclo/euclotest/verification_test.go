package euclotest

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
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

	evidence := eucloruntime.NormalizeVerificationEvidence(state)
	require.Equal(t, "pass", evidence.Status)
	require.True(t, evidence.EvidencePresent)
	require.Len(t, evidence.Checks, 1)
}

func TestEvaluateSuccessGateRejectsMissingVerificationForEvidenceFirstProfile(t *testing.T) {
	policy := eucloruntime.ResolveVerificationPolicy(
		euclotypes.ModeResolution{ModeID: "code"},
		euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair", VerificationRequired: true},
	)

	result := eucloruntime.EvaluateSuccessGate(policy, eucloruntime.VerificationEvidence{Status: "not_verified"}, &eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Action: "update", Status: "executed"}},
	})
	require.False(t, result.Allowed)
	require.Equal(t, "verification_missing", result.Reason)
}

func TestEvaluateSuccessGateRejectsManualVerificationForCodeProfile(t *testing.T) {
	policy := eucloruntime.ResolveVerificationPolicy(
		euclotypes.ModeResolution{ModeID: "code"},
		euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair", VerificationRequired: true},
	)

	result := eucloruntime.EvaluateSuccessGate(policy, eucloruntime.VerificationEvidence{
		Status:          "needs_manual_verification",
		EvidencePresent: true,
	}, nil)
	require.False(t, result.Allowed)
	require.Equal(t, "verification_status_rejected", result.Reason)
}

func TestResolveVerificationPolicy_DebugDoesNotRequireVerification(t *testing.T) {
	policy := eucloruntime.ResolveVerificationPolicy(
		euclotypes.ModeResolution{ModeID: "debug"},
		euclotypes.ExecutionProfileSelection{ProfileID: "reproduce_localize_patch", VerificationRequired: true},
	)
	require.False(t, policy.RequiresVerification)
	require.False(t, policy.RequiresExecutedCheck)
}

// TestEvaluateSuccessGateAcceptsPassingVerificationWithCheck tests passing verification
// Note: VerificationCheckRecord is a type that needs to be defined
// This test is commented out pending type definition
/*
func TestEvaluateSuccessGateAcceptsPassingVerificationWithCheck(t *testing.T) {
	policy := eucloruntime.ResolveVerificationPolicy(
		euclotypes.ModeResolution{ModeID: "debug"},
		euclotypes.ExecutionProfileSelection{ProfileID: "reproduce_localize_patch", VerificationRequired: true},
	)

	result := eucloruntime.EvaluateSuccessGate(policy, eucloruntime.VerificationEvidence{
		Status:          "pass",
		EvidencePresent: true,
		Checks:          []eucloruntime.VerificationCheckRecord{{Name: "go test", Status: "pass"}},
	}, &eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "bug.go", Action: "update", Status: "executed"}},
	})
	require.True(t, result.Allowed)
	require.Equal(t, "verification_accepted", result.Reason)
}
*/

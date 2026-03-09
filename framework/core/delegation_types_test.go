package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelegationRequestValidateRequiresCoreFields(t *testing.T) {
	valid := DelegationRequest{
		ID:                 "delegation-1",
		TargetCapabilityID: "agent:planner",
		TaskType:           "plan",
		Instruction:        "Produce a plan",
	}

	require.NoError(t, valid.Validate())

	missingID := valid
	missingID.ID = ""
	require.ErrorContains(t, missingID.Validate(), "delegation id required")

	missingTarget := valid
	missingTarget.TargetCapabilityID = ""
	require.ErrorContains(t, missingTarget.Validate(), "target capability id required")

	missingTaskType := valid
	missingTaskType.TaskType = ""
	require.ErrorContains(t, missingTaskType.Validate(), "task type required")

	missingInstruction := valid
	missingInstruction.Instruction = ""
	require.ErrorContains(t, missingInstruction.Validate(), "instruction required")
}

func TestDelegationRequestValidateRejectsNegativeDepthAndEmptyResourceRefs(t *testing.T) {
	request := DelegationRequest{
		ID:                 "delegation-1",
		TargetCapabilityID: "agent:architect",
		TaskType:           "design",
		Instruction:        "Design the integration",
		Depth:              -1,
	}

	require.ErrorContains(t, request.Validate(), "depth cannot be negative")

	request.Depth = 1
	request.ResourceRefs = []string{"workflow://state/current", " "}
	require.ErrorContains(t, request.Validate(), "resource refs cannot contain empty values")
}

func TestDelegationResultValidateEnforcesStateSuccessConsistency(t *testing.T) {
	valid := DelegationResult{
		DelegationID: "delegation-1",
		State:        DelegationStateSucceeded,
		Success:      true,
	}

	require.NoError(t, valid.Validate())

	invalidState := valid
	invalidState.State = DelegationState("mystery")
	require.ErrorContains(t, invalidState.Validate(), "invalid")

	failedSucceeded := valid
	failedSucceeded.Success = false
	require.ErrorContains(t, failedSucceeded.Validate(), "must be successful")

	failedResult := DelegationResult{
		DelegationID: "delegation-1",
		State:        DelegationStateFailed,
		Success:      true,
	}
	require.ErrorContains(t, failedResult.Validate(), "cannot be successful")

	invalidRefs := valid
	invalidRefs.ResourceRefs = []string{"resource://ok", ""}
	require.ErrorContains(t, invalidRefs.Validate(), "resource refs cannot contain empty values")

	invalidDiagnostics := valid
	invalidDiagnostics.Diagnostics = []string{"ok", " "}
	require.ErrorContains(t, invalidDiagnostics.Validate(), "diagnostics cannot contain empty values")
}

func TestNewDelegationResultDefaultsInsertionAndProvenance(t *testing.T) {
	request := DelegationRequest{
		ID:                 "delegation-1",
		TargetCapabilityID: "agent:planner",
		TargetProviderID:   "provider-1",
		TargetSessionID:    "session-1",
		ResourceRefs:       []string{"workflow://state/current"},
	}
	snapshot := &PolicySnapshot{ID: "policy-1"}
	data := map[string]any{"summary": "ready"}

	result := NewDelegationResult(
		request,
		"",
		"",
		"",
		TrustClassRemoteApproved,
		DelegationStateSucceeded,
		true,
		data,
		snapshot,
	)

	require.NotNil(t, result)
	require.Equal(t, "delegation-1", result.DelegationID)
	require.Equal(t, "agent:planner", result.TargetCapabilityID)
	require.Equal(t, "provider-1", result.ProviderID)
	require.Equal(t, "session-1", result.SessionID)
	require.Equal(t, DelegationStateSucceeded, result.State)
	require.True(t, result.Success)
	require.Equal(t, InsertionActionSummarized, result.Insertion.Action)
	require.Equal(t, "policy-1", result.Insertion.PolicySnapshotID)
	require.Equal(t, TrustClassRemoteApproved, result.Provenance.TrustClass)
	require.Equal(t, ContentDispositionRaw, result.Provenance.Disposition)
	require.Equal(t, ContentDispositionRaw, result.Disposition)
	require.Equal(t, []string{"workflow://state/current"}, result.ResourceRefs)
	require.False(t, result.RecordedAt.IsZero())
	require.False(t, result.CompletedAt.IsZero())

	data["summary"] = "mutated"
	require.Equal(t, "ready", result.Data["summary"])
}

func TestApplyDelegationInsertionDecisionUpdatesRequiresHITL(t *testing.T) {
	result := &DelegationResult{
		DelegationID: "delegation-1",
		Insertion: InsertionDecision{
			PolicySnapshotID: "policy-1",
		},
	}

	ApplyDelegationInsertionDecision(result, InsertionDecision{
		Action: InsertionActionHITLRequired,
		Reason: "approval required",
	})

	require.Equal(t, InsertionActionHITLRequired, result.Insertion.Action)
	require.True(t, result.Insertion.RequiresHITL)
	require.Equal(t, "policy-1", result.Insertion.PolicySnapshotID)
}

func TestApprovalBindingFromDelegationUsesRequestAndResultContext(t *testing.T) {
	request := DelegationRequest{
		ID:                 "delegation-1",
		WorkflowID:         "workflow-1",
		TaskID:             "task-1",
		TargetCapabilityID: "agent:architect",
		TaskType:           "design",
		Instruction:        "Produce a design",
		ResourceRefs:       []string{"workflow://artifact/design-brief"},
	}
	result := &DelegationResult{
		DelegationID:       "delegation-1",
		TargetCapabilityID: "agent:architect-fallback",
		ProviderID:         "provider-1",
		SessionID:          "session-1",
	}

	binding := ApprovalBindingFromDelegation(request, result)
	require.NotNil(t, binding)
	require.Equal(t, "agent:architect", binding.CapabilityID)
	require.Equal(t, "provider-1", binding.ProviderID)
	require.Equal(t, "session-1", binding.SessionID)
	require.Equal(t, "workflow://artifact/design-brief", binding.TargetResource)
	require.Equal(t, "task-1", binding.TaskID)
	require.Equal(t, "workflow-1", binding.WorkflowID)
}

func TestDelegationSnapshotValidateMatchesRequestAndResult(t *testing.T) {
	snapshot := DelegationSnapshot{
		Request: DelegationRequest{
			ID:                 "delegation-1",
			TargetCapabilityID: "agent:reviewer",
			TaskType:           "review",
			Instruction:        "Review the diff",
		},
		State:          DelegationStateSucceeded,
		TrustClass:     TrustClassRemoteApproved,
		Recoverability: RecoverabilityInProcess,
		Result: &DelegationResult{
			DelegationID: "delegation-1",
			State:        DelegationStateSucceeded,
			Success:      true,
		},
	}

	require.NoError(t, snapshot.Validate())

	snapshot.Result.DelegationID = "other"
	require.ErrorContains(t, snapshot.Validate(), "does not match request id")
}

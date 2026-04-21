package assurance

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclopolicy "codeburg.org/lexbit/relurpify/named/euclo/runtime/policy"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

// VerificationGate evaluates verification policy and success gates.
type VerificationGate struct {
	Environment agentenv.AgentEnvironment
}

// GateResult is the result of verification gate evaluation.
type GateResult struct {
	Evidence    eucloruntime.VerificationEvidence
	SuccessGate eucloruntime.SuccessGateResult
	Err         error
}

// Evaluate applies verification policy, evaluates the success gate,
// and handles waivers and automatic degradation detection.
// This corresponds to the verification half of the old applyVerificationAndArtifacts.
func (g VerificationGate) Evaluate(ctx context.Context, in Input, mutationAllowed bool) GateResult {
	// Resolve and set verification policy
	policy := euclopolicy.ResolveVerificationPolicy(in.Mode, in.Profile)
	euclostate.SetVerificationPolicy(in.State, policy)

	// Apply edit intent artifacts if mutation is allowed
	if mutationAllowed {
		if _, applyErr := eucloruntime.ApplyEditIntentArtifacts(ctx, g.Environment.Registry, in.State); applyErr != nil {
			return GateResult{Err: applyErr}
		}
	}

	// Normalize verification evidence
	evidence := eucloruntime.NormalizeVerificationEvidence(in.State)
	euclostate.SetVerification(in.State, evidence)

	// Get edit execution record if available
	var editRecord *eucloruntime.EditExecutionRecord
	if raw, ok := euclostate.GetEditExecution(in.State); ok {
		editRecord = &raw
	}

	// Evaluate success gate
	successGate := eucloruntime.EvaluateSuccessGate(policy, evidence, editRecord, in.State)

	// Apply waiver if present
	if _, ok := euclostate.GetExecutionWaiver(in.State); ok {
		originalReason := successGate.Reason
		successGate.WaiverApplied = true
		successGate.DegradationMode = "operator_waiver"
		successGate.DegradationReason = "operator_waiver"
		successGate.AutomaticDegradation = false
		successGate.Allowed = true
		if originalReason != "" && originalReason != "manual_verification_allowed" {
			successGate.Details = append(successGate.Details, "waived_reason="+originalReason)
			successGate.Reason = "operator_waiver_applied"
		}
		successGate.AssuranceClass = eucloruntime.AssuranceClassOperatorDeferred
	} else if mode, reason, degraded := eucloruntime.DetectAutomaticVerificationDegradation(policy, in.State, evidence); degraded {
		// Apply automatic degradation detection
		successGate.AutomaticDegradation = true
		successGate.DegradationMode = mode
		successGate.DegradationReason = reason
	}

	// Apply recovery trace effects
	if trace, ok := euclostate.GetRecoveryTrace(in.State); ok {
		switch trace.Status {
		case "repair_exhausted":
			successGate.AssuranceClass = eucloruntime.AssuranceClassRepairExhausted
			if successGate.Reason == "" || successGate.Reason == "verification_status_rejected" {
				successGate.Reason = "repair_exhausted"
			}
			if trace.AttemptCount > 0 {
				successGate.Details = append(successGate.Details, fmt.Sprintf("repair_attempt_count=%d", trace.AttemptCount))
			}
		case "repaired":
			if trace.AttemptCount > 0 {
				successGate.Details = append(successGate.Details, fmt.Sprintf("repair_attempt_count=%d", trace.AttemptCount))
			}
		}
	}

	// Persist success gate and assurance class to state
	euclostate.SetSuccessGate(in.State, successGate)
	euclostate.SetAssuranceClass(in.State, successGate.AssuranceClass)

	// Copy execution waiver to waiver key if present
	if raw, ok := euclostate.GetExecutionWaiver(in.State); ok {
		euclostate.SetWaiver(in.State, raw)
	}

	return GateResult{
		Evidence:    evidence,
		SuccessGate: successGate,
	}
}

package euclo

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

type VerificationPolicy struct {
	PolicyID              string   `json:"policy_id"`
	ModeID                string   `json:"mode_id"`
	ProfileID             string   `json:"profile_id"`
	RequiresVerification  bool     `json:"requires_verification"`
	AcceptedStatuses      []string `json:"accepted_statuses,omitempty"`
	RequiresExecutedCheck bool     `json:"requires_executed_check"`
	ManualOutcomeAllowed  bool     `json:"manual_outcome_allowed"`
}

type VerificationCheckRecord struct {
	Name    string `json:"name,omitempty"`
	Command string `json:"command,omitempty"`
	Status  string `json:"status,omitempty"`
	Details string `json:"details,omitempty"`
}

type VerificationEvidence struct {
	Status          string                    `json:"status"`
	Summary         string                    `json:"summary,omitempty"`
	Checks          []VerificationCheckRecord `json:"checks,omitempty"`
	Source          string                    `json:"source,omitempty"`
	EvidencePresent bool                      `json:"evidence_present"`
}

type SuccessGateResult struct {
	Allowed bool     `json:"allowed"`
	Reason  string   `json:"reason,omitempty"`
	Details []string `json:"details,omitempty"`
}

func ResolveVerificationPolicy(mode ModeResolution, profile ExecutionProfileSelection) VerificationPolicy {
	policy := VerificationPolicy{
		PolicyID:              fmt.Sprintf("%s/%s", mode.ModeID, profile.ProfileID),
		ModeID:                mode.ModeID,
		ProfileID:             profile.ProfileID,
		RequiresVerification:  profile.VerificationRequired,
		AcceptedStatuses:      []string{"pass"},
		RequiresExecutedCheck: profile.VerificationRequired,
	}
	switch mode.ModeID {
	case "review", "planning":
		policy.RequiresVerification = false
		policy.RequiresExecutedCheck = false
	case "debug":
		policy.ManualOutcomeAllowed = false
	case "code", "tdd":
		policy.ManualOutcomeAllowed = false
	}
	return policy
}

func NormalizeVerificationEvidence(state *core.Context) VerificationEvidence {
	if state == nil {
		return VerificationEvidence{Status: "not_verified", Source: "absent"}
	}
	if raw, ok := state.Get("pipeline.verify"); ok && raw != nil {
		evidence := verificationEvidenceFromRaw(raw)
		if evidence.Status != "" {
			return evidence
		}
	}
	if summary := strings.TrimSpace(state.GetString("react.verification_latched_summary")); summary != "" {
		return VerificationEvidence{
			Status:          "pass",
			Summary:         summary,
			Source:          "react.verification_latched_summary",
			EvidencePresent: true,
		}
	}
	return VerificationEvidence{Status: "not_verified", Source: "absent"}
}

func EvaluateSuccessGate(policy VerificationPolicy, evidence VerificationEvidence, editRecord *EditExecutionRecord) SuccessGateResult {
	result := SuccessGateResult{Allowed: true}
	if !policy.RequiresVerification {
		result.Reason = "verification_not_required"
		return result
	}
	if !evidence.EvidencePresent {
		return SuccessGateResult{
			Allowed: false,
			Reason:  "verification_missing",
			Details: []string{"required verification evidence was not produced"},
		}
	}
	status := strings.TrimSpace(strings.ToLower(evidence.Status))
	if status == "" {
		status = "not_verified"
	}
	accepted := false
	for _, allowed := range policy.AcceptedStatuses {
		if status == strings.TrimSpace(strings.ToLower(allowed)) {
			accepted = true
			break
		}
	}
	if !accepted {
		if status == "needs_manual_verification" && policy.ManualOutcomeAllowed {
			return SuccessGateResult{Allowed: true, Reason: "manual_verification_allowed"}
		}
		return SuccessGateResult{
			Allowed: false,
			Reason:  "verification_status_rejected",
			Details: []string{"status=" + status},
		}
	}
	if policy.RequiresExecutedCheck {
		executed := false
		for _, check := range evidence.Checks {
			if strings.EqualFold(strings.TrimSpace(check.Status), "pass") {
				executed = true
				break
			}
		}
		if !executed && editRecord != nil && len(editRecord.Executed) > 0 {
			return SuccessGateResult{
				Allowed: false,
				Reason:  "verification_check_missing",
				Details: []string{"verification passed without any passing check record"},
			}
		}
	}
	result.Reason = "verification_accepted"
	return result
}

func verificationEvidenceFromRaw(raw any) VerificationEvidence {
	switch typed := raw.(type) {
	case map[string]any:
		return verificationEvidenceFromMap(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text == "" || text == "<nil>" {
			return VerificationEvidence{Status: "not_verified", Source: "pipeline.verify"}
		}
		return VerificationEvidence{
			Status:          "pass",
			Summary:         text,
			Source:          "pipeline.verify",
			EvidencePresent: true,
		}
	}
}

func verificationEvidenceFromMap(payload map[string]any) VerificationEvidence {
	evidence := VerificationEvidence{
		Status:          strings.TrimSpace(fmt.Sprint(payload["status"])),
		Summary:         strings.TrimSpace(fmt.Sprint(payload["summary"])),
		Source:          "pipeline.verify",
		EvidencePresent: true,
	}
	if evidence.Status == "<nil>" {
		evidence.Status = ""
	}
	if evidence.Summary == "<nil>" {
		evidence.Summary = ""
	}
	switch typed := payload["checks"].(type) {
	case []VerificationCheckRecord:
		evidence.Checks = append([]VerificationCheckRecord{}, typed...)
	case []any:
		checks := make([]VerificationCheckRecord, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			checks = append(checks, VerificationCheckRecord{
				Name:    strings.TrimSpace(fmt.Sprint(entry["name"])),
				Command: strings.TrimSpace(fmt.Sprint(entry["command"])),
				Status:  strings.TrimSpace(fmt.Sprint(entry["status"])),
				Details: strings.TrimSpace(fmt.Sprint(entry["details"])),
			})
		}
		evidence.Checks = checks
	}
	if evidence.Status == "" {
		evidence.Status = "not_verified"
		evidence.EvidencePresent = false
	}
	return evidence
}

package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statebus"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statekeys"
)

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
		if !profile.VerificationRequired {
			policy.RequiresVerification = false
			policy.RequiresExecutedCheck = false
		}
		policy.ManualOutcomeAllowed = false
	case "code", "tdd":
		policy.ManualOutcomeAllowed = false
	}
	return policy
}

func NormalizeVerificationEvidence(state *core.Context) VerificationEvidence {
	if state == nil {
		return VerificationEvidence{Status: "not_verified", Source: "absent", Provenance: VerificationProvenanceAbsent}
	}
	if raw, ok := statebus.GetAny(state, statekeys.KeyPipelineVerify); ok && raw != nil {
		evidence := verificationEvidenceFromRaw(raw)
		if evidence.Status != "" {
			if evidence.RunID == "" {
				evidence.RunID = strings.TrimSpace(state.GetString("euclo.run_id"))
			}
			return evidence
		}
	}
	if summary := strings.TrimSpace(state.GetString("react.verification_latched_summary")); summary != "" {
		return VerificationEvidence{
			Status:          "pass",
			Summary:         summary,
			Source:          "react.verification_latched_summary",
			Provenance:      VerificationProvenanceReused,
			EvidencePresent: true,
			RunID:           strings.TrimSpace(state.GetString("euclo.run_id")),
		}
	}
	return VerificationEvidence{Status: "not_verified", Source: "absent", Provenance: VerificationProvenanceAbsent}
}

func EvaluateSuccessGate(policy VerificationPolicy, evidence VerificationEvidence, editRecord *EditExecutionRecord, state *core.Context) SuccessGateResult {
	result := SuccessGateResult{Allowed: true, AssuranceClass: AssuranceClassVerifiedSuccess}
	if !policy.RequiresVerification {
		result.Reason = "verification_not_required"
		result.AssuranceClass = AssuranceClassUnverifiedSuccess
		return result
	}
	// No mutations were requested or executed — nothing to verify.
	// Gate only applies when the agent actually attempted file changes.
	if editRecord == nil || (len(editRecord.Requested) == 0 && len(editRecord.Executed) == 0) {
		result.Reason = "verification_skipped_no_mutations"
		result.AssuranceClass = AssuranceClassUnverifiedSuccess
		return result
	}
	if !evidence.EvidencePresent {
		return SuccessGateResult{
			Allowed:        false,
			Reason:         "verification_missing",
			Details:        []string{"required verification evidence was not produced"},
			AssuranceClass: AssuranceClassUnverifiedSuccess,
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
			return SuccessGateResult{Allowed: true, Reason: "manual_verification_allowed", AssuranceClass: AssuranceClassOperatorDeferred}
		}
		return SuccessGateResult{
			Allowed:        false,
			Reason:         "verification_status_rejected",
			Details:        []string{"status=" + status},
			AssuranceClass: AssuranceClassUnverifiedSuccess,
		}
	}
	if len(editRecord.Executed) > 0 {
		switch evidence.Provenance {
		case VerificationProvenanceFallback:
			return SuccessGateResult{
				Allowed:        false,
				Reason:         "verification_fallback_rejected",
				Details:        []string{"fallback verification cannot prove fresh edits"},
				AssuranceClass: AssuranceClassUnverifiedSuccess,
			}
		case VerificationProvenanceReused:
			return SuccessGateResult{
				Allowed:        false,
				Reason:         "verification_reused_rejected",
				Details:        []string{"reused verification cannot prove fresh edits"},
				AssuranceClass: AssuranceClassUnverifiedSuccess,
			}
		}
	}
	if policy.RequiresExecutedCheck {
		executed := false
		for _, check := range evidence.Checks {
			if strings.EqualFold(strings.TrimSpace(check.Status), "pass") &&
				(strings.TrimSpace(string(check.Provenance)) == "" || check.Provenance == VerificationProvenanceExecuted) {
				executed = true
				break
			}
		}
		if !executed && len(editRecord.Executed) > 0 {
			return SuccessGateResult{
				Allowed:        false,
				Reason:         "verification_check_missing",
				Details:        []string{"verification passed without any passing executed check record"},
				AssuranceClass: AssuranceClassUnverifiedSuccess,
			}
		}
	}
	if policy.ProfileID == "test_driven_generation" && len(editRecord.Executed) > 0 {
		if !tddEvidencePresent(state, "euclo.tdd.red_evidence", "fail", evidence.RunID) {
			return SuccessGateResult{
				Allowed:        false,
				Reason:         "tdd_red_missing",
				Details:        []string{"TDD completion requires current-run failing red evidence"},
				AssuranceClass: AssuranceClassTDDIncomplete,
			}
		}
		if !tddEvidencePresent(state, "euclo.tdd.green_evidence", "pass", evidence.RunID) {
			return SuccessGateResult{
				Allowed:        false,
				Reason:         "tdd_green_missing",
				Details:        []string{"TDD completion requires current-run passing green evidence"},
				AssuranceClass: AssuranceClassTDDIncomplete,
			}
		}
		if !tddLifecycleSatisfied(state, evidence.RunID) {
			return SuccessGateResult{
				Allowed:        false,
				Reason:         "tdd_lifecycle_incomplete",
				Details:        []string{"TDD completion requires a completed lifecycle artifact for the current run"},
				AssuranceClass: AssuranceClassTDDIncomplete,
			}
		}
		if tddLifecycleRequestedRefactor(state) && !tddEvidencePresent(state, "euclo.tdd.refactor_evidence", "pass", evidence.RunID) {
			return SuccessGateResult{
				Allowed:        false,
				Reason:         "tdd_refactor_missing",
				Details:        []string{"TDD completion requires current-run passing refactor evidence when refactor was requested"},
				AssuranceClass: AssuranceClassTDDIncomplete,
			}
		}
	}
	result.Reason = "verification_accepted"
	return result
}

func DetectAutomaticVerificationDegradation(policy VerificationPolicy, state *core.Context, evidence VerificationEvidence) (string, string, bool) {
	if !policy.RequiresVerification || state == nil {
		return "", "", false
	}
	if evidence.EvidencePresent && evidence.Provenance == VerificationProvenanceExecuted {
		return "", "", false
	}
	raw, ok := statebus.GetAny(state, statekeys.KeyEnvelope)
	if ok && raw != nil {
		if envelope, ok := raw.(TaskEnvelope); ok {
			if !envelope.CapabilitySnapshot.HasExecuteTools && !envelope.CapabilitySnapshot.HasVerificationTools {
				return "automatic", "verification_tools_unavailable", true
			}
		}
		if payload, ok := raw.(map[string]any); ok {
			if snapshot, ok := payload["capability_snapshot"].(map[string]any); ok {
				if !verificationBoolValue(snapshot["has_execute_tools"]) && !verificationBoolValue(snapshot["has_verification_tools"]) {
					return "automatic", "verification_tools_unavailable", true
				}
			}
		}
	}
	if raw, ok := statebus.GetAny(state, statekeys.KeyVerificationPlan); ok && raw != nil {
		if plan, ok := raw.(map[string]any); ok {
			commands, _ := plan["commands"].([]any)
			if len(commands) == 0 {
				return "automatic", "verification_plan_unavailable", true
			}
		}
	}
	return "", "", false
}

func verificationBoolValue(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func tddEvidencePresent(state *core.Context, key, wantStatus, runID string) bool {
	if state == nil {
		return false
	}
	raw, ok := statebus.GetAny(state, key)
	if !ok || raw == nil {
		return false
	}
	record, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(fmt.Sprint(record["status"])))
	if status != strings.TrimSpace(strings.ToLower(wantStatus)) {
		return false
	}
	if strings.TrimSpace(runID) == "" {
		return true
	}
	return strings.TrimSpace(fmt.Sprint(record["run_id"])) == strings.TrimSpace(runID)
}

func tddLifecycleSatisfied(state *core.Context, runID string) bool {
	if state == nil {
		return false
	}
	raw, ok := statebus.GetAny(state, statekeys.KeyTDDLifecycle)
	if !ok || raw == nil {
		return false
	}
	record, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if strings.TrimSpace(strings.ToLower(fmt.Sprint(record["status"]))) != "completed" {
		return false
	}
	if strings.TrimSpace(strings.ToLower(fmt.Sprint(record["current_phase"]))) != "complete" {
		return false
	}
	if strings.TrimSpace(runID) == "" {
		return true
	}
	for _, entry := range phaseHistoryRecords(record["phase_history"]) {
		if strings.TrimSpace(strings.ToLower(fmt.Sprint(entry["phase"]))) != "complete" {
			continue
		}
		entryRunID := strings.TrimSpace(fmt.Sprint(entry["run_id"]))
		if entryRunID == "" || entryRunID == strings.TrimSpace(runID) {
			return true
		}
	}
	return false
}

func tddLifecycleRequestedRefactor(state *core.Context) bool {
	if state == nil {
		return false
	}
	raw, ok := statebus.GetAny(state, statekeys.KeyTDDLifecycle)
	if !ok || raw == nil {
		return false
	}
	record, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	switch typed := record["requested_refactor"].(type) {
	case bool:
		return typed
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		return trimmed == "true" || trimmed == "yes" || trimmed == "1"
	default:
		return false
	}
}

func phaseHistoryRecords(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any{}, typed...)
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			record, ok := item.(map[string]any)
			if ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}

func verificationEvidenceFromRaw(raw any) VerificationEvidence {
	switch typed := raw.(type) {
	case map[string]any:
		return verificationEvidenceFromMap(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text == "" || text == "<nil>" {
			return VerificationEvidence{Status: "not_verified", Source: "pipeline.verify", Provenance: VerificationProvenanceAbsent}
		}
		return VerificationEvidence{
			Status:          "pass",
			Summary:         text,
			Source:          "pipeline.verify",
			Provenance:      VerificationProvenanceFallback,
			EvidencePresent: true,
		}
	}
}

func verificationEvidenceFromMap(payload map[string]any) VerificationEvidence {
	evidence := VerificationEvidence{
		Status:          strings.TrimSpace(fmt.Sprint(payload["status"])),
		Summary:         strings.TrimSpace(fmt.Sprint(payload["summary"])),
		Source:          "pipeline.verify",
		Provenance:      verificationProvenanceFromRaw(payload["provenance"]),
		EvidencePresent: true,
	}
	if evidence.Status == "<nil>" {
		evidence.Status = ""
	}
	if evidence.Summary == "<nil>" {
		evidence.Summary = ""
	}
	if evidence.Provenance == "" {
		evidence.Provenance = inferVerificationProvenance(payload)
	}
	evidence.RunID = strings.TrimSpace(fmt.Sprint(payload["run_id"]))
	if evidence.RunID == "<nil>" {
		evidence.RunID = ""
	}
	if rawTS := strings.TrimSpace(fmt.Sprint(payload["timestamp"])); rawTS != "" && rawTS != "<nil>" {
		if parsed, err := time.Parse(time.RFC3339, rawTS); err == nil {
			evidence.Timestamp = parsed
		}
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
				Name:                  strings.TrimSpace(fmt.Sprint(entry["name"])),
				Command:               strings.TrimSpace(fmt.Sprint(entry["command"])),
				Args:                  stringSliceAnyVerification(entry["args"]),
				WorkingDirectory:      strings.TrimSpace(fmt.Sprint(entry["working_directory"])),
				Status:                strings.TrimSpace(fmt.Sprint(entry["status"])),
				ExitStatus:            intValueAny(entry["exit_status"]),
				DurationMillis:        int64(intValueAny(entry["duration_millis"])),
				FilesUnderCheck:       stringSliceAnyVerification(entry["files_under_check"]),
				ScopeKind:             strings.TrimSpace(fmt.Sprint(entry["scope_kind"])),
				OriginatingCapability: strings.TrimSpace(fmt.Sprint(entry["originating_capability"])),
				RunID:                 strings.TrimSpace(fmt.Sprint(entry["run_id"])),
				Provenance:            verificationProvenanceFromRaw(entry["provenance"]),
				Details:               strings.TrimSpace(fmt.Sprint(entry["details"])),
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

func verificationProvenanceFromRaw(raw any) VerificationProvenanceClass {
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "" || value == "<nil>" {
		return ""
	}
	return VerificationProvenanceClass(strings.ToLower(value))
}

func inferVerificationProvenance(payload map[string]any) VerificationProvenanceClass {
	source := strings.TrimSpace(strings.ToLower(fmt.Sprint(payload["source"])))
	summary := strings.TrimSpace(strings.ToLower(fmt.Sprint(payload["summary"])))
	switch {
	case strings.Contains(source, "fallback") || strings.Contains(summary, "fallback"):
		return VerificationProvenanceFallback
	case strings.Contains(source, "reused") || strings.Contains(summary, "reused"):
		return VerificationProvenanceReused
	default:
		return VerificationProvenanceExecuted
	}
}

func stringSliceAnyVerification(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value := strings.TrimSpace(fmt.Sprint(item))
			if value != "" && value != "<nil>" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func intValueAny(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" || value == "<nil>" {
			return 0
		}
		var out int
		fmt.Sscanf(value, "%d", &out)
		return out
	}
}

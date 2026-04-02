package local

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type failedVerificationRepairCapability struct{ env agentenv.AgentEnvironment }

func NewFailedVerificationRepairCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &failedVerificationRepairCapability{env: env}
}

func (c *failedVerificationRepairCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:repair.failed_verification",
		Name:          "Failed Verification Repair",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "verification", "repair"},
		Annotations: map[string]any{
			"supported_profiles": []string{"edit_verify_repair", "reproduce_localize_patch", "review_suggest_implement", "test_driven_generation"},
		},
	}
}

func (c *failedVerificationRepairCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{
			{Kind: euclotypes.ArtifactKindIntake, Required: true},
			{Kind: euclotypes.ArtifactKindVerification, Required: true},
		},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindEditIntent,
			euclotypes.ArtifactKindVerification,
			euclotypes.ArtifactKindRecoveryTrace,
		},
	}
}

func (c *failedVerificationRepairCapability) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !snapshot.HasWriteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "failed-verification repair requires write tools"}
	}
	if !snapshot.HasExecuteTools {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "failed-verification repair requires execute tools"}
	}
	if !artifacts.Has(euclotypes.ArtifactKindIntake) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "failed-verification repair requires intake", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindIntake}}
	}
	if !artifacts.Has(euclotypes.ArtifactKindVerification) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "failed-verification repair requires verification evidence", MissingArtifacts: []euclotypes.ArtifactKind{euclotypes.ArtifactKindVerification}}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "failed verification can be repaired"}
}

func (c *failedVerificationRepairCapability) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	verifyPayload, ok := verificationPayloadFromState(env.State)
	if !ok || !verificationPayloadFailed(verifyPayload) {
		return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "verification repair not required"}
	}
	if env.Environment.Config == nil || env.Environment.Model == nil {
		return euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "failed verification repair runtime unavailable",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:         "failed_verification_repair_runtime_unavailable",
				Message:      "failed-verification repair requires a configured model runtime",
				Recoverable:  true,
				FailedPhase:  "repair",
				ParadigmUsed: "react",
			},
		}
	}

	trace := map[string]any{
		"kind":                   "failed_verification_repair",
		"status":                 "in_progress",
		"run_id":                 strings.TrimSpace(env.RunID),
		"original_status":        verificationPayloadStatus(verifyPayload),
		"original_summary":       stringValue(verifyPayload["summary"]),
		"failing_checks":         verificationFailingCheckNames(verifyPayload),
		"files_under_check":      verificationFilesUnderCheck(verifyPayload),
		"attempt_limit":          failedVerificationRepairLimit(env.Task),
		"attempt_count":          0,
		"repairable":             true,
		"originating_capability": "euclo:repair.failed_verification",
		"attempts":               []map[string]any{},
	}
	artifacts := []euclotypes.Artifact{buildFailedVerificationRecoveryTraceArtifact(trace)}
	mergeStateArtifactsToContext(env.State, artifacts)

	limit := failedVerificationRepairLimit(env.Task)
	for attempt := 1; attempt <= limit; attempt++ {
		attemptRecord := map[string]any{
			"attempt":       attempt,
			"status":        "in_progress",
			"check_names":   verificationFailingCheckNames(verifyPayload),
			"target_files":  verificationFilesUnderCheck(verifyPayload),
			"verify_status": verificationPayloadStatus(verifyPayload),
		}
		updateFailedVerificationRecoveryTrace(env.State, trace, attemptRecord, "")

		repairTask := &core.Task{
			ID:          firstNonEmpty(taskIdentifier(env.Task), "verification-repair") + fmt.Sprintf("-repair-%d", attempt),
			Instruction: failedVerificationRepairInstruction(env, verifyPayload, attempt, limit),
			Type:        core.TaskTypeCodeModification,
			Context:     taskContextFromEnvelope(env),
		}
		stepEnv := env
		stepEnv.Task = repairTask
		result, state, err := execution.ExecuteEnvelopeRecipe(ctx, stepEnv, execution.RecipeChatImplementEdit, repairTask.ID, repairTask.Instruction)
		if err != nil || result == nil || !result.Success {
			attemptRecord["status"] = "edit_failed"
			attemptRecord["error"] = errMsg(err, result)
			updateFailedVerificationRecoveryTrace(env.State, trace, attemptRecord, "repair edit failed")
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusFailed,
				Summary:   "failed verification repair could not produce a patch",
				Artifacts: []euclotypes.Artifact{buildFailedVerificationRecoveryTraceArtifact(trace)},
				FailureInfo: &euclotypes.CapabilityFailure{
					Code:         "failed_verification_repair_edit_failed",
					Message:      errMsg(err, result),
					Recoverable:  true,
					FailedPhase:  "repair",
					ParadigmUsed: "react",
				},
			}
		}
		execution.PropagateBehaviorTrace(env.State, state)
		editPayload := firstNonNilMap(result.Data, map[string]any{"summary": resultSummary(result), "attempt": attempt})
		editArtifact := euclotypes.Artifact{
			ID:         fmt.Sprintf("failed_verification_repair_edit_%d", attempt),
			Kind:       euclotypes.ArtifactKindEditIntent,
			Summary:    firstNonEmpty(resultSummary(result), fmt.Sprintf("repair attempt %d", attempt)),
			Payload:    editPayload,
			ProducerID: "euclo:repair.failed_verification",
			Status:     "produced",
		}
		artifacts = append(artifacts, editArtifact)
		mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{editArtifact})

		verificationArtifacts, executed, execErr := ExecuteVerificationFlow(ctx, env, eucloruntime.SnapshotCapabilities(env.Registry))
		if execErr != nil {
			attemptRecord["status"] = "verification_error"
			attemptRecord["error"] = execErr.Error()
			updateFailedVerificationRecoveryTrace(env.State, trace, attemptRecord, "verification rerun failed")
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusFailed,
				Summary:   "failed verification repair could not rerun verification",
				Artifacts: append(artifacts, buildFailedVerificationRecoveryTraceArtifact(trace)),
				FailureInfo: &euclotypes.CapabilityFailure{
					Code:         "failed_verification_repair_verify_failed",
					Message:      execErr.Error(),
					Recoverable:  true,
					FailedPhase:  "verify",
					ParadigmUsed: "react",
				},
			}
		}
		if executed {
			artifacts = append(artifacts, verificationArtifacts...)
		}
		verifyPayload, ok = verificationPayloadFromState(env.State)
		if !ok {
			attemptRecord["status"] = "verification_missing"
			updateFailedVerificationRecoveryTrace(env.State, trace, attemptRecord, "verification evidence missing after repair attempt")
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusFailed,
				Summary:   "failed verification repair did not produce verification evidence",
				Artifacts: append(artifacts, buildFailedVerificationRecoveryTraceArtifact(trace)),
				FailureInfo: &euclotypes.CapabilityFailure{
					Code:         "failed_verification_repair_missing_verification",
					Message:      "verification evidence missing after repair attempt",
					Recoverable:  true,
					FailedPhase:  "verify",
					ParadigmUsed: "react",
				},
			}
		}
		attemptRecord["verify_status"] = verificationPayloadStatus(verifyPayload)
		attemptRecord["verify_summary"] = stringValue(verifyPayload["summary"])
		if verificationPayloadPassed(verifyPayload) {
			attemptRecord["status"] = "repaired"
			updateFailedVerificationRecoveryTrace(env.State, trace, attemptRecord, "verification repaired")
			finalTrace := buildFailedVerificationRecoveryTraceArtifact(trace)
			artifacts = append(artifacts, finalTrace)
			mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{finalTrace})
			return euclotypes.ExecutionResult{
				Status:    euclotypes.ExecutionStatusCompleted,
				Summary:   "failed verification repaired and revalidated",
				Artifacts: artifacts,
			}
		}
		attemptRecord["status"] = "verification_failed"
		attemptRecord["failing_checks"] = verificationFailingCheckNames(verifyPayload)
		updateFailedVerificationRecoveryTrace(env.State, trace, attemptRecord, "verification still failing")
	}

	trace["status"] = "repair_exhausted"
	trace["summary"] = "verification remained failing after bounded repair attempts"
	trace["attempt_count"] = len(attemptMaps(trace["attempts"]))
	finalTrace := buildFailedVerificationRecoveryTraceArtifact(trace)
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{finalTrace})
	return euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusFailed,
		Summary:   "failed verification repair exhausted its bounded attempts",
		Artifacts: append(artifacts, finalTrace),
		FailureInfo: &euclotypes.CapabilityFailure{
			Code:         "failed_verification_repair_exhausted",
			Message:      "verification remained failing after bounded repair attempts",
			Recoverable:  true,
			FailedPhase:  "repair",
			ParadigmUsed: "react",
		},
	}
}

func verificationPayloadFromState(state *core.Context) (map[string]any, bool) {
	if state == nil {
		return nil, false
	}
	raw, ok := state.Get("pipeline.verify")
	if !ok || raw == nil {
		return nil, false
	}
	payload, ok := raw.(map[string]any)
	return payload, ok
}

func verificationPayloadStatus(payload map[string]any) string {
	return strings.ToLower(strings.TrimSpace(stringValue(payload["status"])))
}

func verificationPayloadFailed(payload map[string]any) bool {
	return verificationPayloadStatus(payload) == "fail"
}

func VerificationPayloadFailed(payload map[string]any) bool {
	return verificationPayloadFailed(payload)
}

func verificationPayloadPassed(payload map[string]any) bool {
	return verificationPayloadStatus(payload) == "pass"
}

func verificationFailingCheckNames(payload map[string]any) []string {
	checks, _ := payload["checks"].([]map[string]any)
	if len(checks) == 0 {
		if generic, ok := payload["checks"].([]any); ok {
			checks = mapsFromAnySlice(generic)
		}
	}
	names := make([]string, 0, len(checks))
	for _, check := range checks {
		if strings.EqualFold(strings.TrimSpace(stringValue(check["status"])), "fail") {
			names = append(names, firstNonEmpty(stringValue(check["name"]), stringValue(check["command"])))
		}
	}
	return uniqueStrings(names)
}

func verificationFilesUnderCheck(payload map[string]any) []string {
	checks, _ := payload["checks"].([]map[string]any)
	if len(checks) == 0 {
		if generic, ok := payload["checks"].([]any); ok {
			checks = mapsFromAnySlice(generic)
		}
	}
	files := uniqueStringsFromAny(payload["files"])
	for _, check := range checks {
		files = append(files, uniqueStringsFromAny(check["files_under_check"])...)
	}
	return uniqueStrings(files)
}

func mapsFromAnySlice(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			out = append(out, record)
		}
	}
	return out
}

func failedVerificationRepairLimit(task *core.Task) int {
	if task != nil && task.Context != nil {
		for _, key := range []string{"verification_repair_attempt_limit", "repair_attempt_limit"} {
			switch typed := task.Context[key].(type) {
			case int:
				if typed > 0 {
					return typed
				}
			case float64:
				if int(typed) > 0 {
					return int(typed)
				}
			case string:
				if strings.TrimSpace(typed) == "1" {
					return 1
				}
				if strings.TrimSpace(typed) == "3" {
					return 3
				}
			}
		}
	}
	return 2
}

func failedVerificationRepairInstruction(env euclotypes.ExecutionEnvelope, verifyPayload map[string]any, attempt, limit int) string {
	checkNames := verificationFailingCheckNames(verifyPayload)
	files := verificationFilesUnderCheck(verifyPayload)
	parts := []string{
		"Repair the implementation so the current failing verification scope passes.",
		"Original request: " + taskInstruction(env.Task),
		fmt.Sprintf("Repair attempt %d of %d.", attempt, limit),
	}
	if len(checkNames) > 0 {
		parts = append(parts, "Failing checks: "+strings.Join(checkNames, ", "))
	}
	if len(files) > 0 {
		parts = append(parts, "Focus first on the touched or verified files: "+strings.Join(files, ", "))
	}
	if summary := strings.TrimSpace(stringValue(verifyPayload["summary"])); summary != "" {
		parts = append(parts, "Verification summary: "+summary)
	}
	parts = append(parts, "Keep the fix bounded to the failing scope and preserve existing intended behavior.")
	return strings.Join(parts, "\n")
}

func buildFailedVerificationRecoveryTraceArtifact(trace map[string]any) euclotypes.Artifact {
	return euclotypes.Artifact{
		ID:         "failed_verification_recovery_trace",
		Kind:       euclotypes.ArtifactKindRecoveryTrace,
		Summary:    firstNonEmpty(stringValue(trace["summary"]), "failed verification repair trace"),
		Payload:    trace,
		ProducerID: "euclo:repair.failed_verification",
		Status:     "produced",
	}
}

func updateFailedVerificationRecoveryTrace(state *core.Context, trace map[string]any, attempt map[string]any, summary string) {
	attempts := attemptMaps(trace["attempts"])
	replaced := false
	for i := range attempts {
		if stringValue(attempts[i]["attempt"]) == stringValue(attempt["attempt"]) {
			attempts[i] = attempt
			replaced = true
			break
		}
	}
	if !replaced {
		attempts = append(attempts, attempt)
	}
	trace["attempts"] = attempts
	trace["attempt_count"] = len(attempts)
	trace["summary"] = summary
	if strings.EqualFold(stringValue(attempt["status"]), "repaired") {
		trace["status"] = "repaired"
	} else if trace["status"] == nil || strings.TrimSpace(stringValue(trace["status"])) == "" {
		trace["status"] = "in_progress"
	}
	if state != nil {
		state.Set("euclo.recovery_trace", trace)
	}
}

func attemptMaps(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...)
	case []any:
		return mapsFromAnySlice(typed)
	default:
		return nil
	}
}

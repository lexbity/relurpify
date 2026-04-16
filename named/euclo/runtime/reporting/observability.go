package reporting

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func BuildActionLog(state *core.Context, artifacts []euclotypes.Artifact) []eucloruntime.ActionLogEntry {
	now := time.Now().UTC()
	log := make([]eucloruntime.ActionLogEntry, 0, 8)
	appendEntry := func(kind, message string, metadata map[string]any) {
		log = append(log, eucloruntime.ActionLogEntry{
			Kind:      kind,
			Message:   message,
			Timestamp: now,
			Metadata:  metadata,
		})
	}
	if raw, ok := state.Get("euclo.mode_resolution"); ok && raw != nil {
		appendEntry("mode_resolution", "resolved execution mode", map[string]any{"payload": raw})
	}
	if raw, ok := state.Get("euclo.execution_profile_selection"); ok && raw != nil {
		appendEntry("execution_profile", "selected execution profile", map[string]any{"payload": raw})
	}
	if raw, ok := state.Get("euclo.retrieval_policy"); ok && raw != nil {
		appendEntry("retrieval_policy", "resolved retrieval policy", map[string]any{"payload": raw})
	}
	if raw, ok := state.Get("euclo.context_expansion"); ok && raw != nil {
		appendEntry("context_expansion", "expanded context for execution", map[string]any{"payload": raw})
	}
	if raw, ok := state.Get("euclo.profile_controller"); ok && raw != nil {
		appendEntry("profile_controller", "profile controller execution", map[string]any{"payload": raw})
	}
	if raw, ok := state.Get("euclo.verification"); ok && raw != nil {
		appendEntry("verification", "normalized verification evidence", map[string]any{"payload": raw})
	}
	if raw, ok := state.Get("euclo.success_gate"); ok && raw != nil {
		appendEntry("success_gate", "evaluated completion gate", map[string]any{"payload": raw})
	}
	if raw, ok := state.Get("euclo.recovery_trace"); ok && raw != nil {
		appendEntry("recovery_trace", "recovery stack trace", map[string]any{"payload": raw})
	}
	if len(artifacts) > 0 {
		kinds := make([]string, 0, len(artifacts))
		for _, artifact := range artifacts {
			kinds = append(kinds, string(artifact.Kind))
		}
		appendEntry("artifacts", "assembled euclo artifacts", map[string]any{"kinds": kinds})
	}
	return log
}

func BuildProofSurface(state *core.Context, artifacts []euclotypes.Artifact) eucloruntime.ProofSurface {
	proof := eucloruntime.ProofSurface{}
	if raw, ok := state.Get("euclo.mode_resolution"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.ModeResolution); ok {
			proof.ModeID = typed.ModeID
		}
	}
	if raw, ok := state.Get("euclo.execution_profile_selection"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.ExecutionProfileSelection); ok {
			proof.ProfileID = typed.ProfileID
		}
	}
	if raw, ok := state.Get("euclo.mode_resolution"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.ModeResolution); ok {
			proof.PrimaryFamilyID = primaryFamilyForMode(typed.ModeID)
		}
	}
	if raw, ok := state.Get("euclo.verification"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.VerificationEvidence); ok {
			proof.VerificationStatus = typed.Status
			proof.VerificationProvenance = string(typed.Provenance)
		}
	}
	if raw, ok := state.Get("euclo.success_gate"); ok && raw != nil {
		if typed, ok := raw.(eucloruntime.SuccessGateResult); ok {
			proof.SuccessGateReason = typed.Reason
			proof.AssuranceClass = string(typed.AssuranceClass)
			proof.WaiverApplied = typed.WaiverApplied
			proof.DegradationMode = typed.DegradationMode
			proof.DegradationReason = typed.DegradationReason
		}
	}
	if raw, ok := state.Get("euclo.profile_controller"); ok && raw != nil {
		if typed, ok := raw.(map[string]any); ok {
			if ids, ok := typed["capability_ids"].([]string); ok {
				proof.CapabilityIDs = ids
			}
			if count, ok := typed["gate_evals_count"].(int); ok {
				proof.GateEvalsCount = count
			}
			if phases, ok := typed["phases_executed"].([]string); ok {
				proof.PhasesExecuted = phases
			}
			if count, ok := typed["recovery_attempts"].(int); ok {
				proof.RecoveryAttempts = count
			}
		}
	}
	if raw, ok := state.Get("euclo.recovery_trace"); ok && raw != nil {
		if typed, ok := raw.(map[string]any); ok {
			proof.RecoveryStatus = proofStringValue(typed["status"])
			if count, ok := typed["attempt_count"].(int); ok {
				proof.RecoveryAttempts = count
			} else if attempts, ok := typed["attempts"].([]map[string]any); ok {
				proof.RecoveryAttempts = len(attempts)
			} else if attempts, ok := typed["attempts"].([]any); ok {
				proof.RecoveryAttempts = len(attempts)
			}
		}
	}
	proof.ArtifactKinds = make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		proof.ArtifactKinds = append(proof.ArtifactKinds, string(artifact.Kind))
		if artifact.Kind == euclotypes.ArtifactKindWorkflowRetrieval {
			proof.WorkflowRetrievalUsed = true
		}
	}
	return proof
}

func EmitObservabilityTelemetry(telemetry core.Telemetry, task *core.Task, log []eucloruntime.ActionLogEntry, proof eucloruntime.ProofSurface) {
	if telemetry == nil {
		return
	}
	taskID := ""
	if task != nil {
		taskID = task.ID
	}
	for _, entry := range log {
		telemetry.Emit(core.Event{
			Type:      core.EventStateChange,
			TaskID:    taskID,
			Message:   entry.Message,
			Timestamp: entry.Timestamp,
			Metadata: map[string]any{
				"kind":     entry.Kind,
				"metadata": entry.Metadata,
			},
		})
	}
	telemetry.Emit(core.Event{
		Type:      core.EventAgentFinish,
		TaskID:    taskID,
		Message:   "euclo proof surface",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"mode_id":                 proof.ModeID,
			"profile_id":              proof.ProfileID,
			"primary_family_id":       proof.PrimaryFamilyID,
			"verification_status":     proof.VerificationStatus,
			"verification_provenance": proof.VerificationProvenance,
			"recovery_status":         proof.RecoveryStatus,
			"success_gate_reason":     proof.SuccessGateReason,
			"assurance_class":         proof.AssuranceClass,
			"degradation_mode":        proof.DegradationMode,
			"degradation_reason":      proof.DegradationReason,
			"waiver_applied":          proof.WaiverApplied,
			"artifact_kinds":          proof.ArtifactKinds,
			"workflow_retrieval_used": proof.WorkflowRetrievalUsed,
		},
	})
}

func proofStringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func primaryFamilyForMode(mode string) string {
	switch mode {
	case "debug":
		return "debugging"
	case "review":
		return "review"
	case "planning":
		return "planning"
	case "tdd":
		return "implementation"
	case "chat":
		return "chat"
	default:
		return mode
	}
}

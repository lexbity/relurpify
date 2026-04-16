package reporting

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
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
	if raw, ok := euclostate.GetModeResolution(state); ok {
		appendEntry("mode_resolution", "resolved execution mode", map[string]any{"payload": raw})
	}
	if raw, ok := euclostate.GetExecutionProfileSelection(state); ok {
		appendEntry("execution_profile", "selected execution profile", map[string]any{"payload": raw})
	}
	if raw, ok := euclostate.GetRetrievalPolicy(state); ok {
		appendEntry("retrieval_policy", "resolved retrieval policy", map[string]any{"payload": raw})
	}
	if raw, ok := euclostate.GetContextExpansion(state); ok && raw != nil {
		appendEntry("context_expansion", "expanded context for execution", map[string]any{"payload": raw})
	}
	if raw, ok := euclostate.GetProfileController(state); ok && raw != nil {
		appendEntry("profile_controller", "profile controller execution", map[string]any{"payload": raw})
	}
	if raw, ok := euclostate.GetVerification(state); ok {
		appendEntry("verification", "normalized verification evidence", map[string]any{"payload": raw})
	}
	if raw, ok := euclostate.GetSuccessGate(state); ok {
		appendEntry("success_gate", "evaluated completion gate", map[string]any{"payload": raw})
	}
	if raw, ok := euclostate.GetRecoveryTrace(state); ok {
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
	if raw, ok := euclostate.GetModeResolution(state); ok {
		proof.ModeID = raw.ModeID
	}
	if raw, ok := euclostate.GetExecutionProfileSelection(state); ok {
		proof.ProfileID = raw.ProfileID
	}
	if raw, ok := euclostate.GetModeResolution(state); ok {
		proof.PrimaryFamilyID = primaryFamilyForMode(raw.ModeID)
	}
	if raw, ok := euclostate.GetVerification(state); ok {
		proof.VerificationStatus = raw.Status
		proof.VerificationProvenance = string(raw.Provenance)
	}
	if raw, ok := euclostate.GetSuccessGate(state); ok {
		proof.SuccessGateReason = raw.Reason
		proof.AssuranceClass = string(raw.AssuranceClass)
		proof.WaiverApplied = raw.WaiverApplied
		proof.DegradationMode = raw.DegradationMode
		proof.DegradationReason = raw.DegradationReason
	}
	if raw, ok := euclostate.GetProfileController(state); ok {
		if ids, ok := raw["capability_ids"].([]string); ok {
			proof.CapabilityIDs = ids
		}
		if count, ok := raw["gate_evals_count"].(int); ok {
			proof.GateEvalsCount = count
		}
		if phases, ok := raw["phases_executed"].([]string); ok {
			proof.PhasesExecuted = phases
		}
		if count, ok := raw["recovery_attempts"].(int); ok {
			proof.RecoveryAttempts = count
		}
	}
	if raw, ok := euclostate.GetRecoveryTrace(state); ok {
		proof.RecoveryStatus = raw.Status
		if raw.AttemptCount > 0 {
			proof.RecoveryAttempts = raw.AttemptCount
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

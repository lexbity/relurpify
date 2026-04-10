package orchestrate

import (
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// orchestrateRecorder owns the package's observability and persistence writes.
// It keeps the state-key and payload shaping in one place while the execution
// paths stay focused on control flow.
type orchestrateRecorder struct{}

var defaultOrchestrateRecorder = orchestrateRecorder{}

func (orchestrateRecorder) recordProfileControllerObservability(
	state *core.Context,
	pcResult *ProfileControllerResult,
	mode euclotypes.ModeResolution,
	profile euclotypes.ExecutionProfileSelection,
) {
	if state == nil || pcResult == nil {
		return
	}
	phaseRecords := defaultOrchestrateRecorder.profilePhaseRecordsState(pcResult.PhaseRecords)
	state.Set("euclo.profile_controller", map[string]any{
		"mode_id":           mode.ModeID,
		"profile_id":        profile.ProfileID,
		"capability_ids":    pcResult.CapabilityIDs,
		"phases_executed":   pcResult.PhasesExecuted,
		"phase_records":     phaseRecords,
		"early_stop":        pcResult.EarlyStop,
		"early_stop_phase":  pcResult.EarlyStopPhase,
		"gate_evals_count":  len(pcResult.GateEvals),
		"recovery_attempts": pcResult.RecoveryAttempts,
	})
	state.Set("euclo.profile_phase_records", phaseRecords)
}

func (orchestrateRecorder) profilePhaseRecordsState(records []PhaseArtifactRecord) []map[string]any {
	if len(records) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		entry := map[string]any{
			"phase":              record.Phase,
			"artifacts_consumed": artifactKindsToStrings(record.ArtifactsConsumed),
			"artifacts_produced": artifactKindsToStrings(artifactKindsFromArtifacts(record.ArtifactsProduced)),
		}
		if len(record.ArtifactsProduced) > 0 {
			produced := make([]map[string]any, 0, len(record.ArtifactsProduced))
			for _, artifact := range record.ArtifactsProduced {
				produced = append(produced, map[string]any{
					"kind":    string(artifact.Kind),
					"summary": artifact.Summary,
					"payload": artifact.Payload,
				})
			}
			entry["produced_artifacts"] = produced
		}
		out = append(out, entry)
	}
	return out
}

func (orchestrateRecorder) persistInteractiveState(
	env euclotypes.ExecutionEnvelope,
	machine *interaction.PhaseMachine,
	recordingEmitter *interaction.RecordingEmitter,
	iResult interaction.InteractionResult,
) {
	if env.State == nil {
		return
	}
	mergeCapabilityArtifactsToState(env.State, iResult.Artifacts)

	iState := interaction.ExtractInteractionState(machine)
	iState.PhasesExecuted = append([]string{}, iResult.PhasesExecuted...)
	env.State.Set("euclo.interaction_state", iState)
	if recordingEmitter != nil && recordingEmitter.Recording != nil {
		env.State.Set("euclo.interaction_recording", recordingEmitter.Recording.ToStateMap())
		env.State.Set("euclo.interaction_records", recordingEmitter.Recording.Records())
	}
	if machine != nil {
		if raw, ok := machine.State()["propose.items"]; ok && raw != nil {
			env.State.Set("pipeline.plan", map[string]any{
				"source": "interaction.propose",
				"items":  raw,
			})
		}
	}
}

func (orchestrateRecorder) recoveryTraceArtifact(stack *RecoveryStack, producerID string) euclotypes.Artifact {
	if stack == nil {
		stack = &RecoveryStack{}
	}
	attempts := make([]map[string]any, 0, len(stack.Attempts))
	for _, a := range stack.Attempts {
		attempts = append(attempts, map[string]any{
			"level":    string(a.Level),
			"strategy": string(a.Strategy),
			"from":     a.From,
			"to":       a.To,
			"reason":   a.Reason,
			"success":  a.Success,
		})
	}
	return euclotypes.Artifact{
		ID:         "recovery_trace",
		Kind:       euclotypes.ArtifactKindRecoveryTrace,
		Summary:    fmt.Sprintf("%d recovery attempts, exhausted=%v", len(stack.Attempts), stack.Exhausted),
		ProducerID: producerID,
		Status:     "produced",
		Payload: map[string]any{
			"attempts":  attempts,
			"max_depth": stack.MaxDepth,
			"exhausted": stack.Exhausted,
		},
	}
}

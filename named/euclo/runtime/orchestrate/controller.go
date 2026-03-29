package orchestrate

import (
	"context"
	"fmt"
	"sort"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction/gate"
)

// ProfileController replaces the flat buildExecutorForRouting dispatch with
// phase-by-phase capability execution. It evaluates evidence gates between
// phases and selects capabilities based on contract alignment.
type ProfileController struct {
	// Use interface{} instead of concrete types to avoid circular imports.
	// The root euclo package provides properly typed values at construction.
	Capabilities    CapabilityRegistryI
	Gates           map[string][]gate.PhaseGate
	Environment     agentenv.AgentEnvironment
	ProfileRegistry *euclotypes.ExecutionProfileRegistry
	Recovery        *RecoveryController
}

// NewProfileController creates a ProfileController with the given registries.
func NewProfileController(
	caps CapabilityRegistryI,
	gates map[string][]gate.PhaseGate,
	env agentenv.AgentEnvironment,
	profiles *euclotypes.ExecutionProfileRegistry,
	recovery *RecoveryController,
) *ProfileController {
	return &ProfileController{
		Capabilities:    caps,
		Gates:           gates,
		Environment:     env,
		ProfileRegistry: profiles,
		Recovery:        recovery,
	}
}

// ProfileControllerResult captures the detailed output of profile execution,
// including per-phase gate evaluations and capability results.
type ProfileControllerResult struct {
	Result           *core.Result
	Artifacts        []euclotypes.Artifact
	GateEvals        []gate.GateEvaluation
	CapabilityIDs    []string
	PhasesExecuted   []string
	PhaseRecords     []PhaseArtifactRecord
	EarlyStop        bool
	EarlyStopPhase   string
	RecoveryAttempts int
}

type PhaseArtifactRecord struct {
	Phase             string
	ArtifactsProduced []euclotypes.Artifact
	ArtifactsConsumed []euclotypes.ArtifactKind
}

// ExecuteProfile runs the profile's phases in order, evaluating evidence gates
// between transitions and dispatching to matched capabilities.
func (pc *ProfileController) ExecuteProfile(
	ctx context.Context,
	profile euclotypes.ExecutionProfileSelection,
	mode euclotypes.ModeResolution,
	env euclotypes.ExecutionEnvelope,
) (*core.Result, *ProfileControllerResult, error) {
	phases := OrderedPhases(profile.PhaseRoutes, pc.Gates[profile.ProfileID])
	gates := pc.Gates[profile.ProfileID]
	artifacts := euclotypes.ArtifactStateFromContext(env.State)
	snapshot := snapshotFromEnv(env)

	pcResult := &ProfileControllerResult{}
	recoveryStack := NewRecoveryStack()

	// Try to find a profile-level capability first. Profile-level capabilities
	// handle all phases internally, so we skip inter-phase gates before
	// execution and only evaluate gates post-execution for verification.
	profileCap := pc.resolveProfileCapability(profile.ProfileID, artifacts, snapshot)
	if profileCap != nil {
		initialConsumed := artifactKindsFromState(artifacts)
		capResult := profileCap.Execute(ctx, env)
		capID := profileCap.Descriptor().ID
		pcResult.CapabilityIDs = append(pcResult.CapabilityIDs, capID)
		pcResult.PhasesExecuted = phases

		for _, art := range capResult.Artifacts {
			pcResult.Artifacts = append(pcResult.Artifacts, art)
		}
		mergeCapabilityArtifactsToState(env.State, capResult.Artifacts)
		artifacts = euclotypes.ArtifactStateFromContext(env.State)

		// Evaluate all gates post-execution to verify artifact coverage.
		for _, g := range gates {
			eval := gate.EvaluateGate(g, mode.ModeID, artifacts)
			pcResult.GateEvals = append(pcResult.GateEvals, eval)
		}

		if shouldAttemptRecovery(capResult) && pc.Recovery != nil {
			recoveredResult := pc.Recovery.AttemptRecovery(ctx, *capResult.RecoveryHint, capResult, env, recoveryStack)
			if recoveredResult.Status != euclotypes.ExecutionStatusFailed {
				capResult = recoveredResult
				for _, art := range recoveredResult.Artifacts {
					pcResult.Artifacts = append(pcResult.Artifacts, art)
				}
				mergeCapabilityArtifactsToState(env.State, recoveredResult.Artifacts)
			}
		}

		// Record recovery trace if any attempts were made.
		if len(recoveryStack.Attempts) > 0 {
			traceArt := RecoveryTraceArtifact(recoveryStack, "euclo:profile_controller")
			pcResult.Artifacts = append(pcResult.Artifacts, traceArt)
			pcResult.RecoveryAttempts = len(recoveryStack.Attempts)
			if traceArt.Payload != nil {
				env.State.Set("euclo.recovery_trace", traceArt.Payload)
			}
		}
		pcResult.PhaseRecords = buildProfileCapabilityPhaseRecords(phases, initialConsumed, artifacts.All())

		recordProfileControllerObservability(env.State, pcResult, mode, profile)

		if capResult.Status != euclotypes.ExecutionStatusCompleted {
			return failedResult(env.Task, capResult, pcResult), pcResult,
				fmt.Errorf("capability %s failed: %s", capID, capResult.Summary)
		}

		return successResult(env.Task, capResult, pcResult), pcResult, nil
	}

	// Phase-by-phase execution: iterate through phases, evaluating gates
	// at each transition and dispatching to per-phase capabilities.
	gateIndex := 0
	for i, phase := range phases {
		// Evaluate entrance gate for this phase transition.
		if gateIndex < len(gates) && i > 0 {
			g := gates[gateIndex]
			eval := gate.EvaluateGate(g, mode.ModeID, artifacts)
			pcResult.GateEvals = append(pcResult.GateEvals, eval)
			if !eval.Passed {
				switch eval.Policy {
				case gate.GateFailBlock:
					pcResult.EarlyStop = true
					pcResult.EarlyStopPhase = phase
					recordProfileControllerObservability(env.State, pcResult, mode, profile)
					return partialResult(env.Task, pcResult), pcResult, nil
				case gate.GateFailWarn:
					// continue execution
				case gate.GateFailSkip:
					gateIndex++
					continue
				}
			}
			gateIndex++
		}

		// Find a capability for this phase.
		phaseCap := pc.resolveCapabilityForPhase(phase, profile.ProfileID, artifacts, snapshot)
		if phaseCap == nil {
			// No capability for this phase — skip it.
			continue
		}

		// Execute the phase capability.
		consumedKinds := artifactKindsFromState(artifacts)
		capResult := phaseCap.Execute(ctx, env)
		capID := phaseCap.Descriptor().ID
		pcResult.CapabilityIDs = append(pcResult.CapabilityIDs, capID)
		pcResult.PhasesExecuted = append(pcResult.PhasesExecuted, phase)
		pcResult.PhaseRecords = append(pcResult.PhaseRecords, PhaseArtifactRecord{
			Phase:             phase,
			ArtifactsProduced: append([]euclotypes.Artifact{}, capResult.Artifacts...),
			ArtifactsConsumed: consumedKinds,
		})

		for _, art := range capResult.Artifacts {
			pcResult.Artifacts = append(pcResult.Artifacts, art)
		}
		mergeCapabilityArtifactsToState(env.State, capResult.Artifacts)
		artifacts = euclotypes.ArtifactStateFromContext(env.State)

		if capResult.Status != euclotypes.ExecutionStatusCompleted {
			recovered := false

			// First: try recovery controller if hint is available.
			if shouldAttemptRecovery(capResult) && pc.Recovery != nil && recoveryStack.CanAttempt() {
				recoveredResult := pc.Recovery.AttemptRecovery(ctx, *capResult.RecoveryHint, capResult, env, recoveryStack)
				if recoveredResult.Status == euclotypes.ExecutionStatusCompleted {
					for _, art := range recoveredResult.Artifacts {
						pcResult.Artifacts = append(pcResult.Artifacts, art)
					}
					mergeCapabilityArtifactsToState(env.State, recoveredResult.Artifacts)
					artifacts = euclotypes.ArtifactStateFromContext(env.State)
					recovered = true
				}
			}

			// Second: attempt single-level fallback via alternate capability.
			if !recovered {
				fallbackCap := pc.resolveFallbackCapability(
					phase, profile.ProfileID, capID, artifacts, snapshot,
				)
				if fallbackCap != nil {
					fallbackResult := fallbackCap.Execute(ctx, env)
					fallbackID := fallbackCap.Descriptor().ID
					pcResult.CapabilityIDs = append(pcResult.CapabilityIDs, fallbackID)
					for _, art := range fallbackResult.Artifacts {
						pcResult.Artifacts = append(pcResult.Artifacts, art)
					}
					mergeCapabilityArtifactsToState(env.State, fallbackResult.Artifacts)
					artifacts = euclotypes.ArtifactStateFromContext(env.State)
					if fallbackResult.Status != euclotypes.ExecutionStatusFailed {
						recovered = true
					}
				}
			}

			if !recovered {
				pcResult.EarlyStop = true
				pcResult.EarlyStopPhase = phase
				if len(recoveryStack.Attempts) > 0 {
					traceArt := RecoveryTraceArtifact(recoveryStack, "euclo:profile_controller")
					pcResult.Artifacts = append(pcResult.Artifacts, traceArt)
					pcResult.RecoveryAttempts = len(recoveryStack.Attempts)
					if traceArt.Payload != nil {
						env.State.Set("euclo.recovery_trace", traceArt.Payload)
					}
				}
				recordProfileControllerObservability(env.State, pcResult, mode, profile)
				return failedResult(env.Task, capResult, pcResult), pcResult,
					fmt.Errorf("capability %s failed at phase %s: %s", capID, phase, capResult.Summary)
			}
		}
	}

	if len(recoveryStack.Attempts) > 0 {
		traceArt := RecoveryTraceArtifact(recoveryStack, "euclo:profile_controller")
		pcResult.Artifacts = append(pcResult.Artifacts, traceArt)
		pcResult.RecoveryAttempts = len(recoveryStack.Attempts)
		if traceArt.Payload != nil {
			env.State.Set("euclo.recovery_trace", traceArt.Payload)
		}
	}
	recordProfileControllerObservability(env.State, pcResult, mode, profile)
	return completedResult(env.Task, pcResult), pcResult, nil
}

func shouldAttemptRecovery(result euclotypes.ExecutionResult) bool {
	return result.Status != euclotypes.ExecutionStatusCompleted && result.RecoveryHint != nil
}

// resolveProfileCapability finds a capability that supports the entire profile.
func (pc *ProfileController) resolveProfileCapability(
	profileID string,
	artifacts euclotypes.ArtifactState,
	snapshot euclotypes.CapabilitySnapshot,
) CapabilityI {
	if pc.Capabilities == nil {
		return nil
	}
	candidates := pc.Capabilities.ForProfile(profileID)
	for _, cap := range candidates {
		if result := cap.Eligible(artifacts, snapshot); result.Eligible {
			return cap
		}
	}
	return nil
}

// resolveCapabilityForPhase finds a capability whose contract produces
// the artifact expected for the given phase.
func (pc *ProfileController) resolveCapabilityForPhase(
	phase, profileID string,
	artifacts euclotypes.ArtifactState,
	snapshot euclotypes.CapabilitySnapshot,
) CapabilityI {
	if pc.Capabilities == nil {
		return nil
	}
	expectedKind := phaseExpectedArtifact(phase)
	if expectedKind == "" {
		return nil
	}
	candidates := pc.Capabilities.ForProfile(profileID)
	for _, cap := range candidates {
		if result := cap.Eligible(artifacts, snapshot); !result.Eligible {
			continue
		}
		contract := cap.Contract()
		for _, output := range contract.ProducedOutputs {
			if output == expectedKind {
				return cap
			}
		}
	}
	return nil
}

// resolveFallbackCapability finds an alternative eligible capability for a
// phase, excluding the one that already failed.
func (pc *ProfileController) resolveFallbackCapability(
	phase, profileID, excludeID string,
	artifacts euclotypes.ArtifactState,
	snapshot euclotypes.CapabilitySnapshot,
) CapabilityI {
	if pc.Capabilities == nil {
		return nil
	}
	expectedKind := phaseExpectedArtifact(phase)
	if expectedKind == "" {
		return nil
	}
	candidates := pc.Capabilities.ForProfile(profileID)
	for _, cap := range candidates {
		if cap.Descriptor().ID == excludeID {
			continue
		}
		if result := cap.Eligible(artifacts, snapshot); !result.Eligible {
			continue
		}
		contract := cap.Contract()
		for _, output := range contract.ProducedOutputs {
			if output == expectedKind {
				return cap
			}
		}
	}
	return nil
}

// OrderedPhases derives a deterministic phase ordering from the gate sequence.
// If gates are present, the ordering follows the From→To chain. Otherwise,
// the phases from PhaseRoutes are sorted alphabetically.
// Note: Exported for testing purposes.
func OrderedPhases(phaseRoutes map[string]string, gates []gate.PhaseGate) []string {
	if len(gates) > 0 {
		seen := map[string]bool{}
		var phases []string
		for _, gate := range gates {
			from := string(gate.From)
			to := string(gate.To)
			if !seen[from] {
				seen[from] = true
				phases = append(phases, from)
			}
			if !seen[to] {
				seen[to] = true
				phases = append(phases, to)
			}
		}
		return phases
	}
	// Fallback: sorted keys from PhaseRoutes.
	keys := make([]string, 0, len(phaseRoutes))
	for k := range phaseRoutes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// phaseExpectedArtifact maps a phase name to the artifact kind it is expected
// to produce.
func phaseExpectedArtifact(phase string) euclotypes.ArtifactKind {
	switch phase {
	case "explore":
		return euclotypes.ArtifactKindExplore
	case "plan", "plan_tests":
		return euclotypes.ArtifactKindPlan
	case "edit", "patch", "implement", "stage":
		return euclotypes.ArtifactKindEditIntent
	case "verify":
		return euclotypes.ArtifactKindVerification
	case "reproduce", "trace":
		return euclotypes.ArtifactKindExplore
	case "localize", "analyze", "review":
		return euclotypes.ArtifactKindAnalyze
	case "summarize", "report":
		return euclotypes.ArtifactKindFinalReport
	default:
		return ""
	}
}

// snapshotFromEnv builds a CapabilitySnapshot from the execution envelope.
func snapshotFromEnv(env euclotypes.ExecutionEnvelope) euclotypes.CapabilitySnapshot {
	if env.Registry == nil {
		return euclotypes.CapabilitySnapshot{}
	}
	// Call the global snapshot function that will be set by root euclo
	return defaultSnapshotFunc(env.Registry)
}

// mergeCapabilityArtifactsToState stores capability-produced artifacts
// into the state context so subsequent gates and capabilities can see them.
func mergeCapabilityArtifactsToState(state *core.Context, artifacts []euclotypes.Artifact) {
	if state == nil || len(artifacts) == 0 {
		return
	}
	// Update the euclo.artifacts slice in state.
	existing := euclotypes.ArtifactStateFromContext(state)
	merged := append(existing.All(), artifacts...)
	state.Set("euclo.artifacts", merged)

	// Also set individual state keys for compatibility with CollectArtifactsFromState.
	for _, art := range artifacts {
		key := euclotypes.StateKeyForArtifactKind(art.Kind)
		if key != "" && art.Payload != nil {
			state.Set(key, art.Payload)
		}
	}
}

// recordProfileControllerObservability writes profile controller execution
// details into state for the action log and proof surface.
func recordProfileControllerObservability(
	state *core.Context,
	pcResult *ProfileControllerResult,
	mode euclotypes.ModeResolution,
	profile euclotypes.ExecutionProfileSelection,
) {
	if state == nil || pcResult == nil {
		return
	}
	state.Set("euclo.profile_controller", map[string]any{
		"mode_id":           mode.ModeID,
		"profile_id":        profile.ProfileID,
		"capability_ids":    pcResult.CapabilityIDs,
		"phases_executed":   pcResult.PhasesExecuted,
		"phase_records":     profilePhaseRecordsState(pcResult.PhaseRecords),
		"early_stop":        pcResult.EarlyStop,
		"early_stop_phase":  pcResult.EarlyStopPhase,
		"gate_evals_count":  len(pcResult.GateEvals),
		"recovery_attempts": pcResult.RecoveryAttempts,
	})
	state.Set("euclo.profile_phase_records", profilePhaseRecordsState(pcResult.PhaseRecords))
}

func buildProfileCapabilityPhaseRecords(phases []string, consumed []euclotypes.ArtifactKind, artifacts []euclotypes.Artifact) []PhaseArtifactRecord {
	if len(phases) == 0 {
		return nil
	}
	currentConsumed := append([]euclotypes.ArtifactKind{}, consumed...)
	records := make([]PhaseArtifactRecord, 0, len(phases))
	for _, phase := range phases {
		produced := filterArtifactsByKind(artifacts, phaseExpectedArtifact(phase))
		records = append(records, PhaseArtifactRecord{
			Phase:             phase,
			ArtifactsProduced: produced,
			ArtifactsConsumed: append([]euclotypes.ArtifactKind{}, currentConsumed...),
		})
		currentConsumed = appendUniqueArtifactKinds(currentConsumed, artifactKindsFromArtifacts(produced)...)
	}
	return records
}

func profilePhaseRecordsState(records []PhaseArtifactRecord) []map[string]any {
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

func artifactKindsFromState(state euclotypes.ArtifactState) []euclotypes.ArtifactKind {
	return artifactKindsFromArtifacts(state.All())
}

func artifactKindsFromArtifacts(artifacts []euclotypes.Artifact) []euclotypes.ArtifactKind {
	if len(artifacts) == 0 {
		return nil
	}
	out := make([]euclotypes.ArtifactKind, 0, len(artifacts))
	seen := map[euclotypes.ArtifactKind]struct{}{}
	for _, artifact := range artifacts {
		if artifact.Kind == "" {
			continue
		}
		if _, ok := seen[artifact.Kind]; ok {
			continue
		}
		seen[artifact.Kind] = struct{}{}
		out = append(out, artifact.Kind)
	}
	return out
}

func artifactKindsToStrings(kinds []euclotypes.ArtifactKind) []string {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if kind == "" {
			continue
		}
		out = append(out, string(kind))
	}
	return out
}

func filterArtifactsByKind(artifacts []euclotypes.Artifact, kind euclotypes.ArtifactKind) []euclotypes.Artifact {
	if len(artifacts) == 0 || kind == "" {
		return nil
	}
	var out []euclotypes.Artifact
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			out = append(out, artifact)
		}
	}
	return out
}

func appendUniqueArtifactKinds(base []euclotypes.ArtifactKind, extra ...euclotypes.ArtifactKind) []euclotypes.ArtifactKind {
	seen := make(map[euclotypes.ArtifactKind]struct{}, len(base)+len(extra))
	out := make([]euclotypes.ArtifactKind, 0, len(base)+len(extra))
	for _, kind := range base {
		if kind == "" {
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		out = append(out, kind)
	}
	for _, kind := range extra {
		if kind == "" {
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		out = append(out, kind)
	}
	return out
}

func partialResult(task *core.Task, pcResult *ProfileControllerResult) *core.Result {
	return &core.Result{
		Success: false,
		NodeID:  taskNodeID(task),
		Data: map[string]any{
			"status":          "partial",
			"phases_executed": pcResult.PhasesExecuted,
			"early_stop":      pcResult.EarlyStop,
		},
	}
}

func failedResult(task *core.Task, capResult euclotypes.ExecutionResult, pcResult *ProfileControllerResult) *core.Result {
	data := map[string]any{
		"status":          string(capResult.Status),
		"summary":         capResult.Summary,
		"phases_executed": pcResult.PhasesExecuted,
	}
	if capResult.FailureInfo != nil {
		data["failure_code"] = capResult.FailureInfo.Code
		data["failure_message"] = capResult.FailureInfo.Message
	}
	return &core.Result{
		Success: false,
		NodeID:  taskNodeID(task),
		Data:    data,
	}
}

func successResult(task *core.Task, capResult euclotypes.ExecutionResult, pcResult *ProfileControllerResult) *core.Result {
	return &core.Result{
		Success: true,
		NodeID:  taskNodeID(task),
		Data: map[string]any{
			"status":          string(capResult.Status),
			"summary":         capResult.Summary,
			"phases_executed": pcResult.PhasesExecuted,
			"capability_ids":  pcResult.CapabilityIDs,
		},
	}
}

func completedResult(task *core.Task, pcResult *ProfileControllerResult) *core.Result {
	return &core.Result{
		Success: true,
		NodeID:  taskNodeID(task),
		Data: map[string]any{
			"status":          "completed",
			"phases_executed": pcResult.PhasesExecuted,
			"capability_ids":  pcResult.CapabilityIDs,
		},
	}
}

func taskNodeID(task *core.Task) string {
	if task != nil {
		return task.ID
	}
	return "euclo"
}

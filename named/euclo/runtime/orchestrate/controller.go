package orchestrate

import (
	"context"
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
	return newProfileExecutionEngine(pc).Execute(ctx, profile, mode, env)
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
	defaultOrchestrateRecorder.recordProfileControllerObservability(state, pcResult, mode, profile)
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
	return defaultOrchestrateRecorder.profilePhaseRecordsState(records)
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

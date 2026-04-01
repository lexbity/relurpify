package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func NormalizeTaskEnvelope(task *core.Task, state *core.Context, registry *capability.Registry) TaskEnvelope {
	envelope := TaskEnvelope{
		EditPermitted:      true,
		CapabilitySnapshot: SnapshotCapabilities(registry),
	}
	if task == nil {
		envelope.EditPermitted = envelope.CapabilitySnapshot.HasWriteTools
		return envelope
	}
	envelope.TaskID = task.ID
	envelope.Instruction = strings.TrimSpace(task.Instruction)
	if task.Context != nil {
		envelope.Workspace = stringValue(task.Context["workspace"])
		envelope.ModeHint = normalizedModeHint(
			task.Context["euclo.mode"],
			task.Context["mode"],
			task.Context["mode_hint"],
		)
		envelope.ExplicitVerification = strings.TrimSpace(fmt.Sprint(task.Context["verification"]))
		if envelope.ExplicitVerification == "<nil>" {
			envelope.ExplicitVerification = ""
		}
	}
	if state != nil {
		envelope.ResumedMode = resumedModeFromState(state)
		envelope.PreviousArtifactKinds = previousArtifactKinds(state)
	}
	envelope.EditPermitted = envelope.CapabilitySnapshot.HasWriteTools
	return envelope
}

func resumedModeFromState(state *core.Context) string {
	if state == nil {
		return ""
	}
	if mode := normalizedModeHint(state.GetString("euclo.mode")); mode != "" {
		return mode
	}
	raw, ok := state.Get("euclo.interaction_state")
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case interaction.InteractionState:
		return normalizedModeHint(typed.Mode)
	case map[string]any:
		return normalizedModeHint(typed["mode"])
	}
	return ""
}

// ClassifyTask performs keyword-based classification for backward compatibility.
// For confidence-scored classification with ambiguity detection, use ClassifyTaskScored.
func ClassifyTask(envelope TaskEnvelope) TaskClassification {
	scored := ClassifyTaskScored(envelope)
	return scored.TaskClassification
}

// ClassifyTaskScored performs signal-based classification with confidence scoring
// and ambiguity detection. Returns ranked mode candidates with the signals that
// contributed to each score.
func ClassifyTaskScored(envelope TaskEnvelope) ScoredClassification {
	signals := CollectSignals(envelope)
	candidates := ScoreSignals(signals)

	// Build intents from candidates for backward compat.
	intents := make([]string, 0, len(candidates))
	reasons := make([]string, 0, len(signals))
	for _, c := range candidates {
		intents = append(intents, c.Mode)
	}
	for _, s := range signals {
		reasons = append(reasons, s.Kind+":"+s.Value)
	}

	// Default to code if no keyword/task_structure/error_text signals fired.
	// Skip baseline injection when a review signal is already present — the
	// review keywords carry enough weight to win without the code default competing.
	hasStrongSignal := false
	hasReviewSignal := false
	for _, s := range signals {
		if s.Kind == "keyword" || s.Kind == "task_structure" || s.Kind == "error_text" || s.Kind == "context_hint" {
			hasStrongSignal = true
		}
		// Only suppress the code baseline for explicit review signals (keyword,
		// context_hint, task_structure) — not workspace_state "read_only" which
		// fires for any empty registry and should not be strong enough to win alone.
		if s.Mode == "review" && (s.Kind == "keyword" || s.Kind == "context_hint" || s.Kind == "task_structure") {
			hasReviewSignal = true
		}
	}
	if !hasStrongSignal && !hasReviewSignal {
		// Inject a baseline code signal so it wins over weak workspace signals.
		signals = append(signals, ClassificationSignal{
			Kind: "default", Value: "code", Weight: WeightDefault, Mode: "code",
		})
		candidates = ScoreSignals(signals)
		intents = make([]string, 0, len(candidates))
		for _, c := range candidates {
			intents = append(intents, c.Mode)
		}
		reasons = append(reasons, "default:code")
	}
	if len(intents) == 0 {
		intents = []string{"code"}
		reasons = append(reasons, "default:code")
		candidates = []ModeCandidate{{Mode: "code", Score: 0, Signals: []string{"default"}}}
	}

	lower := strings.ToLower(envelope.Instruction)

	classification := TaskClassification{
		IntentFamilies:                 intents,
		RecommendedMode:                intents[0],
		MixedIntent:                    len(intents) > 1,
		EditPermitted:                  envelope.EditPermitted,
		RequiresEvidenceBeforeMutation: containsIntent(intents, "debug") || strings.TrimSpace(envelope.ExplicitVerification) != "",
		RequiresDeterministicStages:    containsIntent(intents, "planning") || containsIntent(intents, "review"),
		Scope:                          "local",
		RiskLevel:                      "low",
		ReasonCodes:                    reasons,
	}
	if classification.MixedIntent {
		classification.RiskLevel = "medium"
	}
	if containsIntent(intents, "planning") || strings.Contains(lower, "across") || strings.Contains(lower, "multiple") {
		classification.Scope = "cross_cutting"
		classification.RiskLevel = "medium"
	}
	if containsIntent(intents, "review") && !envelope.EditPermitted {
		classification.RiskLevel = "low"
	}
	if !envelope.EditPermitted {
		classification.ReasonCodes = append(classification.ReasonCodes, "constraint:read_only")
	}
	if envelope.CapabilitySnapshot.HasVerificationTools {
		classification.ReasonCodes = append(classification.ReasonCodes, "capability:verification_available")
	}

	// Compute total weight for normalization.
	totalWeight := 0.0
	for _, s := range signals {
		totalWeight += s.Weight
	}

	return ScoredClassification{
		TaskClassification: classification,
		Candidates:         candidates,
		Confidence:         NormalizeConfidence(candidates, totalWeight),
		Ambiguous:          IsAmbiguous(candidates),
		Signals:            signals,
	}
}

func ResolveMode(envelope TaskEnvelope, classification TaskClassification, registry *ModeRegistry) ModeResolution {
	modeID := envelope.ModeHint
	source := "classifier"
	reasons := append([]string{}, classification.ReasonCodes...)
	constraints := make([]string, 0, 2)
	if modeID != "" {
		source = "explicit"
		reasons = append(reasons, "mode:explicit")
	} else if classification.RequiresDeterministicStages && containsIntent(classification.IntentFamilies, "planning") {
		modeID = "planning"
		source = "constraint"
		reasons = append(reasons, "mode:deterministic_scope")
	} else if envelope.ResumedMode != "" {
		modeID = envelope.ResumedMode
		source = "resumed"
		reasons = append(reasons, "mode:resumed")
	} else {
		modeID = classification.RecommendedMode
	}
	if registry != nil {
		if _, ok := registry.Lookup(modeID); !ok {
			modeID = classification.RecommendedMode
			source = "classifier"
			reasons = append(reasons, "mode:fallback_to_classifier")
		}
	}
	if !envelope.EditPermitted {
		constraints = append(constraints, "mutation_blocked")
	}
	return ModeResolution{
		ModeID:      modeID,
		Source:      source,
		ReasonCodes: reasons,
		Constraints: constraints,
	}
}

func SelectExecutionProfile(envelope TaskEnvelope, classification TaskClassification, mode ModeResolution, registry *ExecutionProfileRegistry) ExecutionProfileSelection {
	selection := ExecutionProfileSelection{
		ProfileID:       "edit_verify_repair",
		ReasonCodes:     []string{"profile:default"},
		MutationAllowed: envelope.EditPermitted,
	}
	if registry == nil {
		return selection
	}
	profileID := primaryProfileForMode(mode.ModeID, envelope, classification)
	descriptor, ok := registry.Lookup(profileID)
	if !ok {
		descriptor, _ = registry.Lookup("edit_verify_repair")
	}
	selection.ProfileID = descriptor.ProfileID
	selection.FallbackProfileIDs = append([]string{}, descriptor.FallbackProfiles...)
	selection.RequiredArtifacts = append([]string{}, descriptor.RequiredArtifacts...)
	selection.CompletionContract = descriptor.CompletionContract
	selection.PhaseRoutes = cloneStringMap(descriptor.PhaseRoutes)
	selection.VerificationRequired = descriptor.VerificationRequired
	selection.MutationAllowed = descriptor.MutationPolicy != "disallowed" && envelope.EditPermitted
	selection.ReasonCodes = []string{"profile:" + selection.ProfileID, "mode:" + mode.ModeID}
	if !selection.MutationAllowed {
		selection.ReasonCodes = append(selection.ReasonCodes, "constraint:non_mutating_profile")
	}
	if classification.RequiresEvidenceBeforeMutation && selection.ProfileID == "edit_verify_repair" {
		if debugDescriptor, ok := registry.Lookup("reproduce_localize_patch"); ok {
			selection.ProfileID = debugDescriptor.ProfileID
			selection.FallbackProfileIDs = append([]string{}, debugDescriptor.FallbackProfiles...)
			selection.RequiredArtifacts = append([]string{}, debugDescriptor.RequiredArtifacts...)
			selection.CompletionContract = debugDescriptor.CompletionContract
			selection.PhaseRoutes = cloneStringMap(debugDescriptor.PhaseRoutes)
			selection.VerificationRequired = debugDescriptor.VerificationRequired
			selection.MutationAllowed = debugDescriptor.MutationPolicy != "disallowed" && envelope.EditPermitted
			selection.ReasonCodes = append(selection.ReasonCodes, "profile:evidence_first_upgrade")
		}
	}
	return selection
}

func primaryProfileForMode(modeID string, envelope TaskEnvelope, classification TaskClassification) string {
	switch modeID {
	case "debug":
		return "reproduce_localize_patch"
	case "tdd":
		return "test_driven_generation"
	case "review":
		return "review_suggest_implement"
	case "planning":
		return "plan_stage_execute"
	case "code":
		return profileForCodeMode(envelope, classification)
	default:
		return profileForCodeMode(envelope, classification)
	}
}

// profileForCodeMode selects the execution profile for code mode using
// context-aware heuristics: workspace state, existing artifacts, and scope.
func profileForCodeMode(envelope TaskEnvelope, classification TaskClassification) string {
	if !envelope.EditPermitted {
		return "plan_stage_execute"
	}

	if looksLikeSummaryOnlyTask(envelope.Instruction) {
		return "plan_stage_execute"
	}

	// Evidence-first: debug signals or explicit verification → investigate first.
	if classification.RequiresEvidenceBeforeMutation {
		return "reproduce_localize_patch"
	}

	// Artifact-aware: plan already exists → execute it directly.
	for _, kind := range envelope.PreviousArtifactKinds {
		if strings.Contains(kind, "plan") {
			return "edit_verify_repair"
		}
	}

	// Artifact-aware: reproduction/analysis exists → continue investigation.
	for _, kind := range envelope.PreviousArtifactKinds {
		if strings.Contains(kind, "explore") || strings.Contains(kind, "analyze") {
			return "reproduce_localize_patch"
		}
	}

	// Scope-aware: large scope (>5 files or cross-cutting) → plan first.
	if classification.Scope == "cross_cutting" {
		return "plan_stage_execute"
	}

	return "edit_verify_repair"
}

func looksLikeSummaryOnlyTask(instruction string) bool {
	lower := strings.ToLower(strings.TrimSpace(instruction))
	if lower == "" {
		return false
	}
	hasSummaryIntent := strings.Contains(lower, "summarize") ||
		strings.Contains(lower, "summary") ||
		strings.Contains(lower, "current status") ||
		strings.Contains(lower, "status update") ||
		strings.Contains(lower, "report status")
	if !hasSummaryIntent {
		return false
	}
	mutationSignals := []string{"implement", "fix", "change", "refactor", "patch", "update", "add", "rename", "edit"}
	for _, signal := range mutationSignals {
		if strings.Contains(lower, signal) {
			return false
		}
	}
	return true
}

func SnapshotCapabilities(registry *capability.Registry) euclotypes.CapabilitySnapshot {
	snapshot := euclotypes.CapabilitySnapshot{}
	if registry == nil {
		return snapshot
	}
	tools := registry.ModelCallableTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		name := strings.TrimSpace(tool.Name())
		if name == "" {
			continue
		}
		names = append(names, name)
		perms := tool.Permissions().Permissions
		if perms != nil {
			if len(perms.Network) > 0 {
				snapshot.HasNetworkTools = true
			}
			if len(perms.Executables) > 0 {
				snapshot.HasExecuteTools = true
			}
			for _, fs := range perms.FileSystem {
				switch fs.Action {
				case core.FileSystemRead, core.FileSystemList:
					snapshot.HasReadTools = true
				case core.FileSystemWrite:
					snapshot.HasWriteTools = true
				case core.FileSystemExecute:
					snapshot.HasExecuteTools = true
				}
			}
		}
		lower := strings.ToLower(name)
		if containsAny(lower, "test", "build", "lint", "verify") {
			snapshot.HasVerificationTools = true
		}
		if containsAny(lower, "ast", "lsp") {
			snapshot.HasASTOrLSPTools = true
		}
	}
	if len(names) > 0 {
		sort.Strings(names)
		snapshot.ToolNames = names
	}
	return snapshot
}

func previousArtifactKinds(state *core.Context) []string {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("euclo.artifacts")
	if !ok || raw == nil {
		return nil
	}
	artifacts, ok := raw.([]Artifact)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Kind == "" {
			continue
		}
		out = append(out, string(artifact.Kind))
	}
	return out
}

// Helper functions

func containsAny(text string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func containsIntent(intents []string, target string) bool {
	for _, intent := range intents {
		if intent == target {
			return true
		}
	}
	return false
}

func normalizedModeHint(values ...any) string {
	for _, value := range values {
		mode := strings.TrimSpace(strings.ToLower(fmt.Sprint(value)))
		if mode == "" || mode == "<nil>" {
			continue
		}
		return mode
	}
	return ""
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

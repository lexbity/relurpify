package intake

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

// ResolveMode resolves the execution mode from envelope, classification, and registry.
// This is extracted from runtime/classification.go ResolveMode.
func ResolveMode(envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification, registry *euclotypes.ModeRegistry) euclotypes.ModeResolution {
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
	return euclotypes.ModeResolution{
		ModeID:      modeID,
		Source:      source,
		ReasonCodes: reasons,
		Constraints: constraints,
	}
}

// ResolveProfile resolves the execution profile from envelope, classification, mode, and registry.
// This is extracted from runtime/classification.go SelectExecutionProfile.
func ResolveProfile(envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification, mode euclotypes.ModeResolution, registry *euclotypes.ExecutionProfileRegistry) euclotypes.ExecutionProfileSelection {
	selection := euclotypes.ExecutionProfileSelection{
		ProfileID:       "edit_verify_repair",
		ReasonCodes:     []string{"profile:default"},
		MutationAllowed: envelope.EditPermitted,
	}
	if registry == nil {
		return selection
	}
	profileID := primaryProfileForMode(mode.ModeID, envelope, classification)

	// Task-type aware profile selection for debug mode
	// Analysis tasks should NOT use reproduce_localize_patch (which calls go_test)
	if mode.ModeID == "debug" && classification.TaskType == core.TaskTypeAnalysis {
		if traceDescriptor, ok := registry.Lookup("trace_execute_analyze"); ok {
			profileID = traceDescriptor.ProfileID
		}
	}

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

// primaryProfileForMode returns the primary profile ID for a given mode.
func primaryProfileForMode(modeID string, envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification) string {
	switch modeID {
	case "debug":
		return "reproduce_localize_patch"
	case "tdd":
		return "test_driven_generation"
	case "review":
		return "review_suggest_implement"
	case "planning":
		return "plan_stage_execute"
	case "chat":
		return "chat_ask_respond"
	case "code":
		return profileForCodeMode(envelope, classification)
	default:
		return profileForCodeMode(envelope, classification)
	}
}

// profileForCodeMode selects the execution profile for code mode.
func profileForCodeMode(envelope eucloruntime.TaskEnvelope, classification eucloruntime.TaskClassification) string {
	if !envelope.EditPermitted {
		return "plan_stage_execute"
	}

	if looksLikeSummaryOnlyTask(envelope.Instruction) {
		return "plan_stage_execute"
	}

	// Evidence-first: debug signals or explicit verification -> investigate first.
	if classification.RequiresEvidenceBeforeMutation {
		return "reproduce_localize_patch"
	}

	// Artifact-aware: plan already exists -> execute it directly.
	for _, kind := range envelope.PreviousArtifactKinds {
		if strings.Contains(kind, "plan") {
			return "edit_verify_repair"
		}
	}

	// Artifact-aware: reproduction/analysis exists -> continue investigation.
	for _, kind := range envelope.PreviousArtifactKinds {
		if strings.Contains(kind, "explore") || strings.Contains(kind, "analyze") {
			return "reproduce_localize_patch"
		}
	}

	// Scope-aware: large scope (>5 files or cross-cutting) -> plan first.
	if classification.Scope == "cross_cutting" {
		return "plan_stage_execute"
	}

	return "edit_verify_repair"
}

// looksLikeSummaryOnlyTask checks if the instruction is a summary-only task.
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

// cloneStringMap creates a shallow copy of a string map.
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

// BuildUnitOfWork assembles Euclo's active execution bundle from intake,
// classification, selected mode/profile, and currently available runtime state.
// This is a wrapper that delegates to the existing runtime.BuildUnitOfWork.
func BuildUnitOfWork(
	task *core.Task,
	state *core.Context,
	envelope eucloruntime.TaskEnvelope,
	classification eucloruntime.TaskClassification,
	mode euclotypes.ModeResolution,
	profile euclotypes.ExecutionProfileSelection,
	modeRegistry *euclotypes.ModeRegistry,
	semanticInputs eucloruntime.SemanticInputBundle,
	resolvedPolicy eucloruntime.ResolvedExecutionPolicy,
	executor eucloruntime.WorkUnitExecutorDescriptor,
) eucloruntime.UnitOfWork {
	// Delegate to the existing runtime.BuildUnitOfWork
	// In a full refactor, this would be moved here, but for now we wrap the existing function
	return eucloruntime.BuildUnitOfWork(task, state, envelope, classification, mode, profile, modeRegistry, semanticInputs, resolvedPolicy, executor)
}

// primaryRelurpicCapabilityForWork returns the primary capability ID for the work unit.
func primaryRelurpicCapabilityForWork(envelope eucloruntime.TaskEnvelope, mode euclotypes.ModeResolution) string {
	// Use the classifier's pre-determined capability sequence.
	if len(envelope.CapabilitySequence) > 0 {
		return envelope.CapabilitySequence[0]
	}
	// Fallback: use mode's default capability from registry.
	reg := euclorelurpic.DefaultRegistry()
	if desc, ok := reg.FallbackCapabilityForMode(mode.ModeID); ok {
		return desc.ID
	}
	// Ultimate fallback: ask capability for unknown modes.
	return euclorelurpic.CapabilityChatAsk
}

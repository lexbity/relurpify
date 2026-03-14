package euclo

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

type CapabilitySnapshot struct {
	ToolNames            []string `json:"tool_names,omitempty"`
	HasReadTools         bool     `json:"has_read_tools"`
	HasWriteTools        bool     `json:"has_write_tools"`
	HasExecuteTools      bool     `json:"has_execute_tools"`
	HasNetworkTools      bool     `json:"has_network_tools"`
	HasVerificationTools bool     `json:"has_verification_tools"`
	HasASTOrLSPTools     bool     `json:"has_ast_or_lsp_tools"`
}

// TaskEnvelope is the typed Euclo intake shape used to normalize coding
// requests before routing deeper into the runtime.
type TaskEnvelope struct {
	TaskID                string             `json:"task_id,omitempty"`
	Instruction           string             `json:"instruction,omitempty"`
	Workspace             string             `json:"workspace,omitempty"`
	ModeHint              string             `json:"mode_hint,omitempty"`
	ResumedMode           string             `json:"resumed_mode,omitempty"`
	ExplicitVerification  string             `json:"explicit_verification,omitempty"`
	EditPermitted         bool               `json:"edit_permitted"`
	CapabilitySnapshot    CapabilitySnapshot `json:"capability_snapshot"`
	PreviousArtifactKinds []string           `json:"previous_artifact_kinds,omitempty"`
	ResolvedMode          string             `json:"resolved_mode,omitempty"`
	ExecutionProfile      string             `json:"execution_profile,omitempty"`
}

type TaskClassification struct {
	IntentFamilies                 []string `json:"intent_families,omitempty"`
	RecommendedMode                string   `json:"recommended_mode,omitempty"`
	MixedIntent                    bool     `json:"mixed_intent"`
	EditPermitted                  bool     `json:"edit_permitted"`
	RequiresEvidenceBeforeMutation bool     `json:"requires_evidence_before_mutation"`
	RequiresDeterministicStages    bool     `json:"requires_deterministic_stages"`
	Scope                          string   `json:"scope,omitempty"`
	RiskLevel                      string   `json:"risk_level,omitempty"`
	ReasonCodes                    []string `json:"reason_codes,omitempty"`
}

func NormalizeTaskEnvelope(task *core.Task, state *core.Context, registry *capability.Registry) TaskEnvelope {
	envelope := TaskEnvelope{
		EditPermitted:      true,
		CapabilitySnapshot: snapshotCapabilities(registry),
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
		envelope.ResumedMode = normalizedModeHint(state.GetString("euclo.mode"))
		envelope.PreviousArtifactKinds = previousArtifactKinds(state)
	}
	envelope.EditPermitted = envelope.CapabilitySnapshot.HasWriteTools
	return envelope
}

func ClassifyTask(envelope TaskEnvelope) TaskClassification {
	lower := strings.ToLower(envelope.Instruction)
	intents := make([]string, 0, 3)
	reasons := make([]string, 0, 6)
	addIntent := func(intent string) {
		intent = strings.TrimSpace(intent)
		if intent == "" {
			return
		}
		for _, existing := range intents {
			if existing == intent {
				return
			}
		}
		intents = append(intents, intent)
	}
	if containsAny(lower, "review", "audit", "inspect") {
		addIntent("review")
		reasons = append(reasons, "keyword:review")
	}
	if containsAny(lower, "debug", "diagnose", "root cause", "failing", "failure", "trace") {
		addIntent("debug")
		reasons = append(reasons, "keyword:debug")
	}
	if containsAny(lower, "plan", "design", "architecture", "approach") {
		addIntent("planning")
		reasons = append(reasons, "keyword:planning")
	}
	if containsAny(lower, "test first", "tdd", "write tests", "add tests") {
		addIntent("tdd")
		reasons = append(reasons, "keyword:tdd")
	}
	if containsAny(lower, "implement", "fix", "change", "refactor", "patch", "update") {
		addIntent("code")
		reasons = append(reasons, "keyword:code")
	}
	if len(intents) == 0 {
		addIntent("code")
		reasons = append(reasons, "default:code")
	}
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
	return classification
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
		if !envelope.EditPermitted {
			return "plan_stage_execute"
		}
		if classification.RequiresEvidenceBeforeMutation {
			return "reproduce_localize_patch"
		}
		return "edit_verify_repair"
	default:
		if !envelope.EditPermitted {
			return "plan_stage_execute"
		}
		return "edit_verify_repair"
	}
}

func snapshotCapabilities(registry *capability.Registry) CapabilitySnapshot {
	snapshot := CapabilitySnapshot{}
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

func classificationContextPayload(classification TaskClassification) map[string]any {
	return map[string]any{
		"intent_families":                   append([]string{}, classification.IntentFamilies...),
		"recommended_mode":                  classification.RecommendedMode,
		"mixed_intent":                      classification.MixedIntent,
		"edit_permitted":                    classification.EditPermitted,
		"requires_evidence_before_mutation": classification.RequiresEvidenceBeforeMutation,
		"requires_deterministic_stages":     classification.RequiresDeterministicStages,
		"scope":                             classification.Scope,
		"risk_level":                        classification.RiskLevel,
		"reason_codes":                      append([]string{}, classification.ReasonCodes...),
	}
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

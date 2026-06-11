package intake

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

// ResumeState holds values from envelope for task normalization.
type ResumeState struct {
	Family string
}

// NormalizeTaskEnvelope creates a TaskEnvelope from a execution.Task and resume state.
// It extracts hints from the instruction, pulls context values, and applies resume state.
func NormalizeTaskEnvelope(task *execution.Task, resume *ResumeState) (*TaskEnvelope, error) {
	if task == nil {
		return nil, errors.New("nil task")
	}

	// Create base normalizer and parse the instruction
	normalizer := NewTaskNormalizer()
	result := normalizer.Normalize(task.ID, "", task.Instruction)
	envelope := result.TaskEnvelope

	// Set task type from task.Type or default to "analysis"
	if task.Type != "" {
		envelope.TaskType = task.Type
	} else {
		envelope.TaskType = "analysis"
	}

	// Extract context values from task.Context
	if task.Context != nil {
		// Family hint from context
		if v, ok := task.Context["euclo.family"].(string); ok && v != "" {
			envelope.FamilyHint = v
		}

		// User files from context
		if v, ok := task.Context["euclo.user_files"].([]string); ok {
			envelope.UserFiles = v
		}

		// Session pins from context
		if v, ok := task.Context["euclo.session_pins"].([]string); ok {
			envelope.SessionPins = v
		}

		// Explicit verification from context
		if v, ok := task.Context["verification"].(bool); ok {
			envelope.ExplicitVerification = v
		}
		// Also accept string "true" for verification
		if v, ok := task.Context["verification"].(string); ok {
			envelope.ExplicitVerification = strings.ToLower(v) == "true"
		}
	}

	// Extract negative constraint seeds from instruction
	envelope.NegativeConstraintSeeds = extractNegativeConstraintSeeds(task.Instruction)

	// Apply resume state if available
	if resume != nil {
		if resume.Family != "" {
			envelope.ResumedFamily = resume.Family
		}
	}

	// Sanitize the clean message
	envelope.CleanMessage = SanitizeInstruction(envelope.CleanMessage)
	envelope.Instruction = SanitizeInstruction(envelope.Instruction)
	envelope.Evidence = BuildIntentEvidence(envelope)

	return envelope, nil
}

// BuildIntentEvidence derives a structured evidence record from a normalized task envelope.
// The record is always populated when an envelope is present, even if some fields remain missing.
func BuildIntentEvidence(envelope *TaskEnvelope) *intentcontext.IntentEvidence {
	evidence := &intentcontext.IntentEvidence{
		ActionType:          inferActionType(envelope),
		Target:              inferTarget(envelope),
		Scope:               inferScope(envelope),
		RiskLevel:           inferRiskLevel(envelope),
		ExpectedVerb:        inferExpectedVerb(envelope),
		ExplicitFiles:       append([]string(nil), envelope.ExplicitFiles...),
		UserFiles:           append([]string(nil), envelope.UserFiles...),
		WorkspaceScopes:     append([]string(nil), envelope.WorkspaceScopes...),
		SessionPins:         append([]string(nil), envelope.SessionPins...),
		ContextHints:        collectContextHints(envelope),
		SessionContinuation: strings.TrimSpace(envelope.SessionHint),
		FollowUp:            strings.TrimSpace(envelope.FollowUpHint),
		NegativeConstraints: append([]string(nil), envelope.NegativeConstraintSeeds...),
		ReasonCodes:         collectEvidenceReasonCodes(envelope),
	}

	evidence.MissingFields = collectMissingEvidenceFields(evidence)
	evidence.RequiresClarification = len(evidence.MissingFields) > 0
	if evidence.RequiresClarification {
		evidence.ReasonCodes = append(evidence.ReasonCodes, "clarification_required")
	}
	evidence.Normalize()
	return evidence
}

// BuildIntentInterpretation derives a prompt-facing interpretation record from evidence.
// It annotates evidence and may incorporate classification confidence, but it does not select a route.
func BuildIntentInterpretation(evidence *intentcontext.IntentEvidence, classification *ScoredClassification) *intentcontext.IntentInterpretation {
	interpretation := &intentcontext.IntentInterpretation{}
	if evidence != nil {
		interpretation.ActionType = evidence.ActionType
		interpretation.Target = evidence.Target
		interpretation.Scope = evidence.Scope
		interpretation.RiskLevel = evidence.RiskLevel
		interpretation.MissingInfo = append([]string(nil), evidence.MissingFields...)
		interpretation.ReasonCodes = append(interpretation.ReasonCodes, evidence.ReasonCodes...)
		if evidence.RequiresClarification {
			interpretation.ReasonCodes = append(interpretation.ReasonCodes, "clarification_required")
		}
	}
	if classification != nil {
		interpretation.ReasonCodes = append(interpretation.ReasonCodes, "classification_present")
		interpretation.ConfidenceNote = strings.TrimSpace(classificationSummary(classification))
	}
	if interpretation.ConfidenceNote == "" {
		interpretation.ConfidenceNote = "deterministic interpretation from request evidence"
	}
	if interpretation.Rationale == "" {
		interpretation.Rationale = interpretationRationale(evidence, classification)
	}
	interpretation.Normalize()
	return interpretation
}

// NormalizeTaskEnvelopeWithRegistry creates a TaskEnvelope with registry-based flags.
// The hasWriteTools parameter determines EditPermitted status.
func NormalizeTaskEnvelopeWithRegistry(task *execution.Task, resume *ResumeState, hasWriteTools bool) (*TaskEnvelope, error) {
	envelope, err := NormalizeTaskEnvelope(task, resume)
	if err != nil {
		return nil, err
	}
	if envelope != nil {
		envelope.EditPermitted = hasWriteTools
	}
	return envelope, nil
}

// extractNegativeConstraintSeeds extracts negative constraint phrases from instruction.
// Looks for patterns like "don't change X", "do not modify Y", "without breaking Z"
func extractNegativeConstraintSeeds(instruction string) []string {
	var seeds []string
	seen := make(map[string]bool)

	// Pattern: "don't/do not/never ..."
	dontPattern := regexp.MustCompile(`(?i)(don't|do not|never)\s+([a-z]+\s+(?:the\s+)?[a-z\s]+?)(?:\.|,|;|$|\s+(?:and|or|but))`)
	matches := dontPattern.FindAllStringSubmatch(instruction, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			seed := strings.TrimSpace(match[1] + " " + match[2])
			if seed != "" && !seen[seed] {
				seeds = append(seeds, seed)
				seen[seed] = true
			}
		}
	}

	// Pattern: "without ..."
	withoutPattern := regexp.MustCompile(`(?i)without\s+([a-z\s]+?)(?:\.|,|;|$|\s+(?:and|or|but))`)
	withoutMatches := withoutPattern.FindAllStringSubmatch(instruction, -1)
	for _, match := range withoutMatches {
		if len(match) >= 2 {
			seed := "without " + strings.TrimSpace(match[1])
			if seed != "" && !seen[seed] {
				seeds = append(seeds, seed)
				seen[seed] = true
			}
		}
	}

	// Pattern: "avoid ..."
	avoidPattern := regexp.MustCompile(`(?i)avoid\s+([a-z\s]+?)(?:\.|,|;|$|\s+(?:and|or|but))`)
	avoidMatches := avoidPattern.FindAllStringSubmatch(instruction, -1)
	for _, match := range avoidMatches {
		if len(match) >= 2 {
			seed := "avoid " + strings.TrimSpace(match[1])
			if seed != "" && !seen[seed] {
				seeds = append(seeds, seed)
				seen[seed] = true
			}
		}
	}

	return seeds
}

func inferActionType(envelope *TaskEnvelope) string {
	candidates := []string{
		strings.ToLower(strings.TrimSpace(envelope.TaskType)),
		strings.ToLower(strings.TrimSpace(envelope.FollowUpHint)),
		strings.ToLower(strings.TrimSpace(envelope.ContextHint)),
		strings.ToLower(strings.TrimSpace(envelope.Instruction)),
	}
	for _, candidate := range candidates {
		switch {
		case strings.Contains(candidate, "review"):
			return "review"
		case strings.Contains(candidate, "explain"):
			return "explain"
		case strings.Contains(candidate, "analyze") || strings.Contains(candidate, "analyse") || strings.Contains(candidate, "inspect") || strings.Contains(candidate, "investigate"):
			return "analyze"
		case strings.Contains(candidate, "debug") || strings.Contains(candidate, "fix") || strings.Contains(candidate, "repair") || strings.Contains(candidate, "patch"):
			return "repair"
		case strings.Contains(candidate, "implement") || strings.Contains(candidate, "add") || strings.Contains(candidate, "create"):
			return "implement"
		case strings.Contains(candidate, "refactor") || strings.Contains(candidate, "reshape"):
			return "refactor"
		case strings.Contains(candidate, "plan"):
			return "plan"
		case strings.Contains(candidate, "clarify") || strings.Contains(candidate, "choose") || strings.Contains(candidate, "decide"):
			return "clarify"
		case strings.Contains(candidate, "continue"):
			return "continue"
		}
	}
	return ""
}

func inferTarget(envelope *TaskEnvelope) string {
	files := append([]string(nil), envelope.ExplicitFiles...)
	files = append(files, envelope.UserFiles...)
	files = append(files, envelope.SessionPins...)
	switch {
	case len(files) == 1:
		return strings.TrimSpace(files[0])
	case len(files) > 1:
		return "file_set"
	case len(envelope.WorkspaceScopes) > 0:
		return strings.TrimSpace(envelope.WorkspaceScopes[0])
	case strings.TrimSpace(envelope.FamilyHint) != "":
		return strings.TrimSpace(envelope.FamilyHint)
	case strings.TrimSpace(envelope.ContextHint) != "":
		return strings.TrimSpace(envelope.ContextHint)
	default:
		return ""
	}
}

func inferScope(envelope *TaskEnvelope) string {
	files := append([]string(nil), envelope.ExplicitFiles...)
	files = append(files, envelope.UserFiles...)
	files = append(files, envelope.SessionPins...)
	switch {
	case len(files) == 1:
		return "single_file"
	case len(files) > 1:
		return "multi_file"
	case len(envelope.WorkspaceScopes) > 0:
		return "workspace"
	case len(envelope.SessionPins) > 0:
		return "workspace"
	case strings.TrimSpace(envelope.SessionHint) != "":
		return "session"
	default:
		return "unknown"
	}
}

func inferRiskLevel(envelope *TaskEnvelope) string {
	switch inferActionType(envelope) {
	case "review", "explain", "analyze", "clarify":
		return "low"
	case "plan":
		return "medium"
	case "implement", "refactor", "continue":
		return "medium"
	case "repair":
		return "high"
	default:
		if len(envelope.NegativeConstraintSeeds) > 0 || envelope.ExplicitVerification {
			return "medium"
		}
		return "unknown"
	}
}

func inferExpectedVerb(envelope *TaskEnvelope) string {
	switch inferActionType(envelope) {
	case "review":
		return "review"
	case "explain":
		return "explain"
	case "analyze":
		return "analyze"
	case "repair":
		return "repair"
	case "implement":
		return "implement"
	case "refactor":
		return "refactor"
	case "plan":
		return "plan"
	case "clarify":
		return "clarify"
	case "continue":
		return "continue"
	default:
		return ""
	}
}

func collectContextHints(envelope *TaskEnvelope) []string {
	hints := []string{
		strings.TrimSpace(envelope.ContextHint),
		strings.TrimSpace(envelope.FamilyHint),
		strings.TrimSpace(envelope.SessionHint),
		strings.TrimSpace(envelope.FollowUpHint),
	}
	var out []string
	seen := make(map[string]struct{}, len(hints))
	for _, hint := range hints {
		if hint == "" {
			continue
		}
		if _, ok := seen[hint]; ok {
			continue
		}
		seen[hint] = struct{}{}
		out = append(out, hint)
	}
	return out
}

func collectEvidenceReasonCodes(envelope *TaskEnvelope) []string {
	var codes []string
	if action := inferActionType(envelope); action != "" {
		codes = append(codes, "action:"+action)
	} else {
		codes = append(codes, "missing:action_type")
	}
	if target := inferTarget(envelope); target != "" {
		codes = append(codes, "target:"+target)
	} else {
		codes = append(codes, "missing:target")
	}
	codes = append(codes, "scope:"+inferScope(envelope))
	if len(envelope.ExplicitFiles) > 0 {
		codes = append(codes, "explicit_files")
	}
	if len(envelope.UserFiles) > 0 {
		codes = append(codes, "user_files")
	}
	if len(envelope.WorkspaceScopes) > 0 {
		codes = append(codes, "workspace_scopes")
	}
	if len(envelope.SessionPins) > 0 {
		codes = append(codes, "session_pins")
	}
	if strings.TrimSpace(envelope.SessionHint) != "" {
		codes = append(codes, "session_continuation")
	}
	if strings.TrimSpace(envelope.FollowUpHint) != "" {
		codes = append(codes, "follow_up")
	}
	if len(envelope.NegativeConstraintSeeds) > 0 {
		codes = append(codes, "negative_constraints")
	}
	if envelope.ExplicitVerification {
		codes = append(codes, "explicit_verification")
	}
	if strings.TrimSpace(envelope.RawMessage) != "" {
		codes = append(codes, "raw_message_present")
	}
	return codes
}

func collectMissingEvidenceFields(evidence *intentcontext.IntentEvidence) []string {
	if evidence == nil {
		return nil
	}
	var missing []string
	if strings.TrimSpace(evidence.ActionType) == "" {
		missing = append(missing, "action_type")
	}
	if strings.TrimSpace(evidence.Target) == "" {
		missing = append(missing, "target")
	}
	if strings.TrimSpace(evidence.Scope) == "" || evidence.Scope == "unknown" {
		missing = append(missing, "scope")
	}
	if len(evidence.ExplicitFiles) == 0 && len(evidence.UserFiles) == 0 && len(evidence.WorkspaceScopes) == 0 && len(evidence.SessionPins) == 0 {
		missing = append(missing, "grounding")
	}
	return missing
}

func classificationSummary(classification *ScoredClassification) string {
	if classification == nil {
		return ""
	}
	return fmt.Sprintf("confidence %.2f, ambiguous=%t", classification.Confidence, classification.Ambiguous)
}

func interpretationRationale(evidence *intentcontext.IntentEvidence, classification *ScoredClassification) string {
	parts := make([]string, 0, 4)
	if evidence != nil {
		if action := strings.TrimSpace(evidence.ActionType); action != "" {
			parts = append(parts, "action="+action)
		}
		if target := strings.TrimSpace(evidence.Target); target != "" {
			parts = append(parts, "target="+target)
		}
		if scope := strings.TrimSpace(evidence.Scope); scope != "" {
			parts = append(parts, "scope="+scope)
		}
		if len(evidence.MissingFields) > 0 {
			parts = append(parts, "missing="+strings.Join(evidence.MissingFields, ","))
		}
	}
	if classification != nil {
		parts = append(parts, classificationSummary(classification))
	}
	if len(parts) == 0 {
		return "deterministic interpretation"
	}
	return strings.Join(parts, "; ")
}

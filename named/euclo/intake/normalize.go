package intake

import (
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// ResumeState holds values from envelope for task normalization.
type ResumeState struct {
	Family             string
	CapabilitySequence []string
}

// NormalizeTaskEnvelope creates a TaskEnvelope from a core.Task and resume state.
// It extracts hints from the instruction, pulls context values, and applies resume state.
func NormalizeTaskEnvelope(task *core.Task, resume *ResumeState) (*TaskEnvelope, error) {
	if task == nil {
		return nil, nil
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
		if len(resume.CapabilitySequence) > 0 {
			envelope.CapabilitySequence = resume.CapabilitySequence
		}
	}

	// Sanitize the clean message
	envelope.CleanMessage = SanitizeInstruction(envelope.CleanMessage)
	envelope.Instruction = SanitizeInstruction(envelope.Instruction)

	return envelope, nil
}

// NormalizeTaskEnvelopeWithRegistry creates a TaskEnvelope with registry-based flags.
// The hasWriteTools parameter determines EditPermitted status.
func NormalizeTaskEnvelopeWithRegistry(task *core.Task, resume *ResumeState, hasWriteTools bool) (*TaskEnvelope, error) {
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

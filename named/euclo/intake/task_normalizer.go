package intake

import (
	"strings"
	"unicode"
)

// TaskNormalizer normalizes user instructions for downstream processing.
type TaskNormalizer struct {
	hintParser *HintParser
}

// NewTaskNormalizer creates a new task normalizer.
func NewTaskNormalizer() *TaskNormalizer {
	return &TaskNormalizer{
		hintParser: NewHintParser(),
	}
}

// NormalizeResult holds the fully normalized task data.
type NormalizeResult struct {
	TaskEnvelope *TaskEnvelope
	Hints        *ParseResult
}

// Normalize processes a raw user message into a normalized form.
func (n *TaskNormalizer) Normalize(taskID, sessionID, message string) *NormalizeResult {
	// Parse hints from the message
	hints := n.hintParser.Parse(message)

	// Strip hints to get clean message
	cleanMessage := n.hintParser.StripHints(message)

	// Detect ingest policy if not explicitly set
	ingestPolicy := hints.IngestPolicy
	if ingestPolicy == "" {
		ingestPolicy = n.hintParser.DetectIngestPolicy(message, len(hints.ExplicitFiles) > 0)
	}

	// Build the task envelope
	envelope := &TaskEnvelope{
		TaskID:           taskID,
		SessionID:        sessionID,
		Instruction:      cleanMessage,
		ContextHint:      hints.ContextHint,
		SessionHint:      hints.SessionHint,
		FollowUpHint:     hints.FollowUpHint,
		AgentModeHint:    hints.AgentModeHint,
		WorkspaceScopes:  hints.WorkspaceScopes,
		ExplicitFiles:    hints.ExplicitFiles,
		IngestPolicy:     ingestPolicy,
		IncrementalSince: hints.IncrementalSince,
		CleanMessage:     cleanMessage,
		RawMessage:       message,
		Metadata:         make(map[string]any),
	}

	return &NormalizeResult{
		TaskEnvelope: envelope,
		Hints:        hints,
	}
}

// NormalizeWithDefaults processes a message with default values for optional fields.
func (n *TaskNormalizer) NormalizeWithDefaults(taskID, sessionID, message string, defaults map[string]any) *NormalizeResult {
	result := n.Normalize(taskID, sessionID, message)

	// Apply defaults for any empty fields
	if defaults != nil {
		if result.TaskEnvelope.ContextHint == "" {
			if v, ok := defaults["context_hint"].(string); ok {
				result.TaskEnvelope.ContextHint = v
			}
		}
		if result.TaskEnvelope.SessionHint == "" {
			if v, ok := defaults["session_hint"].(string); ok {
				result.TaskEnvelope.SessionHint = v
			}
		}
		if result.TaskEnvelope.AgentModeHint == "" {
			if v, ok := defaults["agent_mode"].(string); ok {
				result.TaskEnvelope.AgentModeHint = v
			}
		}
		if len(result.TaskEnvelope.WorkspaceScopes) == 0 {
			if v, ok := defaults["workspace_scopes"].([]string); ok {
				result.TaskEnvelope.WorkspaceScopes = v
			}
		}
	}

	return result
}

// SanitizeInstruction removes unnecessary whitespace and normalizes punctuation.
func SanitizeInstruction(text string) string {
	// Trim leading/trailing whitespace
	text = strings.TrimSpace(text)

	// Replace multiple spaces with single space
	var result strings.Builder
	lastWasSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !lastWasSpace {
				result.WriteRune(' ')
				lastWasSpace = true
			}
		} else {
			result.WriteRune(r)
			lastWasSpace = false
		}
	}

	return strings.TrimSpace(result.String())
}

// TruncateInstruction truncates text to a maximum length, preserving word boundaries.
func TruncateInstruction(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	// Find the last space before maxLen
	truncated := text[:maxLen]
	lastSpace := strings.LastIndexAny(truncated, " \t\n")
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// ExtractKeywords extracts relevant keywords from an instruction for classification.
func ExtractKeywords(text string) []string {
	// Simple keyword extraction - split on non-alphanumeric
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	// Filter to meaningful words (length > 2)
	var keywords []string
	seen := make(map[string]bool)
	for _, word := range words {
		lower := strings.ToLower(word)
		if len(lower) > 2 && !seen[lower] {
			keywords = append(keywords, lower)
			seen[lower] = true
		}
	}

	return keywords
}

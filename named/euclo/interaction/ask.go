package interaction

import (
	"fmt"
	"strings"
	"time"
)

// NewAskUserFrame creates a clarification frame for ask-user prompts.
func NewAskUserFrame(taskID, sessionID, question string, choices []string) *InteractionFrame {
	return newClarificationFrame(taskID, sessionID, FrameIntentClarification, question, NormalizeChoices(choices), nil, nil, nil, "", 5*time.Minute)
}

// NewErrorDecisionFrame creates the operational-failure decision frame emitted
// by the on_error: ask policy (D6/D12). The answer vocabulary is
// retry/continue/abort with abort as the default — an unanswered or expired
// frame resolves to abort via the resolver boundary.
func NewErrorDecisionFrame(taskID, sessionID, stepID, failureMessage string) *InteractionFrame {
	return newClarificationFrame(taskID, sessionID, FrameErrorDecision,
		"An operation failed. How should the run proceed?",
		[]string{"retry", "continue", "abort"},
		nil,
		map[string]any{
			"step_id": strings.TrimSpace(stepID),
			"error":   strings.TrimSpace(failureMessage),
		},
		nil,
		"abort",
		5*time.Minute)
}

// ResponseValue returns the selected answer payload for the frame.
func ResponseValue(frame *InteractionFrame) (string, bool) {
	if frame == nil || frame.Response == nil {
		return "", false
	}
	if choice := strings.TrimSpace(frame.Response.ChosenSlot); choice != "" {
		return choice, true
	}
	if raw, ok := frame.Response.ExtraData["answer"]; ok {
		return strings.TrimSpace(fmt.Sprint(raw)), true
	}
	return "", false
}

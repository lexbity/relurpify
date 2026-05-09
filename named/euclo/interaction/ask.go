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

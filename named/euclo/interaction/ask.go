package interaction

import (
	"fmt"
	"strings"
	"time"
)

// NewAskUserFrame creates a clarification frame for ask-user prompts.
func NewAskUserFrame(taskID, sessionID, question string, choices []string) *InteractionFrame {
	slots := make([]ActionSlot, 0, len(choices))
	for i, choice := range choices {
		choice = strings.TrimSpace(choice)
		if choice == "" {
			continue
		}
		slots = append(slots, ActionSlot{
			ID:      choice,
			Label:   choice,
			Action:  choice,
			Risk:    "low",
			Default: i == 0,
		})
	}
	if len(slots) == 0 {
		slots = []ActionSlot{{
			ID:      "answer",
			Label:   "Answer",
			Action:  "answer",
			Risk:    "low",
			Default: true,
		}}
	}

	frame := &InteractionFrame{
		ID:        generateID(),
		Type:      FrameIntentClarification,
		Kind:      FrameIntentClarification,
		TaskID:    taskID,
		SessionID: sessionID,
		Slots:     slots,
		Payload: map[string]any{
			"question": strings.TrimSpace(question),
			"choices":  append([]string(nil), choices...),
		},
		DefaultSlot: slots[0].ID,
	}
	frame.CreatedAt = time.Now().UTC()
	frame.Metadata.Timestamp = frame.CreatedAt
	frame.Timeout = 5 * time.Minute
	return frame
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

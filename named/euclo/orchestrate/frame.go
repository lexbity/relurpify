package orchestrate

import (
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// ClarificationFrame captures a clarification request lifecycle in orchestrate.
type ClarificationFrame struct {
	ID              string
	TaskID          string
	SessionID       string
	ThoughtRecipeID string
	Question        string
	Choices         []string
	DefaultChoice   string
	MissingFields   []string
	Resume          *interaction.ClarificationResumeMetadata
	Response        *interaction.FrameResult
	CreatedAt       time.Time
	RespondedAt     *time.Time
	Skipped         bool
	SkippedReason   string
}

// NewClarificationFrame creates a clarification frame with explicit question and choices.
func NewClarificationFrame(taskID, sessionID, thoughtRecipeID, question string, choices []string, missingFields []string, resume *interaction.ClarificationResumeMetadata) *ClarificationFrame {
	cleanChoices := interaction.NormalizeChoices(choices)
	defaultChoice := ""
	if len(cleanChoices) > 0 {
		defaultChoice = cleanChoices[0]
	}
	return &ClarificationFrame{
		ID:              interaction.NewClarificationFrame(taskID, sessionID, question, cleanChoices, resume).ID,
		TaskID:          strings.TrimSpace(taskID),
		SessionID:       strings.TrimSpace(sessionID),
		ThoughtRecipeID: strings.TrimSpace(thoughtRecipeID),
		Question:        strings.TrimSpace(question),
		Choices:         cleanChoices,
		DefaultChoice:   defaultChoice,
		MissingFields:   interaction.NormalizeChoices(missingFields),
		Resume:          interaction.CloneClarificationResumeMetadata(resume),
		CreatedAt:       time.Now().UTC(),
	}
}

// ToInteractionFrame converts the orchestrate frame into the interaction package frame.
func (f *ClarificationFrame) ToInteractionFrame() *interaction.InteractionFrame {
	if f == nil {
		return nil
	}
	if frame := interaction.NewClarificationFrame(f.TaskID, f.SessionID, f.Question, f.Choices, interaction.CloneClarificationResumeMetadata(f.Resume)); frame != nil {
		frame.ID = f.ID
		frame.DefaultChoice = strings.TrimSpace(f.DefaultChoice)
		if frame.DefaultChoice == "" && len(frame.Choices) > 0 {
			frame.DefaultChoice = frame.Choices[0]
		}
		frame.Payload["thoughtrecipe_id"] = strings.TrimSpace(f.ThoughtRecipeID)
		frame.Payload["missing_fields"] = append([]string(nil), f.MissingFields...)
		frame.Payload["clarification_kind"] = interaction.FrameIntentClarification
		frame.Payload["skipped"] = f.Skipped
		if strings.TrimSpace(f.SkippedReason) != "" {
			frame.Payload["skipped_reason"] = strings.TrimSpace(f.SkippedReason)
		}
		if f.Response != nil {
			frame.Response = f.Response
			respondedAt := f.Response.RespondedAt
			frame.RespondedAt = &respondedAt
		}
		if !f.CreatedAt.IsZero() {
			frame.CreatedAt = f.CreatedAt
			frame.Metadata.Timestamp = f.CreatedAt
		}
		return frame
	}
	return nil
}

// Pending reports whether the frame is waiting on a response.
func (f *ClarificationFrame) Pending() bool {
	return f != nil && !f.Skipped && f.Response == nil && f.RespondedAt == nil
}

// MarkSkipped marks the frame as skipped after an immediate clarification resolution.
func (f *ClarificationFrame) MarkSkipped(reason string) {
	if f == nil {
		return
	}
	f.Skipped = true
	f.SkippedReason = strings.TrimSpace(reason)
	now := time.Now().UTC()
	f.RespondedAt = &now
}

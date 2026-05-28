package interaction

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// ResumeFrame reconstructs the pending frame from the envelope on restart.
// It scans envelope working memory for the highest-seq frame with nil RespondedAt.
func ResumeFrame(env *contextdata.Envelope) (*InteractionFrame, bool) {
	// Get the highest sequence number
	seqVal, ok := env.GetWorkingValue("euclo.interaction.frame_seq")
	if !ok {
		return nil, false
	}
	seq, ok := seqVal.(int)
	if !ok || seq == 0 {
		return nil, false
	}

	// Check frames from highest to lowest to find pending one
	for i := seq - 1; i >= 0; i-- {
		frameKey := fmt.Sprintf("euclo.interaction.frame.%d", i)
		frameVal, ok := env.GetWorkingValue(frameKey)
		if !ok {
			continue
		}
		frame, ok := frameVal.(*InteractionFrame)
		if !ok {
			continue
		}
		// Return the first pending frame (highest seq with nil RespondedAt)
		if frame.RespondedAt == nil {
			return frame, true
		}
	}

	return nil, false
}

// ResumeClarificationFrame returns the most recent pending clarification frame.
func ResumeClarificationFrame(env *contextdata.Envelope) (*InteractionFrame, bool) {
	frame, ok := ResumeFrame(env)
	if !ok || frame == nil {
		return nil, false
	}
	if frame.Type != FrameIntentClarification {
		return nil, false
	}
	return frame, true
}

// ClarificationResumeMetadataFromFrame extracts resume metadata from a clarification frame.
func ClarificationResumeMetadataFromFrame(frame *InteractionFrame) *ClarificationResumeMetadata {
	if frame == nil {
		return nil
	}
	resume := CloneClarificationResumeMetadata(frame.Resume)
	if resume == nil {
		resume = &ClarificationResumeMetadata{}
	}
	if strings.TrimSpace(resume.ResumeNodeID) == "" && strings.TrimSpace(frame.ID) != "" {
		resume.ResumeNodeID = strings.TrimSpace(frame.ID)
	}
	return resume
}

// ClarificationResponseValue reads the structured answer captured on a clarification frame.
func ClarificationResponseValue(frame *InteractionFrame) (string, bool) {
	return ResponseValue(frame)
}

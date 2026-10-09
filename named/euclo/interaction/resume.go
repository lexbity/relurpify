package interaction

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// ResumeFrame reconstructs the pending frame from the envelope on restart.
// It scans envelope working memory for the highest-seq open frame — an
// unanswered frame within its deadline. Expired frames surface as gaps
// (D12/FR-18): a frame whose deadline passed is never resumed.
func ResumeFrame(env *contextdata.Envelope) (*InteractionFrame, bool) {
	// Get the highest sequence number (atomic, per-key; D15). Both the new
	// uint64 counter and a legacy int value are accepted.
	seq := frameSeqFromEnvelope(env)
	if seq == 0 {
		return nil, false
	}
	now := time.Now().UTC()

	// Check frames from highest to lowest to find pending one. The stored
	// counter is the last-assigned sequence (post-increment), so the scan
	// starts at seq inclusive.
	for i := seq; i >= 0; i-- {
		frameKey := fmt.Sprintf("euclo.interaction.frame.%d", i)
		frameVal, ok := contextdata.GetTyped[any](env, frameKey)
		if !ok {
			continue
		}
		frame, ok := frameVal.(*InteractionFrame)
		if !ok {
			continue
		}
		// Return the first pending frame (highest seq with nil RespondedAt)
		// that is still within its deadline.
		if frame.RespondedAt != nil {
			continue
		}
		if frame.Expired(now) {
			continue
		}
		return frame, true
	}

	return nil, false
}

// frameSeqFromEnvelope reads the atomically-maintained frame counter,
// normalizing the numeric shapes it can hold.
func frameSeqFromEnvelope(env *contextdata.Envelope) int {
	if env == nil {
		return 0
	}
	raw, ok := contextdata.GetTyped[any](env, frameSeqKey)
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case uint64:
		return int(v)
	case uint:
		return int(v)
	case int:
		if v > 0 {
			return v
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	}
	return 0
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

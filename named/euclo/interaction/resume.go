package interaction

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// ResumeFrame reconstructs the pending frame from the envelope on restart.
// It scans envelope working memory for the highest-seq frame with nil RespondedAt.
func ResumeFrame(env *contextdata.Envelope) (*InteractionFrame, bool) {
	if env == nil {
		return nil, false
	}
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

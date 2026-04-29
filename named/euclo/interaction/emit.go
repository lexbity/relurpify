package interaction

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// EmitFrame writes a frame to the envelope and publishes to the event log.
func EmitFrame(ctx context.Context, frame *InteractionFrame, env *contextdata.Envelope, eventLog core.Telemetry) error {
	// Assign sequence number from envelope frame counter
	seq := getNextFrameSeq(env)
	frame.Seq = seq

	// Write frame to envelope working memory
	frameKey := "euclo.interaction.frame." + string(rune(seq))
	env.SetWorkingValue(frameKey, frame, contextdata.MemoryClassTask)

	// Update frame sequence counter
	env.SetWorkingValue("euclo.interaction.frame_seq", seq+1, contextdata.MemoryClassTask)

	// Publish event to event log
	if eventLog != nil {
		// Phase 10: will integrate with actual event log
		// eventLog.Emit(event)
		_ = eventLog
	}

	return nil
}

// getNextFrameSeq gets the next frame sequence number from the envelope.
func getNextFrameSeq(env *contextdata.Envelope) int {
	seqVal, ok := env.GetWorkingValue("euclo.interaction.frame_seq")
	if !ok {
		return 0
	}
	if seq, ok := seqVal.(int); ok {
		return seq
	}
	return 0
}

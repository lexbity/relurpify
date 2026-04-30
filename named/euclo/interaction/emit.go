package interaction

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// EmitFrame writes a frame to the envelope and publishes to the event log.
func EmitFrame(ctx context.Context, frame *InteractionFrame, env *contextdata.Envelope, eventLog core.Telemetry) error {
	if frame == nil || env == nil {
		return nil
	}

	seq := getNextFrameSeq(env)
	frame.Seq = seq
	frame.Kind = frame.Type
	if frame.CreatedAt.IsZero() {
		frame.CreatedAt = time.Now().UTC()
	}
	if frame.Metadata.Timestamp.IsZero() {
		frame.Metadata.Timestamp = frame.CreatedAt
	}

	frameKey := frameStorageKey(seq)
	env.SetWorkingValue(frameKey, frame, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.interaction.frame_seq", seq+1, contextdata.MemoryClassTask)

	telemetry := eventLog
	if telemetry == nil {
		telemetry = core.TelemetryFromContext(ctx)
	}
	if telemetry != nil {
		telemetry.Emit(core.Event{
			Type:      core.EventType("euclo.interaction.frame.emitted"),
			TaskID:    env.TaskID,
			NodeID:    frame.ID,
			Timestamp: time.Now().UTC(),
			Metadata: map[string]any{
				"frame_id":     frame.ID,
				"frame_type":   string(frame.Type),
				"frame_seq":    frame.Seq,
				"session_id":   frame.SessionID,
				"default_slot": frame.DefaultSlot,
				"slot_count":   len(frame.Slots),
			},
		})
	}

	return nil
}

// getNextFrameSeq gets the next frame sequence number from the envelope.
func getNextFrameSeq(env *contextdata.Envelope) int {
	if env == nil {
		return 0
	}
	seqVal, ok := env.GetWorkingValue("euclo.interaction.frame_seq")
	if !ok {
		return 0
	}
	if seq, ok := seqVal.(int); ok {
		return seq
	}
	return 0
}

func frameStorageKey(seq int) string {
	return fmt.Sprintf("euclo.interaction.frame.%d", seq)
}

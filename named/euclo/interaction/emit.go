package interaction

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// frameSeqKey is the per-envelope atomic sequence counter for frames (D15:
// assignment is a single Envelope.NextSequence critical section, never an
// unlocked read-increment-write).
const frameSeqKey = "euclo.interaction.frame_seq"

// EmitFrame writes a frame to the envelope and publishes to the event log.
func EmitFrame(ctx context.Context, frame *InteractionFrame, env *contextdata.Envelope, eventLog telemetry.Telemetry) error {
	if frame == nil {
		return nil
	}
	if env == nil {
		return fmt.Errorf("interaction.emit: envelope required")
	}

	frame.Seq = int(env.NextSequence(frameSeqKey))
	if frame.CreatedAt.IsZero() {
		frame.CreatedAt = time.Now().UTC()
	}
	if frame.Metadata.Timestamp.IsZero() {
		frame.Metadata.Timestamp = frame.CreatedAt
	}

	frameKey := frameStorageKey(frame.Seq)
	env.SetWorkingValueWithClass(frameKey, frame, contextdata.MemoryClassTask)

	sink := eventLog
	if sink == nil {
		sink = telemetry.TelemetryFromContext(ctx)
	}
	if sink != nil {
		ev := telemetry.Event{
			Type:      telemetry.EventType("euclo.interaction.frame.emitted"),
			TaskID:    env.TaskIDSnapshot(),
			NodeID:    frame.ID,
			Timestamp: time.Now().UTC(),
			Metadata: map[string]any{
				"frame_id":     frame.ID,
				"frame_type":   string(frame.Type),
				"frame_seq":    frame.Seq,
				"session_id":   frame.SessionID,
				"default_slot": frame.DefaultSlot,
				"slot_count":   len(frame.Slots),
				"deadline":     frame.Deadline(time.Now().UTC()).Format(time.RFC3339Nano),
			},
		}
		telemetry.StampCorrelation(ctx, &ev)
		sink.Emit(ev)
	}

	return nil
}

func frameStorageKey(seq int) string {
	return fmt.Sprintf("euclo.interaction.frame.%d", seq)
}

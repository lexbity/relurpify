package pipeline

import (
	"context"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	pipelineEventStageStart       telemetry.EventType = "pipeline_stage_start"
	pipelineEventStageFinish      telemetry.EventType = "pipeline_stage_finish"
	pipelineEventStageDecodeError telemetry.EventType = "pipeline_stage_decode_error"
	pipelineEventStageValidError  telemetry.EventType = "pipeline_stage_validation_error"
)

// emitStageEvent sends a structured stage event when telemetry is configured.
// ctx carries the turn's correlation identifiers, stamped onto the event before
// it reaches the sink.
func emitStageEvent(ctx context.Context, sink telemetry.Telemetry, eventType telemetry.EventType, taskID, stageName, message string, metadata map[string]any) {
	if sink == nil {
		return
	}
	ev := telemetry.Event{
		Type:      eventType,
		NodeID:    stageName,
		TaskID:    taskID,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	sink.Emit(ev)
}

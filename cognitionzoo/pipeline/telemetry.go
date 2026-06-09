package pipeline

import (
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
func emitStageEvent(sink telemetry.Telemetry, eventType telemetry.EventType, taskID, stageName, message string, metadata map[string]any) {
	if sink == nil {
		return
	}
	sink.Emit(telemetry.Event{
		Type:      eventType,
		NodeID:    stageName,
		TaskID:    taskID,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

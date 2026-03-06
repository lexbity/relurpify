package pipeline

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const (
	pipelineEventStageStart       core.EventType = "pipeline_stage_start"
	pipelineEventStageFinish      core.EventType = "pipeline_stage_finish"
	pipelineEventStageDecodeError core.EventType = "pipeline_stage_decode_error"
	pipelineEventStageValidError  core.EventType = "pipeline_stage_validation_error"
)

// emitStageEvent sends a structured stage event when telemetry is configured.
func emitStageEvent(telemetry core.Telemetry, eventType core.EventType, taskID, stageName, message string, metadata map[string]any) {
	if telemetry == nil {
		return
	}
	telemetry.Emit(core.Event{
		Type:      eventType,
		NodeID:    stageName,
		TaskID:    taskID,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

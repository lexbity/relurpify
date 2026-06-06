package blackboard

import (
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const maxBlackboardAuditEntries = 32

func emitBlackboardEvent(sink telemetry.Telemetry, state *contextdata.Envelope, eventType telemetry.EventType, nodeID, taskID, message string, metadata map[string]any) {
	if state != nil {
		appendBlackboardAudit(state, strings.TrimSpace(message), metadata)
	}
	if sink == nil {
		return
	}
	sink.Emit(telemetry.Event{
		Type:      eventType,
		NodeID:    strings.TrimSpace(nodeID),
		TaskID:    strings.TrimSpace(taskID),
		Message:   strings.TrimSpace(message),
		Timestamp: time.Now().UTC(),
		Metadata:  cloneTelemetryMetadata(metadata),
	})
}

func appendBlackboardAudit(state *contextdata.Envelope, message string, metadata map[string]any) {
	if state == nil {
		return
	}
	entry := map[string]any{
		"message":   strings.TrimSpace(message),
		"timestamp": time.Now().UTC(),
		"metadata":  cloneTelemetryMetadata(metadata),
	}
	raw, _ := envelopeGet(state, contextKeyAuditTrail)
	existing, _ := raw.([]map[string]any)
	next := append(append([]map[string]any(nil), existing...), entry)
	if len(next) > maxBlackboardAuditEntries {
		next = next[len(next)-maxBlackboardAuditEntries:]
	}
	envelopeSet(state, contextKeyAuditTrail, next)
}

func cloneTelemetryMetadata(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

package blackboard

import (
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const maxBlackboardAuditEntries = 32

func emitBlackboardEvent(telemetry core.Telemetry, state *core.Context, eventType core.EventType, nodeID, taskID, message string, metadata map[string]any) {
	if state != nil {
		appendBlackboardAudit(state, strings.TrimSpace(message), metadata)
	}
	if telemetry == nil {
		return
	}
	telemetry.Emit(core.Event{
		Type:      eventType,
		NodeID:    strings.TrimSpace(nodeID),
		TaskID:    strings.TrimSpace(taskID),
		Message:   strings.TrimSpace(message),
		Timestamp: time.Now().UTC(),
		Metadata:  cloneTelemetryMetadata(metadata),
	})
}

func appendBlackboardAudit(state *core.Context, message string, metadata map[string]any) {
	if state == nil {
		return
	}
	entry := map[string]any{
		"message":    strings.TrimSpace(message),
		"timestamp":  time.Now().UTC(),
		"metadata":   cloneTelemetryMetadata(metadata),
	}
	raw, _ := state.Get(contextKeyAuditTrail)
	existing, _ := raw.([]map[string]any)
	next := append(append([]map[string]any(nil), existing...), entry)
	if len(next) > maxBlackboardAuditEntries {
		next = next[len(next)-maxBlackboardAuditEntries:]
	}
	state.Set(contextKeyAuditTrail, next)
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

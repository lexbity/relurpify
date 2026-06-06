package telemetry

import (
	"context"
	"encoding/json"
	"time"

	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/governance/identity"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// EventTelemetry mirrors legacy telemetry events into the framework event log.
type EventTelemetry struct {
	Log       event.Log
	Partition string
	Actor     identity.EventActor
	Clock     func() time.Time
}

func (e EventTelemetry) Emit(ev telemetry.Event) {
	if e.Log == nil {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	when := ev.Timestamp
	if when.IsZero() {
		when = e.now()
	}
	_, _ = e.Log.Append(context.Background(), e.partition(), []telemetry.FrameworkEvent{{
		Timestamp: when.UTC(),
		Type:      e.mapEventType(ev),
		Payload:   payload,
		Actor:     e.actor(),
		Partition: e.partition(),
	}})
}

// EmitHITLEvent records a human-in-the-loop lifecycle event. resolved selects
// the resolved-vs-requested framework event type; ev is marshaled as-is. The
// concrete HITL event type is owned by the authorization domain; telemetry stays
// decoupled from it so the dependency points one way (authorization ->
// telemetry, never the reverse).
func (e EventTelemetry) EmitHITLEvent(resolved bool, ev any) {
	if e.Log == nil {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	eventType := telemetry.FrameworkEventHITLRequested
	if resolved {
		eventType = telemetry.FrameworkEventHITLResolved
	}
	_, _ = e.Log.Append(context.Background(), e.partition(), []telemetry.FrameworkEvent{{
		Timestamp: e.now().UTC(),
		Type:      eventType,
		Payload:   payload,
		Actor:     e.actor(),
		Partition: e.partition(),
	}})
}

func (e EventTelemetry) partition() string {
	if e.Partition == "" {
		return "local"
	}
	return e.Partition
}

func (e EventTelemetry) actor() identity.EventActor {
	if e.Actor.Kind == "" && e.Actor.ID == "" {
		return identity.EventActor{Kind: "system", ID: "relurpify"}
	}
	return e.Actor
}

func (e EventTelemetry) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now().UTC()
}

func (e EventTelemetry) mapEventType(ev telemetry.Event) string {
	switch ev.Type {
	case telemetry.EventAgentStart:
		return telemetry.FrameworkEventAgentRunStarted
	case telemetry.EventAgentFinish:
		if status, ok := metadataValue(ev.Metadata, "status"); ok && status == "failed" {
			return telemetry.FrameworkEventAgentRunFailed
		}
		return telemetry.FrameworkEventAgentRunCompleted
	case telemetry.EventLLMPrompt:
		return telemetry.FrameworkEventLLMRequested
	case telemetry.EventLLMResponse:
		return telemetry.FrameworkEventLLMResponded
	case telemetry.EventCapabilityCall, telemetry.EventToolCall:
		return telemetry.FrameworkEventCapabilityInvoked
	case telemetry.EventCapabilityResult, telemetry.EventToolResult:
		return telemetry.FrameworkEventCapabilityResult
	default:
		return "telemetry." + string(ev.Type) + ".v1"
	}
}

func metadataValue(metadata map[string]interface{}, key string) (string, bool) {
	if metadata == nil {
		return "", false
	}
	value, ok := metadata[key]
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	return s, ok
}

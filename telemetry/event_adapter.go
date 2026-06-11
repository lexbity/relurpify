package telemetry

import (
	"context"
	"encoding/json"
	"time"

	"codeburg.org/lexbit/relurpify/governance/identity"
	evt "codeburg.org/lexbit/relurpify/telemetry/event"
)

// EventTelemetry mirrors legacy telemetry events into the framework event log.
type EventTelemetry struct {
	Log       evt.Log
	Partition string
	Actor     identity.EventActor
	Clock     func() time.Time
}

func (e EventTelemetry) Emit(ev Event) {
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
	_, _ = e.Log.Append(context.Background(), e.partition(), []evt.FrameworkEvent{{
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
func (e EventTelemetry) EmitHITLEvent(ctx context.Context, resolved bool, ev any) {
	if e.Log == nil {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	eventType := evt.EventHITLRequested
	if resolved {
		eventType = evt.EventHITLResolved
	}
	_, _ = e.Log.Append(ctx, e.partition(), []evt.FrameworkEvent{{
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

func (e EventTelemetry) mapEventType(ev Event) string {
	switch ev.Type {
	case EventAgentStart:
		return evt.EventAgentRunStarted
	case EventAgentFinish:
		if status, ok := metadataValue(ev.Metadata, "status"); ok && status == "failed" {
			return evt.EventAgentRunFailed
		}
		return evt.EventAgentRunCompleted
	case EventLLMPrompt:
		return evt.EventLLMRequested
	case EventLLMResponse:
		return evt.EventLLMResponded
	case EventCapabilityCall, EventToolCall:
		return evt.EventCapabilityInvoked
	case EventCapabilityResult, EventToolResult:
		return evt.EventCapabilityResult
	default:
		return "telemetry." + string(ev.Type) + ".v1"
	}
}

func metadataValue(metadata map[string]any, key string) (string, bool) {
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

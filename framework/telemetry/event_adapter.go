package telemetry

import (
	"context"
	"encoding/json"
	"time"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
)

// EventTelemetry mirrors legacy telemetry events into the framework event log.
type EventTelemetry struct {
	Log       event.Log
	Partition string
	Actor     core.EventActor
	Clock     func() time.Time
}

func (e EventTelemetry) Emit(ev core.Event) {
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
	_, _ = e.Log.Append(context.Background(), e.partition(), []core.FrameworkEvent{{
		Timestamp: when.UTC(),
		Type:      e.mapEventType(ev),
		Payload:   payload,
		Actor:     e.actor(),
		Partition: e.partition(),
	}})
}

func (e EventTelemetry) EmitHITLEvent(ev fauthorization.HITLEvent) {
	if e.Log == nil {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	eventType := core.FrameworkEventHITLRequested
	switch ev.Type {
	case fauthorization.HITLEventResolved, fauthorization.HITLEventExpired:
		eventType = core.FrameworkEventHITLResolved
	}
	_, _ = e.Log.Append(context.Background(), e.partition(), []core.FrameworkEvent{{
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

func (e EventTelemetry) actor() core.EventActor {
	if e.Actor.Kind == "" && e.Actor.ID == "" {
		return core.EventActor{Kind: "system", ID: "relurpify"}
	}
	return e.Actor
}

func (e EventTelemetry) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now().UTC()
}

func (e EventTelemetry) mapEventType(ev core.Event) string {
	switch ev.Type {
	case core.EventAgentStart:
		return core.FrameworkEventAgentRunStarted
	case core.EventAgentFinish:
		if status, ok := metadataValue(ev.Metadata, "status"); ok && status == "failed" {
			return core.FrameworkEventAgentRunFailed
		}
		return core.FrameworkEventAgentRunCompleted
	case core.EventLLMPrompt:
		return core.FrameworkEventLLMRequested
	case core.EventLLMResponse:
		return core.FrameworkEventLLMResponded
	case core.EventCapabilityCall, core.EventToolCall:
		return core.FrameworkEventCapabilityInvoked
	case core.EventCapabilityResult, core.EventToolResult:
		return core.FrameworkEventCapabilityResult
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

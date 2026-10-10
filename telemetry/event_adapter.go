package telemetry

import (
	evt "codeburg.org/lexbit/relurpify/telemetry/event"
	"context"
	"encoding/json"
	"time"
)

// EventTelemetry mirrors telemetry events into the framework event log.
type EventTelemetry struct {
	Log       evt.Log
	Partition string
	Actor     evt.Actor
	Clock     func() time.Time
}

// NewEventTelemetry assembles the causal mirror from platform-neutral
// identity inputs. The Actor type stays inside telemetry so
// domain packages (execution, governance, capability) do not need the
// platform import to participate in the causal record.
func NewEventTelemetry(log evt.Log, partition, agentID, label string) EventTelemetry {
	actor := evt.Actor{Kind: "agent", ID: agentID, Label: label}
	return EventTelemetry{
		Log:       log,
		Partition: partition,
		Actor:     actor,
	}
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
	fev := evt.FrameworkEvent{
		Timestamp: when.UTC(),
		Type:      e.mapEventType(ev),
		Payload:   payload,
		Actor:     e.actor(),
		Partition: e.partition(),
	}
	// FR-4: carried CausedBy chains stay intact through the mirror. The
	// causal reference is expressed as the standard metadata key so the
	// telemetry Event schema stays domain-agnostic.
	if causedBy, ok := ev.Metadata["caused_by"].([]uint64); ok {
		fev.CausedBy = causedBy
	}
	_, _ = e.Log.Append(context.Background(), e.partition(), []evt.FrameworkEvent{fev})
}

func (e EventTelemetry) partition() string {
	if e.Partition == "" {
		return "local"
	}
	return e.Partition
}

func (e EventTelemetry) actor() evt.Actor {
	if e.Actor.Kind == "" && e.Actor.ID == "" {
		return evt.Actor{Kind: "system", ID: "relurpify"}
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
	case EventChunkCommitted:
		return evt.EventChunkCommitted
	case EventSummaryCommitted:
		return evt.EventSummaryCommitted
	// Decision forensics map onto their canonical .v1 spellings so the
	// causal record stays queryable without a second writer (FR-5, FR-6).
	case EventPolicyEvaluated:
		return evt.EventPolicyEvaluated
	case EventHITLRequested:
		return evt.EventHITLRequested
	case EventHITLResolved:
		// The lifecycle outcome decides the record type: expired requests
		// land in their own .v1 slot for consumption exactly like approved
		// and denied ones.
		if outcome, ok := metadataValue(ev.Metadata, "outcome"); ok && outcome == "expired" {
			return evt.EventHITLExpired
		}
		return evt.EventHITLResolved
	case EventDoomLoopDetected:
		return evt.EventDoomLoopDetected
	default:
		return "" + string(ev.Type) + ".v1"
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

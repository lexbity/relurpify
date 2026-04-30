package reporting

import (
	"time"
)

// EventType defines the type of reporting event.
type EventType string

const (
	EventTypeTaskStarted       EventType = "task_started"
	EventTypeTaskCompleted     EventType = "task_completed"
	EventTypeTaskFailed        EventType = "task_failed"
	EventTypeStepStarted       EventType = "step_started"
	EventTypeStepCompleted     EventType = "step_completed"
	EventTypeStepFailed        EventType = "step_failed"
	EventTypeFrameEmitted      EventType = "frame_emitted"
	EventTypeFrameResolved     EventType = "frame_resolved"
	EventTypeCapabilityInvoked EventType = "capability_invoked"
)

const (
	EventTypeRouteSelected    EventType = "euclo.route.selected"
	EventTypeRouteCompleted   EventType = "euclo.route.completed"
	EventTypeRouteUnavailable EventType = "euclo.route.unavailable"
	EventTypeRouteDryRun      EventType = "euclo.route.dry_run"
	EventTypeRouteFallback    EventType = "euclo.route.fallback"
)

// Event represents a reporting event.
type Event struct {
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	TaskID    string            `json:"task_id"`
	SessionID string            `json:"session_id"`
	Data      map[string]any    `json:"data"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func mergeEventData(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// EventEmitter defines the interface for emitting events.
type EventEmitter interface {
	Emit(event *Event) error
}

// InMemoryEventEmitter is a simple in-memory event emitter.
type InMemoryEventEmitter struct {
	events []*Event
}

// NewInMemoryEventEmitter creates a new in-memory event emitter.
func NewInMemoryEventEmitter() *InMemoryEventEmitter {
	return &InMemoryEventEmitter{
		events: make([]*Event, 0),
	}
}

// Emit stores an event in memory.
func (e *InMemoryEventEmitter) Emit(event *Event) error {
	e.events = append(e.events, event)
	return nil
}

// Events returns all stored events.
func (e *InMemoryEventEmitter) Events() []*Event {
	return e.events
}

// Clear clears all stored events.
func (e *InMemoryEventEmitter) Clear() {
	e.events = make([]*Event, 0)
}

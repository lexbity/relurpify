package telemetry

import "time"

// Event is the platform-local telemetry envelope used by shell tooling.
type Event struct {
	Type      string
	Message   string
	Timestamp time.Time
	Metadata  map[string]any
}

// Sink receives platform-local telemetry events.
type Sink interface {
	Emit(event Event)
}

package telemetry

import (
	"testing"
)

// TestMultiplexTelemetryFansOutToAllSinks proves the multiplex delivers every
// event to every sink in order.
func TestMultiplexTelemetryFansOutToAllSinks(t *testing.T) {
	first := &muxProbeSink{}
	second := &muxProbeSink{}
	mux := MultiplexTelemetry{Sinks: []Telemetry{first, second}}

	mux.Emit(Event{Type: EventGraphStart, TaskID: "one"})
	mux.Emit(Event{Type: EventGraphFinish, TaskID: "two"})

	for name, sink := range map[string]*muxProbeSink{"first": first, "second": second} {
		if len(sink.events) != 2 {
			t.Fatalf("%s sink received %d events, want 2", name, len(sink.events))
		}
		if sink.events[0].Type != EventGraphStart || sink.events[1].Type != EventGraphFinish {
			t.Fatalf("%s sink events out of order: %v", name, sink.events)
		}
	}
}

// TestMultiplexTelemetryEmptySinksNeverPanics covers the zero-sink shape used
// by disabled telemetry paths.
func TestMultiplexTelemetryEmptySinksNeverPanics(t *testing.T) {
	mux := MultiplexTelemetry{}
	mux.Emit(Event{Type: EventGraphStart})
}

// muxProbeSink is a minimal Telemetry capturing emitted events.
type muxProbeSink struct {
	events []Event
}

func (r *muxProbeSink) Emit(event Event) {
	r.events = append(r.events, event)
}

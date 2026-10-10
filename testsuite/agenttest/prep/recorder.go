package prep

import (
	"sync"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// recordingSink captures every telemetry event in memory for the duration of
// the dry run, mirroring the live harness's recording sink (same interface,
// own copy — the live one is package-private).
type recordingSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *recordingSink) Emit(event telemetry.Event) {
	if event.Type == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

// Events returns the recorded events in emission order.
func (s *recordingSink) Events() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]telemetry.Event(nil), s.events...)
}

// ofType returns all recorded events with the given type.
func (s *recordingSink) ofType(eventType telemetry.EventType) []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []telemetry.Event
	for _, ev := range s.events {
		if ev.Type == eventType {
			out = append(out, ev)
		}
	}
	return out
}

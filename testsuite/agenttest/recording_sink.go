package agenttest

import (
	"sync"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// recordingTelemetrySink is the in-memory telemetry sink used by the e2e
// harness. It replaces the former no-op sink: events emitted by the agent,
// the capability registry, and the LLM instrumentation are captured so the
// CaseReport and the OSB security/benchmark evaluators are derived from real
// telemetry rather than from assumptions (FR-8).
//
// Emission is thread-safe because tool execution and the LLM instrumentation
// emit from independent goroutines.
type recordingTelemetrySink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func newRecordingTelemetrySink() *recordingTelemetrySink {
	return &recordingTelemetrySink{}
}

// Emit captures one telemetry event.
func (r *recordingTelemetrySink) Emit(event telemetry.Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

// Events returns a copy of every event captured so far.
func (r *recordingTelemetrySink) Events() []telemetry.Event {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]telemetry.Event(nil), r.events...)
}

// Len reports how many events have been captured.
func (r *recordingTelemetrySink) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// Reset drops every captured event.
func (r *recordingTelemetrySink) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

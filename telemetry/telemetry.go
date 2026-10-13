package telemetry

import (
	"context"
)

// MultiplexTelemetry broadcasts events to multiple sinks.
type MultiplexTelemetry struct {
	Sinks []Telemetry
}

// StampCorrelation populates the first-class correlation fields on ev from the
// correlation identifiers carried by ctx. It is the single sanctioned way to
// populate SessionID/RunID/TraceID/AgentID/SpanID (NFR-6): emitters must never
// construct those fields by hand, and must never smuggle them through Metadata.
//
// Merge semantics come from CorrelationFromContext (single source of truth):
//
//   - A non-empty RunContext value overwrites the corresponding field, so every
//     event emitted inside a turn agrees on the turn's identifiers.
//   - An empty RunContext value never clears a field the caller already set, so
//     an emitter that legitimately knows a SessionID is not downgraded.
//   - TraceID is turn-scoped (Decision 3): when a RunContext is present it wins
//     over any node-local TraceContext. TraceContext.SpanID is per-node and is
//     always stamped when present; a TraceContext TraceID is only used as a
//     fallback when no turn-scoped TraceID is known.
//   - NodeID is only filled when the event does not already carry one: the
//     graph runtime stamps the authoritative executing node onto the context,
//     so a caller-supplied NodeID is preserved unless context disagrees.
//
// Emit has no context parameter (the Telemetry interface is unchanged), so
// stamping happens at the call site immediately before Emit. That is the only
// place the caller's context is reachable.
func StampCorrelation(ctx context.Context, ev *Event) {
	if ev == nil {
		return
	}
	c := CorrelationFromContext(ctx)
	if c.SessionID != "" {
		ev.SessionID = c.SessionID
	}
	if c.RunID != "" {
		ev.RunID = c.RunID
	}
	if c.TraceIDTurnScoped {
		ev.TraceID = c.TraceID
	} else if c.TraceID != "" && ev.TraceID == "" {
		ev.TraceID = c.TraceID
	}
	if c.AgentID != "" {
		ev.AgentID = c.AgentID
	}
	if c.SpanID != "" {
		ev.SpanID = c.SpanID
	}
	if c.NodeID != "" && ev.NodeID == "" {
		ev.NodeID = c.NodeID
	}
}

// Emit forwards the event to all registered sinks. Correlation fields are
// stamped by the caller via StampCorrelation before the event reaches the
// multiplex; see the StampCorrelation doc comment for why.
func (m MultiplexTelemetry) Emit(event Event) {
	for _, s := range m.Sinks {
		s.Emit(event)
	}
}

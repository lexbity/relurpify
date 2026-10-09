package telemetry

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/observability"
	"github.com/stretchr/testify/require"
)

// recordingSink is a test sink that captures emitted events.
type recordingSink struct {
	events []Event
}

func (r *recordingSink) Emit(ev Event) {
	r.events = append(r.events, ev)
}

// TestMultiplexTelemetryEmitForwardsCorrelation verifies that MultiplexTelemetry
// forwards events with correlation fields intact. The caller is responsible for
// calling StampCorrelation before Emit; MultiplexTelemetry must not strip or
// overwrite those fields.
func TestMultiplexTelemetryEmitForwardsCorrelation(t *testing.T) {
	sink := &recordingSink{}
	mux := MultiplexTelemetry{Sinks: []Telemetry{sink}}

	rc := observability.RunContext{SessionID: "sess-1", RunID: "run-1", TraceID: "trace-1", AgentID: "agent-1"}
	ctx := observability.WithRunContext(context.Background(), rc)

	ev := Event{
		Type: EventAgentStart,
	}
	StampCorrelation(ctx, &ev)
	mux.Emit(ev)

	require.Len(t, sink.events, 1)
	require.Equal(t, "sess-1", sink.events[0].SessionID)
	require.Equal(t, "run-1", sink.events[0].RunID)
	require.Equal(t, "trace-1", sink.events[0].TraceID)
	require.Equal(t, "agent-1", sink.events[0].AgentID)
}

// TestMultiplexTelemetryEmitMultipleSinks verifies that MultiplexTelemetry
// forwards events to all registered sinks.
func TestMultiplexTelemetryEmitMultipleSinks(t *testing.T) {
	sink1 := &recordingSink{}
	sink2 := &recordingSink{}
	mux := MultiplexTelemetry{Sinks: []Telemetry{sink1, sink2}}

	ev := Event{
		Type: EventAgentStart,
	}
	mux.Emit(ev)

	require.Len(t, sink1.events, 1)
	require.Len(t, sink2.events, 1)
	require.Equal(t, ev, sink1.events[0])
	require.Equal(t, ev, sink2.events[0])
}

// TestStampCorrelationWithTraceContext verifies that StampCorrelation also
// stamps SpanID from observability.TraceContext when present.
func TestStampCorrelationWithTraceContext(t *testing.T) {
	rc := observability.RunContext{SessionID: "sess-1", RunID: "run-1", TraceID: "trace-1", AgentID: "agent-1"}
	tc := observability.TraceContext{TraceID: "node-trace", SpanID: "span-1"}
	ctx := observability.WithRunContext(context.Background(), rc)
	ctx = observability.WithTraceContext(ctx, tc)

	ev := &Event{
		Type: EventAgentStart,
	}
	StampCorrelation(ctx, ev)

	// observability.RunContext wins for TraceID (Decision 3)
	require.Equal(t, "trace-1", ev.TraceID)
	// SpanID comes from observability.TraceContext
	require.Equal(t, "span-1", ev.SpanID)
}

// TestStampCorrelationTraceContextFallback verifies that observability.TraceContext.TraceID
// is used as a fallback when no observability.RunContext is present.
func TestStampCorrelationTraceContextFallback(t *testing.T) {
	tc := observability.TraceContext{TraceID: "node-trace", SpanID: "span-1"}
	ctx := observability.WithTraceContext(context.Background(), tc)

	ev := &Event{
		Type: EventAgentStart,
	}
	StampCorrelation(ctx, ev)

	require.Equal(t, "node-trace", ev.TraceID)
	require.Equal(t, "span-1", ev.SpanID)
}

// TestStampCorrelationEmptyRunContextPreservesExisting verifies that an empty
// observability.RunContext value never clears a field the caller already set.
func TestStampCorrelationEmptyRunContextPreservesExisting(t *testing.T) {
	ctx := observability.WithRunContext(context.Background(), observability.RunContext{})

	ev := &Event{
		Type:      EventAgentStart,
		SessionID: "existing-sess",
		RunID:     "existing-run",
	}
	StampCorrelation(ctx, ev)

	require.Equal(t, "existing-sess", ev.SessionID)
	require.Equal(t, "existing-run", ev.RunID)
}

// TestStampCorrelationNilEvent verifies that StampCorrelation handles nil events.
func TestStampCorrelationNilEvent(t *testing.T) {
	StampCorrelation(context.Background(), nil)
	// Should not panic
}

func TestCorrelationFromContext_MergesAllCarriers(t *testing.T) {
	rc := observability.RunContext{SessionID: "sess-1", RunID: "run-1", TraceID: "trace-1", AgentID: "agent-1"}
	ctx := observability.WithRunContext(context.Background(), rc)
	ctx = observability.WithTraceContext(ctx, observability.TraceContext{TraceID: "node-trace", SpanID: "span-1"})
	ctx = observability.WithNodeContext(ctx, "node-1")

	c := observability.CorrelationFromContext(ctx)

	require.Equal(t, "sess-1", c.SessionID)
	require.Equal(t, "run-1", c.RunID)
	require.Equal(t, "trace-1", c.TraceID, "turn-scoped TraceID wins")
	require.True(t, c.TraceIDTurnScoped)
	require.Equal(t, "agent-1", c.AgentID)
	require.Equal(t, "node-1", c.NodeID)
	require.Equal(t, "span-1", c.SpanID)
}

func TestCorrelationFromContext_TraceContextFallbackRules(t *testing.T) {
	// Without a observability.RunContext, observability.TraceContext supplies TraceID as fallback.
	c := observability.CorrelationFromContext(observability.WithTraceContext(context.Background(), observability.TraceContext{TraceID: "node-trace", SpanID: "span-1"}))
	require.Equal(t, "node-trace", c.TraceID)
	require.False(t, c.TraceIDTurnScoped)
	require.True(t, c.HasTraceID())

	// With a turn-scoped TraceID present, the observability.TraceContext TraceID is not
	// promoted; it stays a fallback the stamper applies only when the event
	// is missing a TraceID.
	c = observability.CorrelationFromContext(observability.WithTraceContext(
		observability.WithRunContext(context.Background(), observability.RunContext{RunID: "run-1"}),
		observability.TraceContext{TraceID: "node-trace"},
	))
	require.Equal(t, "node-trace", c.TraceID, "fallback is still resolved for events without a TraceID")
	require.False(t, c.TraceIDTurnScoped, "must not be marked turn-scoped, so it never overwrites")
}

func TestCorrelationFromContext_NoCarriers(t *testing.T) {
	c := observability.CorrelationFromContext(context.Background())
	require.Equal(t, observability.Correlation{}, c)
	require.False(t, c.HasTraceID())

	require.Equal(t, observability.Correlation{}, observability.CorrelationFromContext(nil))
}

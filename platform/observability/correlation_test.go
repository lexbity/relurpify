package observability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithTraceContextRoundTrip(t *testing.T) {
	tc := TraceContext{TraceID: "abc123", SpanID: "def456"}
	ctx := WithTraceContext(context.Background(), tc)
	got, ok := TraceContextFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "abc123", got.TraceID)
	require.Equal(t, "def456", got.SpanID)
}

func TestTraceContextFromNilContext(t *testing.T) {
	_, ok := TraceContextFromContext(context.TODO())
	require.False(t, ok)
}

func TestTraceContextFromEmptyContext(t *testing.T) {
	_, ok := TraceContextFromContext(context.Background())
	require.False(t, ok)
}

func TestNewTraceIDIsNonEmpty(t *testing.T) {
	id := NewTraceID()
	require.NotEmpty(t, id, "trace ID must not be empty")
	require.Len(t, id, 32, "trace ID must be 32 hex chars (16 bytes)")
}

func TestNewSpanIDIsNonEmpty(t *testing.T) {
	id := NewSpanID()
	require.NotEmpty(t, id, "span ID must not be empty")
	require.Len(t, id, 16, "span ID must be 16 hex chars (8 bytes)")
}

func TestTraceContextIsUnique(t *testing.T) {
	ids := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		id := NewTraceID()
		require.NotContains(t, ids, id, "trace IDs must be unique")
		ids[id] = struct{}{}
	}
}

func TestWithRunContextRoundTrip(t *testing.T) {
	rc := RunContext{SessionID: "sess-1", RunID: "run-1", TraceID: "trace-1", AgentID: "agent-1"}
	ctx := WithRunContext(context.Background(), rc)
	got, ok := RunContextFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "sess-1", got.SessionID)
	require.Equal(t, "run-1", got.RunID)
	require.Equal(t, "trace-1", got.TraceID)
	require.Equal(t, "agent-1", got.AgentID)
}

func TestRunContextFromNilContext(t *testing.T) {
	_, ok := RunContextFromContext(context.TODO())
	require.False(t, ok)
}

func TestRunContextFromEmptyContext(t *testing.T) {
	_, ok := RunContextFromContext(context.Background())
	require.False(t, ok)
}

func TestStampCorrelationPopulatesFields(t *testing.T) {
	rc := RunContext{SessionID: "sess-1", RunID: "run-1", TraceID: "trace-1", AgentID: "agent-1"}
	ctx := WithRunContext(context.Background(), rc)

	ev := &Event{
		Type: EventAgentStart,
	}
	StampCorrelation(ctx, ev)

	require.Equal(t, "sess-1", ev.SessionID)
	require.Equal(t, "run-1", ev.RunID)
	require.Equal(t, "trace-1", ev.TraceID)
	require.Equal(t, "agent-1", ev.AgentID)
}

func TestStampCorrelationOverwritesWithRunContext(t *testing.T) {
	rc := RunContext{SessionID: "sess-1", RunID: "run-1", TraceID: "trace-1", AgentID: "agent-1"}
	ctx := WithRunContext(context.Background(), rc)

	ev := &Event{
		Type:      EventAgentStart,
		SessionID: "existing-sess",
		RunID:     "existing-run",
	}
	StampCorrelation(ctx, ev)

	// RunContext should win (per NFR-6: emitters must never construct those fields by hand)
	require.Equal(t, "sess-1", ev.SessionID)
	require.Equal(t, "run-1", ev.RunID)
	require.Equal(t, "trace-1", ev.TraceID)
	require.Equal(t, "agent-1", ev.AgentID)
}

func TestStampCorrelationNoOpWhenNoContext(t *testing.T) {
	ev := &Event{
		Type: EventAgentStart,
	}
	StampCorrelation(context.Background(), ev)

	require.Empty(t, ev.SessionID)
	require.Empty(t, ev.RunID)
	require.Empty(t, ev.TraceID)
	require.Empty(t, ev.AgentID)
}

func TestWithNodeContextRoundTrip(t *testing.T) {
	ctx := WithNodeContext(context.Background(), "node-1")
	nodeID, ok := NodeIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "node-1", nodeID)
}

func TestNodeIDFromNilContext(t *testing.T) {
	_, ok := NodeIDFromContext(nil)
	require.False(t, ok)
}

func TestNodeIDFromEmptyContext(t *testing.T) {
	_, ok := NodeIDFromContext(context.Background())
	require.False(t, ok)
}

func TestWithNodeContextOnNilContext(t *testing.T) {
	ctx := WithNodeContext(nil, "node-2")
	nodeID, ok := NodeIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "node-2", nodeID)
}

package telemetry

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

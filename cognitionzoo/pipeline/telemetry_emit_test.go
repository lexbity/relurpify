package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestEmitStageEventNilSinkIsNoop(t *testing.T) {
	assert.NotPanics(t, func() {
		emitStageEvent(context.Background(), nil, pipelineEventStageStart, "t", "s", "", nil)
	})
}

func TestEmitStageEventStampsEventFields(t *testing.T) {
	sink := &recordingTelemetry{}
	metadata := map[string]any{"attempt": 2}
	emitStageEvent(context.Background(), sink, pipelineEventStageValidError, "task-7", "explore", "failed validation", metadata)

	events := sink.emitted()
	assert.Len(t, events, 1)
	ev := events[0]
	assert.Equal(t, pipelineEventStageValidError, ev.Type)
	assert.Equal(t, "explore", ev.NodeID)
	assert.Equal(t, "task-7", ev.TaskID)
	assert.Equal(t, "failed validation", ev.Message)
	assert.Equal(t, metadata, ev.Metadata)
	assert.False(t, ev.Timestamp.IsZero())
}

func TestEmitStageEventCarriesCorrelationFromContext(t *testing.T) {
	sink := &recordingTelemetry{}
	ctx := telemetry.WithTraceContext(
		telemetry.WithRunContext(context.Background(), telemetry.RunContext{
			SessionID: "sess-1",
			RunID:     "run-1",
			TraceID:   "trace-1",
		}),
		telemetry.TraceContext{TraceID: "ignored-turn-scoped", SpanID: "span-9"},
	)
	emitStageEvent(ctx, sink, pipelineEventStageStart, "t", "s", "", nil)

	events := sink.emitted()
	assert.Len(t, events, 1)
	assert.Equal(t, "sess-1", events[0].SessionID)
	assert.Equal(t, "run-1", events[0].RunID)
	assert.Equal(t, "trace-1", events[0].TraceID, "turn-scoped trace id wins")
	assert.Equal(t, "span-9", events[0].SpanID)
}

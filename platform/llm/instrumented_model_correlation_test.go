package llm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"

	"codeburg.org/lexbit/relurpify/telemetry"
)

type correlationSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *correlationSink) Emit(_ context.Context, ev any) {
	telemetryEv, ok := ev.(telemetry.Event)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, telemetryEv)
}

func (s *correlationSink) Snapshot() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]telemetry.Event, len(s.events))
	copy(out, s.events)
	return out
}

func llmCorrelationContext() (context.Context, *contextdata.Envelope) {
	rc := telemetry.RunContext{
		SessionID: "session-1",
		RunID:     "run-1",
		TraceID:   "trace-1",
		AgentID:   "agent-1",
	}
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetNodeID("node-1")
	ctx := telemetry.WithRunContext(
		contextdata.WithEnvelope(context.Background(), env),
		rc,
	)
	return ctx, env
}

func requireLLMCorrelated(t *testing.T, events []telemetry.Event) {
	t.Helper()
	require.NotEmpty(t, events)
	for _, ev := range events {
		switch ev.Type {
		case telemetry.EventLLMPrompt, telemetry.EventLLMResponse:
			require.Equal(t, "session-1", ev.SessionID, "session_id must be stamped")
			require.Equal(t, "run-1", ev.RunID, "run_id must be stamped")
			require.Equal(t, "trace-1", ev.TraceID, "trace_id must be stamped")
			require.Equal(t, "agent-1", ev.AgentID, "agent_id must be stamped")
			// TaskID/NodeID envelope backfill is asserted at the composition
			// root's EventSink adapter (Q9) — platform has no context edge.
			// Decision 1: Metadata is for domain payloads only — correlation
			// must live on the event's first-class fields, never duplicated
			// into Metadata.
			for key := range ev.Metadata {
				switch key {
				case "run_id", "trace_id", "agent_id", "node_id", "session_id", "task_id":
					require.Fail(t, "correlation must not be smuggled into Metadata", "key %q", key)
				}
			}
		}
	}
}

func TestInstrumentedModel_NodeContextStampsNodeID(t *testing.T) {
	sink := &correlationSink{}
	instrumented := NewInstrumentedModel(&profileAwareStubModel{}, sink, false)
	rc := telemetry.RunContext{SessionID: "session-n", RunID: "run-n", TraceID: "trace-n", AgentID: "agent-n"}
	ctx := telemetry.WithNodeContext(
		telemetry.WithRunContext(context.Background(), rc),
		"node-ctx",
	)

	_, err := instrumented.Generate(ctx, "prompt", nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(sink.Snapshot()) >= 2
	}, time.Second, 10*time.Millisecond)

	for _, ev := range sink.Snapshot() {
		require.Equal(t, "node-ctx", ev.NodeID, "node context must stamp first-class node_id")
	}
}

func TestInstrumentedModel_GenerateCarriesCorrelation(t *testing.T) {
	sink := &correlationSink{}
	instrumented := NewInstrumentedModel(&profileAwareStubModel{}, sink, false)
	ctx, _ := llmCorrelationContext()

	_, err := instrumented.Generate(ctx, "prompt", nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(sink.Snapshot()) >= 2
	}, time.Second, 10*time.Millisecond)

	prompt, response := categorize(sink.Snapshot())
	require.NotNil(t, prompt)
	require.NotNil(t, response)
	requireLLMCorrelated(t, []telemetry.Event{*prompt, *response})
}

func TestInstrumentedModel_ChatWithToolsCarriesCorrelation(t *testing.T) {
	sink := &correlationSink{}
	instrumented := NewInstrumentedModel(&profileAwareStubModel{}, sink, false)
	ctx, _ := llmCorrelationContext()

	_, err := instrumented.ChatWithTools(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(sink.Snapshot()) >= 2
	}, time.Second, 10*time.Millisecond)

	prompt, response := categorize(sink.Snapshot())
	require.NotNil(t, prompt)
	require.NotNil(t, response)
	requireLLMCorrelated(t, []telemetry.Event{*prompt, *response})
}

// The old task-attribution holder is gone: correlation is resolved once
// inside stampObservabilityCorrelation. These tests exercise its fallback
// rules directly.

func stampOnce(ctx context.Context) telemetry.Event {
	var ev telemetry.Event
	stampObservabilityCorrelation(ctx, &ev)
	return ev
}

func TestInstrumentedModel_TaskIDFallsBackWithoutEnvelope(t *testing.T) {
	rc := telemetry.RunContext{RunID: "run-2", TraceID: "trace-2"}
	ev := stampOnce(telemetry.WithRunContext(context.Background(), rc))

	require.Empty(t, ev.TaskID, "no envelope means no task attribution — it must stay empty, not wrong")
	require.Equal(t, "run-2", ev.RunID)
	require.Equal(t, "trace-2", ev.TraceID)
}

func TestStampObservabilityCorrelation_NilEvent(t *testing.T) {
	stampObservabilityCorrelation(context.Background(), nil)
}

func categorize(events []telemetry.Event) (prompt *telemetry.Event, response *telemetry.Event) {
	for i := range events {
		switch events[i].Type {
		case telemetry.EventLLMPrompt:
			prompt = &events[i]
		case telemetry.EventLLMResponse:
			response = &events[i]
		}
	}
	return prompt, response
}

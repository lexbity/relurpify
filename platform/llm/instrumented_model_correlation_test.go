package llm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/observability"
)

type correlationSink struct {
	mu     sync.Mutex
	events []observability.Event
}

func (s *correlationSink) Emit(ev observability.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *correlationSink) Snapshot() []observability.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]observability.Event, len(s.events))
	copy(out, s.events)
	return out
}

func llmCorrelationContext() (context.Context, *contextdata.Envelope) {
	rc := observability.RunContext{
		SessionID: "session-1",
		RunID:     "run-1",
		TraceID:   "trace-1",
		AgentID:   "agent-1",
	}
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.NodeID = "node-1"
	ctx := observability.WithRunContext(
		contextdata.WithEnvelope(context.Background(), env),
		rc,
	)
	return ctx, env
}

func requireLLMCorrelated(t *testing.T, events []observability.Event) {
	t.Helper()
	require.NotEmpty(t, events)
	for _, ev := range events {
		switch ev.Type {
		case observability.EventLLMPrompt, observability.EventLLMResponse:
			require.Equal(t, "session-1", ev.SessionID, "session_id must be stamped")
			require.Equal(t, "run-1", ev.RunID, "run_id must be stamped")
			require.Equal(t, "trace-1", ev.TraceID, "trace_id must be stamped")
			require.Equal(t, "agent-1", ev.AgentID, "agent_id must be stamped")
			require.Equal(t, "task-1", ev.TaskID, "task_id must come from the envelope")
			require.Equal(t, "node-1", ev.NodeID, "node_id must come from the envelope")
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
	rc := observability.RunContext{SessionID: "session-n", RunID: "run-n", TraceID: "trace-n", AgentID: "agent-n"}
	ctx := observability.WithNodeContext(
		observability.WithRunContext(context.Background(), rc),
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

func TestInstrumentedModel_NodeContextPreferredOverEnvelopeNodeID(t *testing.T) {
	sink := &correlationSink{}
	instrumented := NewInstrumentedModel(&profileAwareStubModel{}, sink, false)
	env := contextdata.NewEnvelope("task-p", "session-p")
	env.NodeID = "node-env"
	ctx := observability.WithNodeContext(
		contextdata.WithEnvelope(context.Background(), env),
		"node-ctx",
	)

	_, err := instrumented.Chat(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(sink.Snapshot()) >= 2
	}, time.Second, 10*time.Millisecond)

	for _, ev := range sink.Snapshot() {
		require.Equal(t, "node-ctx", ev.NodeID, "node context must win over the envelope NodeID")
	}
}

func TestInstrumentedModel_TaskIDAndNodeIDFromGraphContext(t *testing.T) {
	sink := &correlationSink{}
	instrumented := NewInstrumentedModel(&profileAwareStubModel{}, sink, false)
	// Simulate the context a graph node receives: turn correlation via
	// RunContext, the task envelope, and the active node via node context.
	env := contextdata.NewEnvelope("task-graph", "session-graph")
	ctx := contextdata.WithEnvelope(context.Background(), env)
	ctx = observability.WithRunContext(ctx, observability.RunContext{
		SessionID: "session-graph",
		RunID:     "run-graph",
		TraceID:   "trace-graph",
		AgentID:   "agent-graph",
	})
	ctx = observability.WithNodeContext(ctx, "euclo.think")

	_, err := instrumented.ChatWithTools(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(sink.Snapshot()) >= 2
	}, time.Second, 10*time.Millisecond)

	prompt, response := categorize(sink.Snapshot())
	require.NotNil(t, prompt)
	require.NotNil(t, response)
	for _, ev := range []observability.Event{*prompt, *response} {
		require.Equal(t, "task-graph", ev.TaskID)
		require.Equal(t, "euclo.think", ev.NodeID)
		require.Equal(t, "run-graph", ev.RunID)
		require.Equal(t, "trace-graph", ev.TraceID)
		require.Equal(t, "session-graph", ev.SessionID)
		require.Equal(t, "agent-graph", ev.AgentID)
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
	requireLLMCorrelated(t, []observability.Event{*prompt, *response})
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
	requireLLMCorrelated(t, []observability.Event{*prompt, *response})
}

// The old task-attribution holder is gone: correlation is resolved once
// inside stampObservabilityCorrelation. These tests exercise its fallback
// rules directly.

func stampOnce(ctx context.Context) observability.Event {
	var ev observability.Event
	stampObservabilityCorrelation(ctx, &ev)
	return ev
}

func TestInstrumentedModel_TaskIDFallsBackWithoutEnvelope(t *testing.T) {
	rc := observability.RunContext{RunID: "run-2", TraceID: "trace-2"}
	ev := stampOnce(observability.WithRunContext(context.Background(), rc))

	require.Empty(t, ev.TaskID, "no envelope means no task attribution — it must stay empty, not wrong")
	require.Equal(t, "run-2", ev.RunID)
	require.Equal(t, "trace-2", ev.TraceID)
}

func TestInstrumentedModel_TraceContextFallbackWhenNoTurnScope(t *testing.T) {
	ev := stampOnce(observability.WithTraceContext(
		contextdata.WithEnvelope(
			context.Background(),
			contextdata.NewEnvelope("task-3", "session-3"),
		),
		observability.TraceContext{TraceID: "trace-3", SpanID: "span-3"},
	))

	require.Equal(t, "task-3", ev.TaskID)
	require.Equal(t, "session-3", ev.SessionID, "envelope SessionID is the fallback when no RunContext is present")
	require.Equal(t, "trace-3", ev.TraceID)
	require.Equal(t, "span-3", ev.SpanID)
}

func TestInstrumentedModel_EnvelopeFieldsNeverDowngraded(t *testing.T) {
	env := contextdata.NewEnvelope("", "session-env")
	env.NodeID = "node-env"
	ctx := contextdata.WithEnvelope(context.Background(), env)
	ctx = observability.WithRunContext(ctx, observability.RunContext{
		RunID:     "run-4",
		TraceID:   "trace-4",
		AgentID:   "agent-4",
		SessionID: "session-ctx",
	})

	ev := stampOnce(ctx)
	// RunContext SessionID wins over the envelope's.
	require.Equal(t, "session-ctx", ev.SessionID)
	require.Equal(t, "run-4", ev.RunID)
	require.Equal(t, "trace-4", ev.TraceID)
	require.Equal(t, "agent-4", ev.AgentID)
	// The envelope's NodeID and SessionID fallbacks only fill empty fields.
	require.Equal(t, "node-env", ev.NodeID)
}

func TestStampObservabilityCorrelation_NilEvent(t *testing.T) {
	stampObservabilityCorrelation(context.Background(), nil)
}

func categorize(events []observability.Event) (prompt *observability.Event, response *observability.Event) {
	for i := range events {
		switch events[i].Type {
		case observability.EventLLMPrompt:
			prompt = &events[i]
		case observability.EventLLMResponse:
			response = &events[i]
		}
	}
	return prompt, response
}

package envcomposition

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/telemetry"
	"github.com/stretchr/testify/require"
)

// recordingTelemetry captures every event flowing through the chain.
type recordingTelemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (r *recordingTelemetry) Emit(ev telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recordingTelemetry) snapshot() []telemetry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]telemetry.Event, len(r.events))
	copy(out, r.events)
	return out
}

// promptStubModel generates a fixed response so the instrumented model emits
// a prompt and a response event per call.
type promptStubModel struct{}

func (promptStubModel) Generate(_ context.Context, _ string, _ *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func (promptStubModel) GenerateStream(context.Context, string, *llm.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (promptStubModel) Chat(_ context.Context, _ []model.Message, _ *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func (promptStubModel) ChatWithTools(_ context.Context, _ []model.Message, _ []llm.LLMToolSpec, _ *llm.LLMOptions) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func waitEvents(t *testing.T, r *recordingTelemetry, n int) []telemetry.Event {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.snapshot()) >= n }, time.Second, 10*time.Millisecond)
	return r.snapshot()
}

func categorizeEvents(events []telemetry.Event) (prompt *telemetry.Event, response *telemetry.Event) {
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

// TestEventSinkAdapter_EnvelopeBackfill is the adapter-level counterpart of
// the correlation suite the platform suite dropped (Q9): the node/session/
// task identity flows from the graph context envelope through the adapter.
func TestEventSinkAdapter_EnvelopeBackfill(t *testing.T) {
	rec := &recordingTelemetry{}
	sink := NewEventSinkAdapter(rec, IdentityTriple{})
	instrumented := llm.NewInstrumentedModel(promptStubModel{}, sink, false)

	// Simulate the context a graph node receives: the task envelope and the
	// active node via node context.
	env := contextdata.NewEnvelope("task-graph", "session-graph")
	ctx := contextdata.WithEnvelope(context.Background(), env)
	ctx = telemetry.WithNodeContext(ctx, "euclo.think")

	_, err := instrumented.ChatWithTools(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil, nil)
	require.NoError(t, err)

	prompt, response := categorizeEvents(waitEvents(t, rec, 2))
	require.NotNil(t, prompt)
	require.NotNil(t, response)
	for _, ev := range []telemetry.Event{*prompt, *response} {
		require.Equal(t, "task-graph", ev.TaskID, "task_id must come from the envelope via the adapter")
		require.Equal(t, "euclo.think", ev.NodeID, "node_id must come from the envelope via the adapter")
	}
}

// TestEventSinkAdapter_NodeContextPreferredOverEnvelope pins the merge rule:
// the graph runtime's node context wins over a stale envelope NodeID.
func TestEventSinkAdapter_NodeContextPreferredOverEnvelope(t *testing.T) {
	rec := &recordingTelemetry{}
	sink := NewEventSinkAdapter(rec, IdentityTriple{})
	instrumented := llm.NewInstrumentedModel(promptStubModel{}, sink, false)

	env := contextdata.NewEnvelope("task-p", "session-p")
	env.SetNodeID("node-env")
	ctx := telemetry.WithNodeContext(
		contextdata.WithEnvelope(context.Background(), env),
		"node-ctx",
	)

	_, err := instrumented.Chat(ctx, []model.Message{{Role: "user", Content: "ping"}}, nil)
	require.NoError(t, err)

	for _, ev := range waitEvents(t, rec, 2) {
		require.Equal(t, "node-ctx", ev.NodeID, "node context must win over the envelope NodeID")
	}
}

// TestEventSinkAdapter_SessionIDFallback pins the merge rule: the envelope's
// SessionID is the fallback when no turn-scoped RunContext is present.
func TestEventSinkAdapter_SessionIDFallback(t *testing.T) {
	rec := &recordingTelemetry{}
	sink := NewEventSinkAdapter(rec, IdentityTriple{})
	instrumented := llm.NewInstrumentedModel(promptStubModel{}, sink, false)

	ctx := contextdata.WithEnvelope(
		context.Background(),
		contextdata.NewEnvelope("task-3", "session-3"),
	)

	_, err := instrumented.Generate(ctx, "prompt", nil)
	require.NoError(t, err)

	prompt, _ := categorizeEvents(waitEvents(t, rec, 2))
	require.NotNil(t, prompt)
	require.Equal(t, "session-3", prompt.SessionID, "envelope SessionID is the fallback when no RunContext is present")
}

// TestEventSinkAdapter_EnvelopeFieldsNeverDowngraded pins the merge rule:
// the turn-scoped RunContext wins; envelope fallbacks only fill empty fields.
func TestEventSinkAdapter_EnvelopeFieldsNeverDowngraded(t *testing.T) {
	rec := &recordingTelemetry{}
	sink := NewEventSinkAdapter(rec, IdentityTriple{})
	instrumented := llm.NewInstrumentedModel(promptStubModel{}, sink, false)

	env := contextdata.NewEnvelope("", "session-env")
	env.SetNodeID("node-env")
	ctx := contextdata.WithEnvelope(context.Background(), env)
	ctx = telemetry.WithRunContext(ctx, telemetry.RunContext{
		RunID:     "run-4",
		TraceID:   "trace-4",
		AgentID:   "agent-4",
		SessionID: "session-ctx",
	})

	_, err := instrumented.Generate(ctx, "prompt", nil)
	require.NoError(t, err)

	prompt, _ := categorizeEvents(waitEvents(t, rec, 2))
	require.NotNil(t, prompt)
	// RunContext SessionID wins over the envelope's.
	require.Equal(t, "session-ctx", prompt.SessionID)
	require.Equal(t, "run-4", prompt.RunID)
	require.Equal(t, "trace-4", prompt.TraceID)
	require.Equal(t, "agent-4", prompt.AgentID)
	// The envelope's NodeID fallback only fills empty fields.
	require.Equal(t, "node-env", prompt.NodeID)
}

// TestEventSinkAdapter_IdentityTriple pins the wiring-point enrichment: a
// triple supplied at composition stamps events the emitters left empty, and
// never overrides a first-class field.
func TestEventSinkAdapter_IdentityTriple(t *testing.T) {
	rec := &recordingTelemetry{}
	sink := NewEventSinkAdapter(rec, IdentityTriple{SessionID: "sess-wired", TaskID: "task-wired", NodeID: "node-wired"})
	instrumented := llm.NewInstrumentedModel(promptStubModel{}, sink, false)

	_, err := instrumented.Generate(context.Background(), "prompt", nil)
	require.NoError(t, err)

	prompt, _ := categorizeEvents(waitEvents(t, rec, 2))
	require.NotNil(t, prompt)
	require.Equal(t, "sess-wired", prompt.SessionID)
	require.Equal(t, "task-wired", prompt.TaskID)
	require.Equal(t, "node-wired", prompt.NodeID)
}

// TestEventSinkAdapter_NilChain documents the degraded-boot contract: a nil
// telemetry chain yields a nil sink, and the instrumented model silently
// drops events instead of panicking.
func TestEventSinkAdapter_NilChain(t *testing.T) {
	require.Nil(t, NewEventSinkAdapter(nil, IdentityTriple{}))
}

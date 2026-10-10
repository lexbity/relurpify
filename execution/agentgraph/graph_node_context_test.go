package agentgraph

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/platform/llm"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// probeModel is a LanguageModel that records the context of every
// invocation so tests can assert what the model layer was handed.
type probeModel struct {
	mu   sync.Mutex
	ctxs []context.Context
}

func (m *probeModel) record(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctxs = append(m.ctxs, ctx)
}

func (m *probeModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	m.record(ctx)
	return &model.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func (m *probeModel) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	m.record(ctx)
	ch := make(chan string, 1)
	ch <- "ok"
	close(ch)
	return ch, nil
}

func (m *probeModel) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	m.record(ctx)
	return &model.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func (m *probeModel) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	m.record(ctx)
	return &model.LLMResponse{Text: "ok", FinishReason: "stop"}, nil
}

func (m *probeModel) invocationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ctxs)
}

// observabilitySink captures the LLM events emitted by an
// InstrumentedModel.
type observabilitySink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *observabilitySink) Emit(_ context.Context, ev any) {
	telemetryEv, ok := ev.(telemetry.Event)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, telemetryEv)
}

func (s *observabilitySink) snapshot() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]telemetry.Event, len(s.events))
	copy(out, s.events)
	return out
}

// llmCallNode invokes the wrapped model while executing, the way a
// paradigm node (e.g. the ReAct think node) drives the model.
type llmCallNode struct {
	id    string
	model llm.LanguageModel
	ctx   context.Context
}

func (n *llmCallNode) ID() string { return n.id }

func (n *llmCallNode) Type() NodeType { return NodeTypeSystem }

func (n *llmCallNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	n.ctx = ctx
	if _, err := n.model.Generate(ctx, "probe prompt", nil); err != nil {
		return nil, err
	}
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

func TestGraph_InjectsEnvelopeAndNodeIDIntoNodeContext(t *testing.T) {
	probe := &probeModel{}
	node := &llmCallNode{id: "probe.node", model: probe}
	done := NewTerminalNode("done")

	graph := NewGraph()
	require.NoError(t, graph.AddNode(node))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.SetStart(node.ID()))
	require.NoError(t, graph.AddEdge(node.ID(), done.ID(), nil, false))

	env := contextdata.NewEnvelope("task-ctx", "session-ctx")
	ctx := telemetry.WithRunContext(context.Background(), telemetry.RunContext{
		SessionID: "session-ctx",
		RunID:     "run-ctx",
		TraceID:   "trace-ctx",
		AgentID:   "agent-ctx",
	})
	_, err := graph.Execute(ctx, env)
	require.NoError(t, err)
	require.Equal(t, 1, probe.invocationCount())

	require.NotNil(t, node.ctx)
	nodeEnv, ok := contextdata.EnvelopeFrom(node.ctx)
	require.True(t, ok, "node context must carry the envelope")
	require.Same(t, env, nodeEnv)
	nodeID, ok := telemetry.NodeIDFromContext(node.ctx)
	require.True(t, ok, "node context must carry the executing node ID")
	require.Equal(t, "probe.node", nodeID)
	rc, ok := telemetry.RunContextFromContext(node.ctx)
	require.True(t, ok, "node context must preserve the turn's run context")
	require.Equal(t, "run-ctx", rc.RunID)
}

// envelopeBackfillSink mirrors the composition root's EventSink adapter
// (Q9): the graph runtime stamps node context; the task envelope's
// node/session/task backfill is composition-owned, so the test wires the
// same behavior locally to prove the whole diagnostic-loop pipeline.
type envelopeBackfillSink struct {
	inner *observabilitySink
}

func (s *envelopeBackfillSink) Emit(ctx context.Context, event any) {
	ev, ok := event.(telemetry.Event)
	if !ok {
		return
	}
	telemetry.StampCorrelation(ctx, &ev)
	if env, ok := contextdata.EnvelopeFrom(ctx); ok {
		if ev.NodeID == "" && env.NodeIDSnapshot() != "" {
			ev.NodeID = env.NodeIDSnapshot()
		}
		if ev.SessionID == "" && env.SessionIDSnapshot() != "" {
			ev.SessionID = env.SessionIDSnapshot()
		}
		if ev.TaskID == "" && env.TaskIDSnapshot() != "" {
			ev.TaskID = env.TaskIDSnapshot()
		}
	}
	s.inner.Emit(ctx, ev)
}

func TestGraph_AttributesLLMEventsToNodeAndTask(t *testing.T) {
	probe := &probeModel{}
	sink := &observabilitySink{}
	instrumented := llm.NewInstrumentedModel(probe, &envelopeBackfillSink{inner: sink}, false)
	node := &llmCallNode{id: "euclo.probe", model: instrumented}
	done := NewTerminalNode("done")

	graph := NewGraph()
	require.NoError(t, graph.AddNode(node))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.SetStart(node.ID()))
	require.NoError(t, graph.AddEdge(node.ID(), done.ID(), nil, false))

	env := contextdata.NewEnvelope("task-42", "session-42")
	ctx := telemetry.WithRunContext(context.Background(), telemetry.RunContext{
		SessionID: "session-42",
		RunID:     "run-42",
		TraceID:   "trace-42",
		AgentID:   "agent-42",
	})
	_, err := graph.Execute(ctx, env)
	require.NoError(t, err)

	events := sink.snapshot()
	var prompt, response *telemetry.Event
	for i := range events {
		switch events[i].Type {
		case telemetry.EventLLMPrompt:
			prompt = &events[i]
		case telemetry.EventLLMResponse:
			response = &events[i]
		}
	}
	require.NotNil(t, prompt, "llm_prompt event must be emitted")
	require.NotNil(t, response, "llm_response event must be emitted")

	// The diagnostic-loop contract: every LLM event carries the same
	// correlation fields as the graph's own node events, so a prompt
	// can be joined to the turn, task, and node that caused it. Correlation
	// lives on the event's first-class fields only — never in Metadata.
	for _, ev := range []*telemetry.Event{prompt, response} {
		require.Equal(t, "task-42", ev.TaskID, "task_id must be attributed to the turn's task")
		require.Equal(t, "euclo.probe", ev.NodeID, "node_id must be attributed to the executing node")
		require.Equal(t, "run-42", ev.RunID, "run_id must match the turn")
		require.Equal(t, "trace-42", ev.TraceID, "trace_id must match the turn")
		require.Equal(t, "session-42", ev.SessionID, "session_id must match the session")
		require.Equal(t, "agent-42", ev.AgentID, "agent_id must match the agent")
		for key := range ev.Metadata {
			switch key {
			case "run_id", "trace_id", "agent_id", "node_id", "session_id", "task_id":
				require.Fail(t, "correlation must not be smuggled into Metadata", "key %q", key)
			}
		}
	}
}

func TestGraph_LLMEventAttributionWithoutTurnContext(t *testing.T) {
	// A graph executed with a bare context (no RunContext) still
	// attributes LLM events to the task and node — correlation fields
	// that are unknown simply stay empty rather than being wrong.
	probe := &probeModel{}
	sink := &observabilitySink{}
	instrumented := llm.NewInstrumentedModel(probe, &envelopeBackfillSink{inner: sink}, false)
	node := &llmCallNode{id: "bare.node", model: instrumented}
	done := NewTerminalNode("done")

	graph := NewGraph()
	require.NoError(t, graph.AddNode(node))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.SetStart(node.ID()))
	require.NoError(t, graph.AddEdge(node.ID(), done.ID(), nil, false))

	env := contextdata.NewEnvelope("task-bare", "session-bare")
	_, err := graph.Execute(context.Background(), env)
	require.NoError(t, err)

	events := sink.snapshot()
	require.NotEmpty(t, events)
	for _, ev := range events {
		if ev.Type == telemetry.EventLLMPrompt || ev.Type == telemetry.EventLLMResponse {
			require.Equal(t, "task-bare", ev.TaskID)
			require.Equal(t, "bare.node", ev.NodeID)
			require.Empty(t, ev.RunID)
			require.Empty(t, ev.TraceID)
		}
	}
}

func TestGraph_NodeContextSurvivesNestedGraphExecution(t *testing.T) {
	// A nested graph (e.g. a thoughtrecipe sub-graph or a ReAct agent
	// graph) executed from a node must stamp its own nodes onto the
	// context, so nested LLM calls attribute to the inner node while
	// keeping the turn's correlation identifiers.
	innerProbe := &probeModel{}
	innerSink := &observabilitySink{}
	innerModel := llm.NewInstrumentedModel(innerProbe, &envelopeBackfillSink{inner: innerSink}, false)
	innerNode := &llmCallNode{id: "inner.think", model: innerModel}
	innerDone := NewTerminalNode("inner.done")
	innerGraph := NewGraph()
	require.NoError(t, innerGraph.AddNode(innerNode))
	require.NoError(t, innerGraph.AddNode(innerDone))
	require.NoError(t, innerGraph.SetStart(innerNode.ID()))
	require.NoError(t, innerGraph.AddEdge(innerNode.ID(), innerDone.ID(), nil, false))

	outerNode := NewSystemNode("outer.spawn", func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
		_, err := innerGraph.Execute(ctx, env)
		return &execution.Result{NodeID: "outer.spawn", Success: true}, err
	})
	outerDone := NewTerminalNode("outer.done")
	outerGraph := NewGraph()
	require.NoError(t, outerGraph.AddNode(outerNode))
	require.NoError(t, outerGraph.AddNode(outerDone))
	require.NoError(t, outerGraph.SetStart(outerNode.ID()))
	require.NoError(t, outerGraph.AddEdge(outerNode.ID(), outerDone.ID(), nil, false))

	env := contextdata.NewEnvelope("task-nested", "session-nested")
	ctx := telemetry.WithRunContext(context.Background(), telemetry.RunContext{
		SessionID: "session-nested",
		RunID:     "run-nested",
		TraceID:   "trace-nested",
		AgentID:   "agent-nested",
	})
	_, err := outerGraph.Execute(ctx, env)
	require.NoError(t, err)

	events := innerSink.snapshot()
	require.NotEmpty(t, events)
	for _, ev := range events {
		if ev.Type == telemetry.EventLLMPrompt || ev.Type == telemetry.EventLLMResponse {
			require.Equal(t, "task-nested", ev.TaskID)
			require.Equal(t, "inner.think", ev.NodeID, "nested LLM events attribute to the inner node")
			require.Equal(t, "run-nested", ev.RunID)
			require.Equal(t, "trace-nested", ev.TraceID)
		}
	}
}

var _ = time.Second // keep time import for eventual-style assertions if extended

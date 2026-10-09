package react

import (
	"context"
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// scriptedReactModel returns canned decisions per think iteration: the first
// len(scripted) ChatWithTools calls consume scripted entries in order, then
// the last one repeats. Generate always completes the task.
type scriptedReactModel struct {
	scripted []string
	calls    int
}

func (m *scriptedReactModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = prompt
	_ = options
	return &model.LLMResponse{
		Text: `{"thought":"converged","action":"complete","complete":true,"summary":"failure analyzed"}`,
	}, nil
}

func (m *scriptedReactModel) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	_ = ctx
	_ = prompt
	_ = options
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *scriptedReactModel) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Generate(ctx, "", nil)
}

func (m *scriptedReactModel) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = messages
	_ = tools
	_ = options
	idx := m.calls
	if idx >= len(m.scripted) {
		idx = len(m.scripted) - 1
	}
	m.calls++
	return &model.LLMResponse{Text: m.scripted[idx]}, nil
}

func (m *scriptedReactModel) ToolRepairStrategy() string  { return "heuristic-only" }
func (m *scriptedReactModel) MaxToolsPerCall() int        { return 0 }
func (m *scriptedReactModel) UsesNativeToolCalling() bool { return true }

// recordingReactTool records every invocation into a shared order log.
type recordingReactTool struct {
	name    string
	fail    bool
	summary string
	order   *[]string
}

func (t *recordingReactTool) Name() string        { return t.name }
func (t *recordingReactTool) Description() string { return t.name }
func (t *recordingReactTool) Category() string    { return "test" }
func (t *recordingReactTool) Parameters() []ports.ToolParameter {
	return []ports.ToolParameter{}
}
func (t *recordingReactTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	_ = ctx
	_ = args
	*t.order = append(*t.order, t.name)
	if t.fail {
		return &ports.ToolResult{
			Success: false,
			Error:   "build failed: assertion boom in TestX",
			Data:    map[string]any{"stdout": "", "stderr": "assertion boom"},
		}, nil
	}
	summary := t.summary
	if summary == "" {
		summary = fmt.Sprintf("%s ok", t.name)
	}
	return &ports.ToolResult{Success: true, Data: map[string]any{"stdout": summary}}, nil
}
func (t *recordingReactTool) IsAvailable(ctx context.Context) bool { _ = ctx; return true }
func (t *recordingReactTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (t *recordingReactTool) Tags() []string { return nil }

type capturingTelemetry struct {
	events []telemetry.Event
}

func (s *capturingTelemetry) Emit(ev telemetry.Event) { s.events = append(s.events, ev) }

func newProbeDeliveryAgent(t *testing.T, order *[]string, sink *capturingTelemetry) (*ReActAgent, *scriptedReactModel) {
	t.Helper()
	ctx := context.Background()
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(ctx, &recordingReactTool{name: "run_suite", fail: true, order: order}); err != nil {
		t.Fatalf("register run_suite: %v", err)
	}
	if err := reg.RegisterLegacyTool(ctx, &recordingReactTool{name: "search_grep", order: order}); err != nil {
		t.Fatalf("register search_grep: %v", err)
	}
	mdl := &scriptedReactModel{
		scripted: []string{
			`{"thought":"run the failing suite","tool":"run_suite","arguments":{},"complete":false}`,
			`{"thought":"investigate the failure","tool":"search_grep","arguments":{"pattern":"boom"},"complete":false}`,
			`{"thought":"nothing else to try","action":"complete","complete":true,"summary":"failure analyzed"}`,
		},
	}
	agent := &ReActAgent{
		Model: mdl,
		Tools: reg,
		Config: &execution.Config{
			Model:             "test-model",
			NativeToolCalling: true,
			Telemetry:         sink,
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Implementation: "react",
				Orchestration: agentspec.AgentOrchestrationConfig{
					Recovery: agentspec.AgentRecoveryPolicy{
						FailureProbeTools: []string{"search_grep"},
					},
				},
			},
		},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("initialize agent: %v", err)
	}
	return agent, mdl
}

func runReactLoop(t *testing.T, agent *ReActAgent, task *execution.Task, env *contextdata.Envelope) {
	t.Helper()
	think := &reactThinkNode{id: "react_think", agent: agent, task: task}
	act := &reactActNode{id: "react_act", agent: agent, task: task}
	observe := &reactObserveNode{id: "react_observe", agent: agent, task: task}
	for i := 0; i < agent.maxIterations+2; i++ {
		if _, err := think.Execute(context.Background(), env); err != nil {
			t.Fatalf("think iteration %d: %v", i, err)
		}
		if _, err := act.Execute(context.Background(), env); err != nil {
			t.Fatalf("act iteration %d: %v", i, err)
		}
		if _, err := observe.Execute(context.Background(), env); err != nil {
			t.Fatalf("observe iteration %d: %v", i, err)
		}
		if done, _ := contextdata.GetTyped[bool](env, "react.done"); done {
			return
		}
	}
	t.Fatal("react loop did not terminate")
}

func TestRecoveryProbeExecutesBeforeLLMCalls(t *testing.T) {
	order := []string{}
	sink := &capturingTelemetry{}
	agent, _ := newProbeDeliveryAgent(t, &order, sink)
	task := &execution.Task{ID: "task-1", Instruction: "analyze why the suite fails"}
	env := contextdata.NewEnvelope("task-1", "session-1")

	runReactLoop(t, agent, task, env)

	// think1 → run_tests (fails); observe1 queues the probe; think2 →
	// search_grep; act2 must execute the queued probe before the LLM's call.
	want := []string{"run_suite", "search_grep", "search_grep"}
	if len(order) != len(want) {
		t.Fatalf("invocation order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("invocation order = %v, want %v", order, want)
		}
	}
	if queued, _ := contextdata.GetTyped[model.ToolCall](env, queuedProbeKey); queued.Name != "" {
		t.Fatalf("probe slot not cleared after consumption: %q", queued.Name)
	}
	probeEvents := 0
	for _, ev := range sink.events {
		if ev.Metadata["security_event"] == "react.recovery_probe_executed" {
			probeEvents++
			if ev.Metadata["probe"] != "search_grep" {
				t.Fatalf("probe event names %v, want search_grep", ev.Metadata["probe"])
			}
		}
	}
	if probeEvents != 1 {
		t.Fatalf("recovery_probe_executed events = %d, want 1", probeEvents)
	}
}

func TestRecoveryProbeThinkNeverSeesQueuedProbe(t *testing.T) {
	// The think node must not read or write the probe slot: the probe queued
	// by observe1 is still pending when think2 runs and must survive it.
	order := []string{}
	agent, _ := newProbeDeliveryAgent(t, &order, &capturingTelemetry{})
	task := &execution.Task{ID: "task-2", Instruction: "analyze why the suite fails"}
	env := contextdata.NewEnvelope("task-2", "session-2")

	think := &reactThinkNode{id: "react_think", agent: agent, task: task}
	act := &reactActNode{id: "react_act", agent: agent, task: task}
	observe := &reactObserveNode{id: "react_observe", agent: agent, task: task}

	// iteration 1: think → run_tests, act executes it (fails), observe queues probe.
	if _, err := think.Execute(context.Background(), env); err != nil {
		t.Fatalf("think1: %v", err)
	}
	if _, err := act.Execute(context.Background(), env); err != nil {
		t.Fatalf("act1: %v", err)
	}
	if _, err := observe.Execute(context.Background(), env); err != nil {
		t.Fatalf("observe1: %v", err)
	}
	queued, ok := contextdata.GetTyped[model.ToolCall](env, queuedProbeKey)
	if !ok || queued.Name != "search_grep" {
		t.Fatalf("observe1 did not queue a probe: %+v ok=%v", queued, ok)
	}
	// think2 must leave the queued probe untouched.
	if _, err := think.Execute(context.Background(), env); err != nil {
		t.Fatalf("think2: %v", err)
	}
	after, _ := contextdata.GetTyped[model.ToolCall](env, queuedProbeKey)
	if after.Name != "search_grep" || len(after.Args) != len(queued.Args) {
		t.Fatalf("think clobbered the queued probe: %+v (was %+v)", after, queued)
	}
}

func TestRecoveryProbeNotScheduledWhenLLMAlreadyQueuedCalls(t *testing.T) {
	order := []string{}
	agent, mdl := newProbeDeliveryAgent(t, &order, &capturingTelemetry{})
	// Script think1 to return a native tool-call list (not a parsed decision)
	// so act finds pending calls and observe must not queue a probe over them.
	mdl.scripted = []string{
		`{"thought":"run tests","tool":"run_suite","arguments":{},"complete":false}`,
	}
	task := &execution.Task{ID: "task-3", Instruction: "analyze why the suite fails"}
	env := contextdata.NewEnvelope("task-3", "session-3")

	think := &reactThinkNode{id: "react_think", agent: agent, task: task}
	act := &reactActNode{id: "react_act", agent: agent, task: task}
	observe := &reactObserveNode{id: "react_observe", agent: agent, task: task}

	if _, err := think.Execute(context.Background(), env); err != nil {
		t.Fatalf("think1: %v", err)
	}
	// Leave a pending call in the slot as if the LLM had queued it natively.
	env.SetWorkingValueWithClass("react.tool_calls", []model.ToolCall{{ID: "pending-1", Name: "search_grep", Args: map[string]any{"pattern": "boom"}}}, contextdata.MemoryClassTask)
	if _, err := act.Execute(context.Background(), env); err != nil {
		t.Fatalf("act1: %v", err)
	}
	if _, err := observe.Execute(context.Background(), env); err != nil {
		t.Fatalf("observe1: %v", err)
	}
	if queued, _ := contextdata.GetTyped[model.ToolCall](env, queuedProbeKey); queued.Name != "" {
		t.Fatalf("probe queued despite pending LLM tool calls: %+v", queued)
	}
}

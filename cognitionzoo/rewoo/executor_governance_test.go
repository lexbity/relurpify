package rewoo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// checkerFunc adapts a function into a permissions.CapabilityChecker.
type checkerFunc func(ctx context.Context, agentID, capability string) error

func (f checkerFunc) CheckCapability(ctx context.Context, agentID, capability string) error {
	return f(ctx, agentID, capability)
}

func allowAllChecker() checkerFunc {
	return checkerFunc(func(ctx context.Context, agentID, capability string) error { return nil })
}

// recordingTool counts invocations so tests can assert which steps ran.
type recordingTool struct {
	name  string
	calls *int
	fail  bool
}

func (t recordingTool) Name() string                      { return t.name }
func (t recordingTool) Description() string               { return t.name }
func (t recordingTool) Category() string                  { return "test" }
func (t recordingTool) Parameters() []ports.ToolParameter { return nil }
func (t recordingTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	_ = ctx
	_ = args
	*t.calls++
	if t.fail {
		return &ports.ToolResult{Success: false, Error: "boom"}, nil
	}
	return &ports.ToolResult{Success: true, Data: map[string]any{"ok": true}}, nil
}
func (t recordingTool) IsAvailable(ctx context.Context) bool { _ = ctx; return true }
func (t recordingTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (t recordingTool) Tags() []string { return nil }

// scriptedReWooModel returns canned Chat responses in order.
type scriptedReWooModel struct {
	responses []string
	chats     int
	failChat  bool
}

func (m *scriptedReWooModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, nil, options)
}

func (m *scriptedReWooModel) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	_ = ctx
	_ = prompt
	_ = options
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *scriptedReWooModel) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	_ = ctx
	_ = messages
	_ = options
	if m.failChat {
		return nil, errors.New("model offline")
	}
	idx := m.chats
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	m.chats++
	return &model.LLMResponse{Text: m.responses[idx]}, nil
}

func (m *scriptedReWooModel) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, messages, options)
}

type capturingReWooTelemetry struct {
	events []telemetry.Event
}

func (s *capturingReWooTelemetry) Emit(ev telemetry.Event) { s.events = append(s.events, ev) }

func (s *capturingReWooTelemetry) count(kind string) int {
	n := 0
	for _, ev := range s.events {
		if string(ev.Type) == kind {
			n++
		}
	}
	return n
}

func newGovernanceRegistry(t *testing.T, calls *int, fail bool) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), recordingTool{name: "rewoo_read", calls: calls}); err != nil {
		t.Fatalf("register rewoo_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(context.Background(), recordingTool{name: "rewoo_fail", calls: calls, fail: fail}); err != nil {
		t.Fatalf("register rewoo_fail: %v", err)
	}
	return reg
}

func TestExecutePlanRefusesNilPermissionChecker(t *testing.T) {
	reg := newGovernanceRegistry(t, new(int), false)
	plan := &RewooPlan{Steps: []RewooStep{{ID: "s1", Tool: "rewoo_read"}}}
	_, err := ExecutePlan(context.Background(), reg, plan, contextdata.NewEnvelope("task", "session"), RewooOptions{})
	if !errors.Is(err, ErrNoPermissionChecker) {
		t.Fatalf("err = %v, want ErrNoPermissionChecker", err)
	}
}

func TestExecutePlanDenyPolicyAbortStopsRemainingSteps(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	denied := checkerFunc(func(ctx context.Context, agentID, capability string) error {
		if capability == "rewoo_fail" {
			return errors.New("not allowed")
		}
		return nil
	})
	plan := &RewooPlan{Steps: []RewooStep{
		{ID: "s1", Tool: "rewoo_read"},
		{ID: "s2", Tool: "rewoo_fail"},
		{ID: "s3", Tool: "rewoo_read", DependsOn: []string{"s2"}},
	}}
	env := contextdata.NewEnvelope("task", "session")
	_, err := ExecutePlan(context.Background(), reg, plan, env, RewooOptions{PermissionChecker: denied})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v, want permission denial", err)
	}
	if calls != 1 {
		t.Fatalf("tool invocations = %d, want 1 (step 3 must never run)", calls)
	}
	// Steps 1 and 2 are recorded under rewoo.step.* (step 2 as denied).
	for _, id := range []string{"s1", "s2"} {
		if raw, ok := contextdata.GetTyped[any](env, fmt.Sprintf("rewoo.step.%s", id)); !ok {
			t.Fatalf("rewoo.step.%s not recorded", id)
		} else if r, ok := raw.(RewooStepResult); ok && id == "s2" && r.Success {
			t.Fatal("denied step recorded as success")
		}
	}
	if _, ok := contextdata.GetTyped[any](env, "rewoo.step.s3"); ok {
		t.Fatal("step 3 was recorded but must never have been attempted")
	}
}

func TestExecutePlanDenyPolicySkipContinues(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	denied := checkerFunc(func(ctx context.Context, agentID, capability string) error {
		if capability == "rewoo_fail" {
			return errors.New("not allowed")
		}
		return nil
	})
	plan := &RewooPlan{Steps: []RewooStep{
		{ID: "s1", Tool: "rewoo_fail"},
		{ID: "s2", Tool: "rewoo_read"},
	}}
	results, err := ExecutePlan(context.Background(), reg, plan, contextdata.NewEnvelope("task", "session"), RewooOptions{
		PermissionChecker:  denied,
		OnPermissionDenied: StepOnFailureSkip,
	})
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}
	if calls != 1 || len(results) != 2 {
		t.Fatalf("calls=%d results=%d, want skip-then-continue", calls, len(results))
	}
	if results[0].Success {
		t.Fatal("denied step must be recorded as failure under skip policy")
	}
}

func newTestRewooAgent(t *testing.T, reg *capability.CapabilityRegistry, mdl model.LanguageModel, tel *capturingReWooTelemetry) *RewooAgent {
	t.Helper()
	agent := &RewooAgent{Model: mdl, Tools: reg}
	if err := agent.Initialize(&execution.Config{Model: "test-model", Telemetry: tel}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return agent
}

const validPlannerJSON = `{"goal":"read the doc","steps":[{"id":"s1","tool":"rewoo_read","params":{"path":"README.md"}}]}`

func TestPlannerLLMPhaseRunsWhenNoContextPlan(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	mdl := &scriptedReWooModel{responses: []string{validPlannerJSON, "Final answer: read the doc."}}
	tel := &capturingReWooTelemetry{}
	agent := newTestRewooAgent(t, reg, mdl, tel)
	agent.Options.PermissionChecker = allowAllChecker()

	task := &execution.Task{ID: "task-1", Instruction: "read the doc"}
	env := contextdata.NewEnvelope("task-1", "session-1")
	if _, err := agent.Execute(context.Background(), task, env); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Exactly one planner call + one synthesizer call; the plan executed.
	if mdl.chats != 2 {
		t.Fatalf("LLM calls = %d, want 2 (plan + synthesize)", mdl.chats)
	}
	if calls != 1 {
		t.Fatalf("tool invocations = %d, want 1", calls)
	}
	source, _ := contextdata.GetTyped[any](env, "rewoo.plan_source")
	if source != planSourceLLM {
		t.Fatalf("rewoo.plan_source = %v, want %q", source, planSourceLLM)
	}
	if out, _ := contextdata.GetTyped[any](env, "rewoo.final_output"); out != "Final answer: read the doc." {
		t.Fatalf("rewoo.final_output = %v", out)
	}
	if got := tel.count("rewoo.llm_phase"); got != 2 {
		t.Fatalf("rewoo.llm_phase events = %d, want 2", got)
	}
}

func TestPlannerInvalidJSONFailsTurnWithZeroToolCalls(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	mdl := &scriptedReWooModel{responses: []string{"this is not json at all"}}
	tel := &capturingReWooTelemetry{}
	agent := newTestRewooAgent(t, reg, mdl, tel)
	agent.Options.PermissionChecker = allowAllChecker()

	env := contextdata.NewEnvelope("task-2", "session-2")
	_, err := agent.Execute(context.Background(), &execution.Task{ID: "task-2", Instruction: "do things"}, env)
	if !errors.Is(err, ErrRewooPlanInvalid) {
		t.Fatalf("err = %v, want ErrRewooPlanInvalid", err)
	}
	if calls != 0 {
		t.Fatalf("tool invocations = %d, want 0", calls)
	}
	if got := tel.count("rewoo_plan_invalid"); got != 1 {
		t.Fatalf("rewoo_plan_invalid events = %d, want 1", got)
	}
}

func TestContextPlanSkipsPlannerLLMCall(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	mdl := &scriptedReWooModel{responses: []string{validPlannerJSON}}
	agent := newTestRewooAgent(t, reg, mdl, &capturingReWooTelemetry{})
	agent.Options.PermissionChecker = allowAllChecker()

	env := contextdata.NewEnvelope("task-3", "session-3")
	task := &execution.Task{
		ID:          "task-3",
		Instruction: "read the doc",
		Context:     map[string]any{"plan": map[string]any{"goal": "read", "steps": []any{map[string]any{"id": "s1", "tool": "rewoo_read"}}}},
	}
	if _, err := agent.Execute(context.Background(), task, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if mdl.chats != 1 {
		t.Fatalf("LLM calls = %d, want 1 (synthesizer only; the planner call must be skipped)", mdl.chats)
	}
	if source, _ := contextdata.GetTyped[any](env, "rewoo.plan_source"); source != planSourceContext {
		t.Fatalf("rewoo.plan_source = %v, want %q", source, planSourceContext)
	}
}

func TestSynthesizeOptOutProducesMechanicalSummary(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	mdl := &scriptedReWooModel{responses: []string{validPlannerJSON}}
	tel := &capturingReWooTelemetry{}
	agent := newTestRewooAgent(t, reg, mdl, tel)
	agent.Options.PermissionChecker = allowAllChecker()
	no := false
	agent.Options.Synthesize = &no

	env := contextdata.NewEnvelope("task-4", "session-4")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task-4", Instruction: "read the doc"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if mdl.chats != 1 {
		t.Fatalf("LLM calls = %d, want 1 (planner only)", mdl.chats)
	}
	out, _ := contextdata.GetTyped[any](env, "rewoo.final_output")
	outStr, _ := out.(string)
	if !strings.Contains(outStr, "Mechanical step summary") || !strings.Contains(outStr, "s1:ok") {
		t.Fatalf("mechanical final_output = %q", outStr)
	}
	modes := 0
	for _, ev := range tel.events {
		if string(ev.Type) == "rewoo.llm_phase" && ev.Metadata["mode"] == "mechanical" {
			modes++
		}
	}
	if modes != 1 {
		t.Fatalf("mode=mechanical events = %d, want 1", modes)
	}
}

func TestSynthesizerFailureFailsTurn(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	mdl := &scriptedReWooModel{responses: []string{validPlannerJSON}, failChat: true}
	agent := newTestRewooAgent(t, reg, mdl, &capturingReWooTelemetry{})
	agent.Options.PermissionChecker = allowAllChecker()
	// failChat suppresses all chats: the planner path is not reached because
	// the context plan is present, so the first Chat is the synthesizer.
	env := contextdata.NewEnvelope("task-5", "session-5")
	task := &execution.Task{
		ID:          "task-5",
		Instruction: "read the doc",
		Context:     map[string]any{"plan": map[string]any{"goal": "read", "steps": []any{map[string]any{"id": "s1", "tool": "rewoo_read"}}}},
	}
	if _, err := agent.Execute(context.Background(), task, env); err == nil {
		t.Fatal("synthesizer model failure must fail the turn")
	}
}

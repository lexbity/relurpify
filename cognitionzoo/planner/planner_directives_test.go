package planner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
)

// plannerScriptedModel returns scripted responses in order and counts the
// library (Generate) and directive (Chat) phases separately. An exhausted
// response list repeats the last response.
type plannerScriptedModel struct {
	mu            sync.Mutex
	responses     []string
	idx           int
	generateCalls int
	chatCalls     int
}

func (m *plannerScriptedModel) respond() *model.LLMResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	text := ""
	if len(m.responses) > 0 {
		idx := m.idx
		if idx >= len(m.responses) {
			idx = len(m.responses) - 1
		}
		text = m.responses[idx]
		m.idx++
	}
	return &model.LLMResponse{Text: text}
}

func (m *plannerScriptedModel) Generate(_ context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.generateCalls++
	m.mu.Unlock()
	return m.respond(), nil
}

func (m *plannerScriptedModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *plannerScriptedModel) Chat(_ context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.chatCalls++
	m.mu.Unlock()
	return m.respond(), nil
}

func (m *plannerScriptedModel) ChatWithTools(ctx context.Context, messages []model.Message, _ []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, messages, options)
}

func (m *plannerScriptedModel) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generateCalls + m.chatCalls
}

// plannerRecordingTool records the order in which plan steps execute.
type plannerRecordingTool struct {
	name  string
	order *[]string
}

func (t plannerRecordingTool) Name() string                      { return t.name }
func (t plannerRecordingTool) Description() string               { return t.name }
func (t plannerRecordingTool) Category() string                  { return "test" }
func (t plannerRecordingTool) Parameters() []ports.ToolParameter { return nil }
func (t plannerRecordingTool) IsAvailable(context.Context) bool  { return true }
func (t plannerRecordingTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (t plannerRecordingTool) Tags() []string { return nil }
func (t plannerRecordingTool) Execute(_ context.Context, _ map[string]any) (*ports.ToolResult, error) {
	*t.order = append(*t.order, t.name)
	return &ports.ToolResult{Success: true, Data: map[string]any{"tool": t.name}}, nil
}

func newPlannerTestRegistry(t *testing.T, order *[]string, names ...string) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewRegistry()
	for _, name := range names {
		if err := reg.RegisterLegacyTool(context.Background(), plannerRecordingTool{name: name, order: order}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	return reg
}

func newPlannerAgent(t *testing.T, mdl model.LanguageModel, reg *capability.CapabilityRegistry, opts ...Option) *PlannerAgent {
	t.Helper()
	deps := &paradigm.Deps{Model: mdl, Registry: reg, Config: &execution.Config{Model: "test-model"}}
	return New(deps, opts...)
}

func executePlanner(t *testing.T, agent *PlannerAgent, env *contextdata.Envelope) *execution.Result {
	t.Helper()
	result, err := agent.Execute(context.Background(), &execution.Task{ID: "task", Instruction: "do the work"}, env)
	if err != nil {
		t.Fatalf("planner execute: %v", err)
	}
	return result
}

// TestAuthoredPlanZeroModelPlanningCalls proves the D1/D3 contract: authored
// steps execute deterministically with zero planning-model calls.
func TestAuthoredPlanZeroModelPlanningCalls(t *testing.T) {
	order := []string{}
	reg := newPlannerTestRegistry(t, &order, "cap_a", "cap_b")
	mdl := &plannerScriptedModel{responses: []string{"unused"}}
	agent := newPlannerAgent(t, mdl, reg, WithAuthoredPlan("objective", []pl.PlanStep{
		{ID: "s1", Description: "first", Tool: "cap_a"},
		{ID: "s2", Description: "second", Tool: "cap_b"},
	}))

	env := contextdata.NewEnvelope("task", "session")
	executePlanner(t, agent, env)

	if got := mdl.calls(); got != 0 {
		t.Fatalf("model calls = %d, want 0 (authored plan; no verify/summarize)", got)
	}
	if len(order) != 2 || order[0] != "cap_a" || order[1] != "cap_b" {
		t.Fatalf("executed steps = %v, want [cap_a cap_b]", order)
	}
	if origin, _ := contextdata.GetTyped[string](env, EnvelopeKeyPlanOrigin); origin != planOriginAuthored {
		t.Fatalf("plan_origin = %q, want authored", origin)
	}
}

// TestAuthoredPlanResumeCompletesRemaining proves completed-step resume: a step
// recorded in plan.completed_steps is not re-executed.
func TestAuthoredPlanResumeCompletesRemaining(t *testing.T) {
	order := []string{}
	reg := newPlannerTestRegistry(t, &order, "cap_a", "cap_b")
	mdl := &plannerScriptedModel{}
	agent := newPlannerAgent(t, mdl, reg, WithAuthoredPlan("objective", []pl.PlanStep{
		{ID: "s1", Description: "first", Tool: "cap_a"},
		{ID: "s2", Description: "second", Tool: "cap_b"},
	}))

	env := contextdata.NewEnvelope("task", "session")
	env.SetWorkingValueWithClass(EnvelopeKeyCompletedSteps, []string{"s1"}, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("planner.step.s1", map[string]any{"tool": "cap_a"}, contextdata.MemoryClassTask)
	executePlanner(t, agent, env)

	if len(order) != 1 || order[0] != "cap_b" {
		t.Fatalf("executed steps = %v, want [cap_b] (s1 resumed)", order)
	}
}

// tallPlanJSON is a 13-step generated plan, one over the default bound.
var tallPlanJSON = `{"goal":"g","steps":[` + strings.Repeat(`{"id":"s","text":"x"},`, 12) + `{"id":"s13","text":"x"}]}`

// TestGeneratedPlanBounded proves a generated plan over the bound is rejected
// as invalid model output, and the node-boundary retry yields exactly two
// model calls.
func TestGeneratedPlanBounded(t *testing.T) {
	reg := newPlannerTestRegistry(t, new([]string), "cap_a")
	mdl := &plannerScriptedModel{responses: []string{tallPlanJSON}}
	agent := newPlannerAgent(t, mdl, reg, WithGeneratedPlan("objective", DefaultGeneratedPlanBound))

	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task"}, contextdata.NewEnvelope("task", "s1")); !errors.Is(err, ErrInvalidModelOutput) {
		t.Fatalf("err = %v, want ErrInvalidModelOutput", err)
	}
	// The node-boundary protocol retries exactly once.
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task"}, contextdata.NewEnvelope("task", "s2")); !errors.Is(err, ErrInvalidModelOutput) {
		t.Fatalf("retry err = %v, want ErrInvalidModelOutput", err)
	}
	if got := mdl.calls(); got != 2 {
		t.Fatalf("model calls = %d, want 2 (initial + one retry)", got)
	}
}

// TestGeneratedPlanValid proves a valid generated plan executes in order with
// plan_origin=generated.
func TestGeneratedPlanValid(t *testing.T) {
	order := []string{}
	reg := newPlannerTestRegistry(t, &order, "cap_a", "cap_b")
	mdl := &plannerScriptedModel{responses: []string{`{"goal":"checks","steps":[{"id":"s1","text":"first","tool":"cap_a"},{"id":"s2","text":"second","tool":"cap_b"}]}`}}
	agent := newPlannerAgent(t, mdl, reg, WithGeneratedPlan("objective", DefaultGeneratedPlanBound))

	env := contextdata.NewEnvelope("task", "session")
	executePlanner(t, agent, env)

	if len(order) != 2 || order[0] != "cap_a" || order[1] != "cap_b" {
		t.Fatalf("executed steps = %v, want [cap_a cap_b]", order)
	}
	if origin, _ := contextdata.GetTyped[string](env, EnvelopeKeyPlanOrigin); origin != planOriginGenerated {
		t.Fatalf("plan_origin = %q, want generated", origin)
	}
	if got := mdl.calls(); got != 1 {
		t.Fatalf("model calls = %d, want 1 (plan only)", got)
	}
}

// TestVerifyPassAndFailFields proves verify writes the verdict fields and a
// fail verdict is data, not an operational failure.
func TestVerifyPassAndFailFields(t *testing.T) {
	order := []string{}
	reg := newPlannerTestRegistry(t, &order, "cap_a")
	mdl := &plannerScriptedModel{responses: []string{`{"verdict":"fail","issues":["missing evidence"]}`}}
	agent := newPlannerAgent(t, mdl, reg,
		WithAuthoredPlan("objective", []pl.PlanStep{{ID: "s1", Description: "first", Tool: "cap_a"}}),
		WithVerify("the criterion"),
	)

	env := contextdata.NewEnvelope("task", "session")
	result := executePlanner(t, agent, env)
	if result == nil || !result.Success {
		t.Fatalf("verify fail must not fail the run: %+v", result)
	}
	fields := execution.ResultFields(result.Data)
	if fields[ResultVerification] != "fail" {
		t.Fatalf("result verification = %v, want fail", fields[ResultVerification])
	}
	if got := strings.Join(toStrings(fields[ResultVerificationIssues]), ","); got != "missing evidence" {
		t.Fatalf("result verification_issues = %v", fields[ResultVerificationIssues])
	}
	if verdict, _ := contextdata.GetTyped[string](env, EnvelopeKeyVerification); verdict != "fail" {
		t.Fatalf("envelope verification = %q, want fail", verdict)
	}
}

// TestVerifyStrictVerdictParse proves a verdict outside pass|fail is invalid
// model output.
func TestVerifyStrictVerdictParse(t *testing.T) {
	order := []string{}
	reg := newPlannerTestRegistry(t, &order, "cap_a")
	mdl := &plannerScriptedModel{responses: []string{`{"verdict":"maybe"}`}}
	agent := newPlannerAgent(t, mdl, reg,
		WithAuthoredPlan("objective", []pl.PlanStep{{ID: "s1", Description: "first", Tool: "cap_a"}}),
		WithVerify("the criterion"),
	)
	_, err := agent.Execute(context.Background(), &execution.Task{ID: "task"}, contextdata.NewEnvelope("task", "session"))
	if !errors.Is(err, ErrInvalidModelOutput) {
		t.Fatalf("err = %v, want ErrInvalidModelOutput", err)
	}
}

// TestSummarizeReplacesResultPreservesRaw proves summarize replaces the result
// and preserves the pre-summary aggregate as result_raw.
func TestSummarizeReplacesResultPreservesRaw(t *testing.T) {
	order := []string{}
	reg := newPlannerTestRegistry(t, &order, "cap_a")
	mdl := &plannerScriptedModel{responses: []string{"final summary"}}
	agent := newPlannerAgent(t, mdl, reg,
		WithAuthoredPlan("objective", []pl.PlanStep{{ID: "s1", Description: "first", Tool: "cap_a"}}),
		WithSummarize("be concise"),
	)

	env := contextdata.NewEnvelope("task", "session")
	result := executePlanner(t, agent, env)
	fields := execution.ResultFields(result.Data)
	if fields[ResultResult] != "final summary" {
		t.Fatalf("result = %v, want the synthesized summary", fields[ResultResult])
	}
	if _, ok := fields[ResultRaw]; !ok {
		t.Fatal("expected result_raw (pre-summary aggregate)")
	}
	if raw, _ := contextdata.GetTyped[any](env, EnvelopeKeyResultRaw); raw == nil {
		t.Fatal("expected planner.result_raw on the envelope")
	}
}

// TestPlannerZeroOptionsUnchanged pins FR-9: with no directive options the
// planner keeps the library generated-from-goal behavior and records no
// directive-mode provenance.
func TestPlannerZeroOptionsUnchanged(t *testing.T) {
	order := []string{}
	reg := newPlannerTestRegistry(t, &order, "cap_a")
	mdl := &plannerScriptedModel{responses: []string{`{"goal":"g","steps":[{"id":"step-1","description":"d","tool":"cap_a","params":{}}],"dependencies":{},"files":[]}`}}
	agent := newPlannerAgent(t, mdl, reg)

	env := contextdata.NewEnvelope("task", "session")
	executePlanner(t, agent, env)

	if mdl.generateCalls == 0 {
		t.Fatal("library mode must use the Generate planning path")
	}
	if _, ok := contextdata.GetTyped[string](env, EnvelopeKeyPlanOrigin); ok {
		t.Fatal("library mode must not record plan_origin")
	}
	if _, ok := contextdata.GetTyped[any](env, EnvelopeKeyCompletedSteps); ok {
		t.Fatal("library mode must not record completed-step resume state")
	}
}

func toStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, item.(string))
		}
		return out
	case nil:
		return nil
	default:
		return nil
	}
}

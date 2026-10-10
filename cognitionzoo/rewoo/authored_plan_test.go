package rewoo

import (
	"context"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
)

// recordingReWooModel records every Chat call's messages and returns scripted
// responses in order. It lets tests distinguish planner from synthesizer calls
// by inspecting the prompt content.
type recordingReWooModel struct {
	mu        sync.Mutex
	responses []string
	calls     [][]model.Message
}

func (m *recordingReWooModel) next() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, nil) // placeholder replaced by record()
	idx := len(m.calls) - 1
	if len(m.responses) == 0 {
		return ""
	}
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	return m.responses[idx]
}

func (m *recordingReWooModel) record(messages []model.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		m.calls = append(m.calls, append([]model.Message(nil), messages...))
		return
	}
	m.calls[len(m.calls)-1] = append([]model.Message(nil), messages...)
}

func (m *recordingReWooModel) messageCalls() [][]model.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]model.Message, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *recordingReWooModel) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, nil, options)
}

func (m *recordingReWooModel) GenerateStream(_ context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *recordingReWooModel) Chat(_ context.Context, messages []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	text := m.next()
	m.record(messages)
	return &model.LLMResponse{Text: text}, nil
}

func (m *recordingReWooModel) ChatWithTools(ctx context.Context, messages []model.Message, _ []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Chat(ctx, messages, options)
}

// orderedTool records the order in which governed tools execute.
type orderedTool struct {
	name  string
	order *[]string
}

func (t orderedTool) Name() string                      { return t.name }
func (t orderedTool) Description() string               { return t.name }
func (t orderedTool) Category() string                  { return "test" }
func (t orderedTool) Parameters() []ports.ToolParameter { return nil }
func (t orderedTool) IsAvailable(context.Context) bool  { return true }
func (t orderedTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (t orderedTool) Tags() []string { return nil }
func (t orderedTool) Execute(_ context.Context, _ map[string]any) (*ports.ToolResult, error) {
	*t.order = append(*t.order, t.name)
	return &ports.ToolResult{Success: true, Data: map[string]any{"tool": t.name}}, nil
}

func newAuthoredTestAgent(t *testing.T, reg *capability.CapabilityRegistry, mdl model.LanguageModel, opts RewooOptions) *RewooAgent {
	t.Helper()
	agent := &RewooAgent{Model: mdl, Tools: reg}
	if err := agent.Initialize(&execution.Config{Model: "test-model"}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	opts.PermissionChecker = allowAllChecker()
	agent.Options = opts
	return agent
}

// TestAuthoredStepsSkipPlannerCall proves the D1/D7 contract: authored steps
// execute deterministically with zero planning-model calls, and provenance
// records plan_origin=authored.
func TestAuthoredStepsSkipPlannerCall(t *testing.T) {
	order := []string{}
	reg := capability.NewRegistry()
	for _, name := range []string{"cap.one", "cap.two"} {
		if err := reg.RegisterLegacyTool(context.Background(), orderedTool{name: name, order: &order}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	mdl := &recordingReWooModel{responses: []string{"synthesized answer"}}
	agent := newAuthoredTestAgent(t, reg, mdl, RewooOptions{
		AuthoredSteps: []RewooStep{
			{ID: "s1", Description: "first", Tool: "cap.one", Params: map[string]any{}},
			{ID: "s2", Description: "second", Tool: "cap.two", Params: map[string]any{}, DependsOn: []string{"s1"}},
		},
		PlanObjective: "do the thing",
	})

	env := contextdata.NewEnvelope("task-authored", "session-authored")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task-authored", Instruction: "ignored"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}

	calls := mdl.messageCalls()
	if len(calls) != 1 {
		t.Fatalf("model calls = %d, want 1 (synthesize only; planner skipped)", len(calls))
	}
	for _, messages := range calls {
		for _, message := range messages {
			if strings.Contains(message.Content, "ReWOO planner") {
				t.Fatalf("planner prompt ran in authored mode: %q", message.Content)
			}
		}
	}
	if len(order) != 2 || order[0] != "cap.one" || order[1] != "cap.two" {
		t.Fatalf("tool execution order = %v, want [cap.one cap.two]", order)
	}
	if origin, _ := contextdata.GetTyped[string](env, "rewoo.plan_origin"); origin != planOriginAuthored {
		t.Fatalf("rewoo.plan_origin = %q, want %q", origin, planOriginAuthored)
	}
	if out, _ := contextdata.GetTyped[any](env, "rewoo.final_output"); out != "synthesized answer" {
		t.Fatalf("rewoo.final_output = %v, want the synthesized answer", out)
	}
}

// TestAuthoredStepsGovernanceInvoked proves every authored step passes through
// the permission checker exactly once, in order.
func TestAuthoredStepsGovernanceInvoked(t *testing.T) {
	order := []string{}
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), orderedTool{name: "cap.one", order: &order}); err != nil {
		t.Fatalf("register: %v", err)
	}
	var checked []string
	checker := checkerFunc(func(_ context.Context, _ string, capabilityID string) error {
		checked = append(checked, capabilityID)
		return nil
	})
	mdl := &recordingReWooModel{responses: []string{"ok"}}
	agent := &RewooAgent{Model: mdl, Tools: reg}
	if err := agent.Initialize(&execution.Config{Model: "test-model"}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	agent.Options = RewooOptions{
		PermissionChecker: checker,
		AuthoredSteps:     []RewooStep{{ID: "s1", Tool: "cap.one", Params: map[string]any{}}},
	}
	env := contextdata.NewEnvelope("task-gov", "session-gov")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task-gov"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(checked) != 1 || checked[0] != "cap.one" {
		t.Fatalf("permission checks = %v, want [cap.one]", checked)
	}
}

// TestSynthesizeGuidanceReachesPrompt proves the authored `synthesize`
// guidance reaches the synthesizer prompt as an authoritative system message.
func TestSynthesizeGuidanceReachesPrompt(t *testing.T) {
	order := []string{}
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), orderedTool{name: "cap.one", order: &order}); err != nil {
		t.Fatalf("register: %v", err)
	}
	mdl := &recordingReWooModel{responses: []string{"answer"}}
	agent := newAuthoredTestAgent(t, reg, mdl, RewooOptions{
		AuthoredSteps:      []RewooStep{{ID: "s1", Tool: "cap.one", Params: map[string]any{}}},
		SynthesizeGuidance: "Prefer a terse bullet list.",
	})
	env := contextdata.NewEnvelope("task-guidance", "session-guidance")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task-guidance"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	found := false
	for _, messages := range mdl.messageCalls() {
		for _, message := range messages {
			if strings.Contains(message.Content, "Prefer a terse bullet list.") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("synthesize guidance did not reach the synthesizer prompt")
	}
}

// TestPlanObjectiveRefinesGeneratedPrompt proves the authored `plan` text
// refines the generated planner objective even when no steps are authored.
func TestPlanObjectiveRefinesGeneratedPrompt(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	mdl := &recordingReWooModel{responses: []string{validPlannerJSON, "final"}}
	agent := newAuthoredTestAgent(t, reg, mdl, RewooOptions{PlanObjective: "audit the layering"})
	env := contextdata.NewEnvelope("task-objective", "session-objective")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task-objective", Instruction: "generic"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	plannerUser := ""
	for _, messages := range mdl.messageCalls() {
		for _, message := range messages {
			if message.Role == "user" && strings.Contains(message.Content, "Produce a plan") {
				plannerUser = message.Content
			}
		}
	}
	if !strings.Contains(plannerUser, "audit the layering") {
		t.Fatalf("planner prompt = %q, want the authored objective", plannerUser)
	}
}

// TestReWOOZeroOptionsUnchanged pins FR-9: an agent constructed with no
// directive options keeps the generated planner behavior (two model calls,
// plan_origin=generated).
func TestReWOOZeroOptionsUnchanged(t *testing.T) {
	calls := 0
	reg := newGovernanceRegistry(t, &calls, false)
	mdl := &scriptedReWooModel{responses: []string{validPlannerJSON, "final"}}
	agent := newAuthoredTestAgent(t, reg, mdl, RewooOptions{})
	env := contextdata.NewEnvelope("task-zero", "session-zero")
	if _, err := agent.Execute(context.Background(), &execution.Task{ID: "task-zero", Instruction: "read the doc"}, env); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if mdl.chats != 2 {
		t.Fatalf("LLM calls = %d, want 2 (generated plan + synthesize)", mdl.chats)
	}
	if origin, _ := contextdata.GetTyped[string](env, "rewoo.plan_origin"); origin != planOriginGenerated {
		t.Fatalf("rewoo.plan_origin = %q, want %q", origin, planOriginGenerated)
	}
}

package react

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type scopeAwareReactModel struct {
	nativeToolCalling bool
	generatePrompts   []string
	chatMessages      [][]contracts.Message
	chatToolSpecs     [][]contracts.LLMToolSpec
}

func (m *scopeAwareReactModel) Generate(ctx context.Context, prompt string, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	_ = ctx
	_ = options
	m.generatePrompts = append(m.generatePrompts, prompt)
	return &contracts.LLMResponse{
		Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`,
	}, nil
}

func (m *scopeAwareReactModel) GenerateStream(ctx context.Context, prompt string, options *contracts.LLMOptions) (<-chan string, error) {
	_ = ctx
	_ = prompt
	_ = options
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *scopeAwareReactModel) Chat(ctx context.Context, messages []contracts.Message, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	_ = ctx
	_ = options
	return &contracts.LLMResponse{
		Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`,
	}, nil
}

func (m *scopeAwareReactModel) ChatWithTools(ctx context.Context, messages []contracts.Message, tools []contracts.LLMToolSpec, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	_ = ctx
	_ = options
	m.chatMessages = append(m.chatMessages, append([]contracts.Message(nil), messages...))
	m.chatToolSpecs = append(m.chatToolSpecs, append([]contracts.LLMToolSpec(nil), tools...))
	return &contracts.LLMResponse{
		Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`,
	}, nil
}

func (m *scopeAwareReactModel) ToolRepairStrategy() string {
	return "heuristic-only"
}

func (m *scopeAwareReactModel) MaxToolsPerCall() int {
	return 0
}

func (m *scopeAwareReactModel) UsesNativeToolCalling() bool {
	return m.nativeToolCalling
}

type scopeAwareReactTool struct {
	name string
}

func (t scopeAwareReactTool) Name() string                          { return t.name }
func (t scopeAwareReactTool) Description() string                   { return t.name }
func (t scopeAwareReactTool) Category() string                      { return "test" }
func (t scopeAwareReactTool) Parameters() []contracts.ToolParameter { return nil }
func (t scopeAwareReactTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	_ = ctx
	_ = args
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"name": t.name}}, nil
}
func (t scopeAwareReactTool) IsAvailable(ctx context.Context) bool {
	_ = ctx
	return true
}
func (t scopeAwareReactTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{}
}
func (t scopeAwareReactTool) Tags() []string { return nil }

func toolCapabilityID(name string) string {
	return "tool:" + name
}

func TestReActUsesScopedRegistryForPromptAndNativeToolCalling(t *testing.T) {
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(scopeAwareReactTool{name: "scope_read"}); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(scopeAwareReactTool{name: "scope_write"}); err != nil {
		t.Fatalf("register scope_write: %v", err)
	}
	scoped := reg.WithAllowlist([]string{toolCapabilityID("scope_read")})
	if scoped == nil {
		t.Fatal("expected scoped registry")
	}

	task := &core.Task{ID: "task-1", Instruction: "inspect the workspace"}
	env := contextdata.NewEnvelope("task-1", "session-1")
	step := &reactThinkNode{
		id: "react_think",
		agent: &ReActAgent{
			Model:  &scopeAwareReactModel{nativeToolCalling: true},
			Tools:  scoped,
			Config: &core.Config{Model: "test-model", NativeToolCalling: true},
		},
		task: task,
	}

	runtimeCtx := step.buildRuntimeContext(env, scoped.ModelCallableTools())
	if got, want := toolNames(runtimeCtx.Tools), []string{"scope_read"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime tools = %#v, want %#v", got, want)
	}

	_, err := step.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("native execute failed: %v", err)
	}

	model := step.agent.Model.(*scopeAwareReactModel)
	if got, want := len(model.chatToolSpecs), 1; got != want {
		t.Fatalf("native tool call count = %d, want %d", got, want)
	}
	if got, want := len(model.chatToolSpecs[0]), 1; got != want {
		t.Fatalf("native tool spec count = %d, want %d", got, want)
	}
	if got := model.chatToolSpecs[0][0].Name; got != "scope_read" {
		t.Fatalf("native tool spec name = %q, want scope_read", got)
	}
	systemPrompt := model.chatMessages[0][0].Content
	if !strings.Contains(systemPrompt, "scope_read") {
		t.Fatalf("native system prompt missing scoped tool: %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "scope_write") {
		t.Fatalf("native system prompt leaked hidden tool: %q", systemPrompt)
	}

	fallbackModel := &scopeAwareReactModel{}
	fallbackStep := &reactThinkNode{
		id: "react_think_fallback",
		agent: &ReActAgent{
			Model:  fallbackModel,
			Tools:  scoped,
			Config: &core.Config{Model: "test-model", NativeToolCalling: false},
		},
		task: task,
	}
	if _, err := fallbackStep.Execute(context.Background(), contextdata.NewEnvelope("task-2", "session-2")); err != nil {
		t.Fatalf("fallback execute failed: %v", err)
	}
	if got, want := len(fallbackModel.generatePrompts), 1; got != want {
		t.Fatalf("fallback prompt count = %d, want %d", got, want)
	}
	if prompt := fallbackModel.generatePrompts[0]; !strings.Contains(prompt, "scope_read") || strings.Contains(prompt, "scope_write") {
		t.Fatalf("fallback prompt does not match scoped tool surface: %q", prompt)
	}
}

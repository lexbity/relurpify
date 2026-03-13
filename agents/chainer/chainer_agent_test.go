package chainer_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/core"
)

type captureModel struct {
	responses []string
	calls     int
	messages  [][]core.Message
}

func (m *captureModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}
func (m *captureModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}
func (m *captureModel) Chat(_ context.Context, messages []core.Message, _ *core.LLMOptions) (*core.LLMResponse, error) {
	copied := make([]core.Message, len(messages))
	copy(copied, messages)
	m.messages = append(m.messages, copied)
	text := ""
	if m.calls < len(m.responses) {
		text = m.responses[m.calls]
	}
	m.calls++
	return &core.LLMResponse{Text: text}, nil
}
func (m *captureModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func TestChain_Validate_EmptyName(t *testing.T) {
	chain := &chainer.Chain{Links: []chainer.Link{{OutputKey: "x"}}}
	if err := chain.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestChain_Validate_EmptyOutputKey(t *testing.T) {
	chain := &chainer.Chain{Links: []chainer.Link{{Name: "x"}}}
	if err := chain.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterState_OnlyDeclaredKeys(t *testing.T) {
	state := core.NewContext()
	state.Set("a", 1)
	state.Set("b", 2)
	state.Set("c", 3)
	filtered := chainer.FilterState(state, []string{"a", "b"})
	if len(filtered) != 2 || filtered["a"] != 1 || filtered["b"] != 2 {
		t.Fatalf("unexpected filtered state: %+v", filtered)
	}
	if _, ok := filtered["c"]; ok {
		t.Fatal("unexpected key c")
	}
}

func TestChainRunner_WritesOutputKey(t *testing.T) {
	model := &captureModel{responses: []string{"hello"}}
	state := core.NewContext()
	chain := &chainer.Chain{Links: []chainer.Link{
		chainer.NewLink("one", "Prompt", nil, "out", nil),
	}}
	if err := chainer.RunChain(context.Background(), model, &core.Task{Instruction: "go"}, chain, state); err != nil {
		t.Fatalf("RunChain: %v", err)
	}
	if got := state.GetString("out"); got != "hello" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestChainRunner_RetryOnParseFailure(t *testing.T) {
	model := &captureModel{responses: []string{"bad", "2"}}
	state := core.NewContext()
	parseCalls := 0
	chain := &chainer.Chain{Links: []chainer.Link{{
		Name:         "one",
		SystemPrompt: "Prompt",
		OutputKey:    "out",
		OnFailure:    chainer.FailurePolicyRetry,
		MaxRetries:   1,
		Parse: func(text string) (any, error) {
			parseCalls++
			if parseCalls == 1 {
				return nil, errors.New("bad parse")
			}
			return strconv.Atoi(text)
		},
	}}}
	if err := chainer.RunChain(context.Background(), model, &core.Task{Instruction: "go"}, chain, state); err != nil {
		t.Fatalf("RunChain: %v", err)
	}
	if model.calls != 2 {
		t.Fatalf("expected 2 model calls, got %d", model.calls)
	}
	if value, _ := state.Get("out"); value != 2 {
		t.Fatalf("unexpected parsed output: %+v", value)
	}
}

func TestChainRunner_FailFastOnParseFailure(t *testing.T) {
	model := &captureModel{responses: []string{"bad"}}
	chain := &chainer.Chain{Links: []chainer.Link{{
		Name:         "one",
		SystemPrompt: "Prompt",
		OutputKey:    "out",
		OnFailure:    chainer.FailurePolicyFailFast,
		Parse: func(text string) (any, error) {
			return nil, errors.New("bad parse")
		},
	}}}
	err := chainer.RunChain(context.Background(), model, &core.Task{Instruction: "go"}, chain, core.NewContext())
	if !errors.Is(err, chainer.ErrLinkParseFailure) {
		t.Fatalf("expected ErrLinkParseFailure, got %v", err)
	}
	if model.calls != 1 {
		t.Fatalf("expected 1 model call, got %d", model.calls)
	}
}

func TestChainerAgent_ImplementsGraphAgent(t *testing.T) {
	agent := &chainer.ChainerAgent{
		Chain: &chainer.Chain{Links: []chainer.Link{
			chainer.NewLink("one", "Prompt", nil, "out", nil),
		}},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(agent.Capabilities()) == 0 {
		t.Fatal("expected capabilities")
	}
	g, err := agent.BuildGraph(&core.Task{Instruction: "go"})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if g == nil {
		t.Fatal("expected graph")
	}
}

func TestChainerAgent_SequentialLinks(t *testing.T) {
	model := &captureModel{responses: []string{"first", "second"}}
	agent := &chainer.ChainerAgent{
		Model: model,
		Chain: &chainer.Chain{Links: []chainer.Link{
			chainer.NewLink("one", "first", nil, "out.one", nil),
			chainer.NewLink("two", "second {{.Input.out.one}}", []string{"out.one"}, "out.two", nil),
		}},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{Instruction: "go"}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if state.GetString("out.one") != "first" || state.GetString("out.two") != "second" {
		t.Fatalf("missing outputs: %+v", state.StateSnapshot())
	}
}

func TestChainerAgent_InputKeyIsolation(t *testing.T) {
	model := &captureModel{responses: []string{"first", "second"}}
	agent := &chainer.ChainerAgent{
		Model: model,
		Chain: &chainer.Chain{Links: []chainer.Link{
			chainer.NewLink("one", "visible {{.Instruction}}", nil, "out.one", nil),
			chainer.NewLink("two", "only instruction {{.Instruction}}", nil, "out.two", nil),
		}},
	}
	if err := agent.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := agent.Execute(context.Background(), &core.Task{Instruction: "go"}, core.NewContext()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(model.messages) < 2 {
		t.Fatalf("expected captured messages")
	}
	if strings.Contains(model.messages[1][0].Content, "first") {
		t.Fatalf("second link prompt leaked prior output: %q", model.messages[1][0].Content)
	}
}

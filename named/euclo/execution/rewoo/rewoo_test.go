package rewoo

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

type wrapperStubModel struct {
	responses []string
	calls     int
}

func (m *wrapperStubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *wrapperStubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (m *wrapperStubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	text := ""
	if m.calls < len(m.responses) {
		text = m.responses[m.calls]
	}
	m.calls++
	return &core.LLMResponse{Text: text}, nil
}

func (m *wrapperStubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func TestNewReturnsConfiguredRunner(t *testing.T) {
	env := testutil.Env(t)

	runner := New(env)
	if runner == nil {
		t.Fatal("expected runner")
	}
	if runner.Model != env.Model {
		t.Fatal("expected model to be wired from environment")
	}
	if runner.Tools != env.Registry {
		t.Fatal("expected registry to be wired from environment")
	}
}

func TestExecuteRunsWithStubModel(t *testing.T) {
	env := testutil.Env(t)
	if err := env.Registry.Register(testutil.EchoTool{}); err != nil {
		t.Fatalf("register echo: %v", err)
	}
	env.Model = &wrapperStubModel{responses: []string{
		`{"goal":"g","steps":[{"id":"a","description":"d","tool":"echo","params":{"value":"ok"},"depends_on":[],"on_failure":"skip"}]}`,
		"final answer",
	}}

	result, err := Execute(context.Background(), env, &core.Task{Instruction: "do it"}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
}

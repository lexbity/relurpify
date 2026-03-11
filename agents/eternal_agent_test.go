package agents

import (
	"context"
	"github.com/lexcodex/relurpify/framework/core"
	"testing"
	"time"
)

type fixedStreamModel struct {
	calls int
	text  string
}

func (m *fixedStreamModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "unused"}, nil
}

func (m *fixedStreamModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	m.calls++
	ch := make(chan string, 1)
	ch <- m.text
	close(ch)
	return ch, nil
}

func (m *fixedStreamModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "unused"}, nil
}

func (m *fixedStreamModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "unused"}, nil
}

func TestEternalAgentFiniteCycles(t *testing.T) {
	t.Helper()
	model := &fixedStreamModel{text: "meow"}
	agent := &EternalAgent{Model: model}
	if err := agent.Initialize(&core.Config{Model: "test"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	task := &core.Task{
		ID:          "eternal-test",
		Instruction: "initiate sequence",
		Type:        core.TaskTypeCodeGeneration,
		Context: map[string]any{
			"eternal.infinite":   false,
			"eternal.max_cycles": 2,
			"eternal.sleep":      "0s",
		},
	}
	state := core.NewContext()
	res, err := agent.Execute(ctx, task, state)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("unexpected result: %+v", res)
	}
	if model.calls != 2 {
		t.Fatalf("expected 2 cycles, got %d", model.calls)
	}
	h := state.History()
	if len(h) < 2 {
		t.Fatalf("expected history entries, got %d", len(h))
	}
}

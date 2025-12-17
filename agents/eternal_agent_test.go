package agents

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework"
)

type fixedStreamModel struct {
	calls int
	text  string
}

func (m *fixedStreamModel) Generate(context.Context, string, *framework.LLMOptions) (*framework.LLMResponse, error) {
	return &framework.LLMResponse{Text: "unused"}, nil
}

func (m *fixedStreamModel) GenerateStream(context.Context, string, *framework.LLMOptions) (<-chan string, error) {
	m.calls++
	ch := make(chan string, 1)
	ch <- m.text
	close(ch)
	return ch, nil
}

func (m *fixedStreamModel) Chat(context.Context, []framework.Message, *framework.LLMOptions) (*framework.LLMResponse, error) {
	return &framework.LLMResponse{Text: "unused"}, nil
}

func (m *fixedStreamModel) ChatWithTools(context.Context, []framework.Message, []framework.Tool, *framework.LLMOptions) (*framework.LLMResponse, error) {
	return &framework.LLMResponse{Text: "unused"}, nil
}

func TestEternalAgentFiniteCycles(t *testing.T) {
	t.Helper()
	model := &fixedStreamModel{text: "meow"}
	agent := &EternalAgent{Model: model}
	if err := agent.Initialize(&framework.Config{Model: "test"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	task := &framework.Task{
		ID:          "eternal-test",
		Instruction: "initiate sequence",
		Type:        framework.TaskTypeCodeGeneration,
		Context: map[string]any{
			"eternal.infinite":   false,
			"eternal.max_cycles": 2,
			"eternal.sleep":      "0s",
		},
	}
	state := framework.NewContext()
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

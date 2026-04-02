package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestErrorModelReturnsConfiguredError(t *testing.T) {
	want := errors.New("boom")
	model := ErrorModel{Err: want}

	if _, err := model.Generate(context.Background(), "prompt", nil); !errors.Is(err, want) {
		t.Fatalf("Generate error = %v, want %v", err, want)
	}
	if _, err := model.GenerateStream(context.Background(), "prompt", nil); !errors.Is(err, want) {
		t.Fatalf("GenerateStream error = %v, want %v", err, want)
	}
	if _, err := model.Chat(context.Background(), nil, nil); !errors.Is(err, want) {
		t.Fatalf("Chat error = %v, want %v", err, want)
	}
	if _, err := model.ChatWithTools(context.Background(), nil, nil, nil); !errors.Is(err, want) {
		t.Fatalf("ChatWithTools error = %v, want %v", err, want)
	}
}

func TestEchoToolReturnsFirstArgument(t *testing.T) {
	tool := EchoTool{}
	result, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"value": "hello",
		"extra": "ignored",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if got := result.Data["echo"]; got != "hello" {
		t.Fatalf("echo = %#v, want hello", got)
	}
}

func TestNoopExecutorRecordsCalls(t *testing.T) {
	exec := &NoopExecutor{}
	task := &core.Task{ID: "task-1"}
	result, err := exec.Execute(context.Background(), task, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if exec.Calls != 1 {
		t.Fatalf("Calls = %d, want 1", exec.Calls)
	}
	if len(exec.Tasks) != 1 || exec.Tasks[0] != task {
		t.Fatalf("Tasks = %#v, want recorded task", exec.Tasks)
	}
}

func TestRegistryWithRegistersTools(t *testing.T) {
	registry := RegistryWith(EchoTool{ToolName: "echo-test"})
	if _, ok := registry.Get("echo-test"); !ok {
		t.Fatal("expected tool to be registered")
	}
}

func TestEnvWithScenarioModelWiresScenarioModel(t *testing.T) {
	env, model := EnvWithScenarioModel(t, Turn("chat").Responding("ok").Build())
	if env.Model != model {
		t.Fatal("expected environment model to match returned model")
	}
	if env.Memory == nil {
		t.Fatal("expected environment memory")
	}
	if len(model.Turns()) != 1 {
		t.Fatalf("Turns len = %d, want 1", len(model.Turns()))
	}
}

func TestTurnBuilderBuildsScenarioTurn(t *testing.T) {
	turn := Turn("chat_with_tools").
		ExpectingPromptFragment("implement").
		WithToolCall("echo", map[string]interface{}{"value": "hi"}).
		Responding(`{"ok":true}`).
		Build()

	if turn.Method != "chat_with_tools" {
		t.Fatalf("Method = %q", turn.Method)
	}
	if len(turn.PromptContains) != 1 || turn.PromptContains[0] != "implement" {
		t.Fatalf("PromptContains = %#v", turn.PromptContains)
	}
	if turn.Response == nil || turn.Response.Text != `{"ok":true}` {
		t.Fatalf("Response = %#v", turn.Response)
	}
	if len(turn.Response.ToolCalls) != 1 || turn.Response.ToolCalls[0].Name != "echo" {
		t.Fatalf("ToolCalls = %#v", turn.Response.ToolCalls)
	}
}

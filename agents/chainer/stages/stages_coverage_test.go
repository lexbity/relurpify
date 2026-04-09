package stages_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/agents/chainer/stages"
	"github.com/lexcodex/relurpify/framework/core"
)

type coverageModel struct{}

func (m *coverageModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *coverageModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (m *coverageModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *coverageModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func TestStageHelpers(t *testing.T) {
	ctx := core.NewContext()
	ctx.Set("visible", "value")
	ctx.Set("hidden", "secret")

	filtered := stages.FilterState(ctx, []string{"visible", "missing"})
	if len(filtered) != 1 || filtered["visible"] != "value" {
		t.Fatalf("unexpected filtered state: %+v", filtered)
	}
	if got := stages.FilterState(nil, []string{"visible"}); len(got) != 0 {
		t.Fatalf("expected empty filtered state, got %+v", got)
	}

	prompt, err := stages.RenderPrompt("{{.Instruction}}/{{.Input.visible}}", "task", filtered)
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	if prompt != "task/value" {
		t.Fatalf("unexpected prompt %q", prompt)
	}
	if _, err := stages.RenderPrompt("", "task", filtered); err == nil {
		t.Fatal("expected empty template error")
	}
	if _, err := stages.RenderPrompt("{{.Instruction", "task", filtered); err == nil {
		t.Fatal("expected parse error")
	}

	stages.RecordInteraction(nil, "assistant", "ignored", nil)
	stages.RecordInteraction(ctx, "assistant", "hello", map[string]any{"link": "demo"})
	if history := ctx.History(); len(history) != 1 || history[0].Content != "hello" {
		t.Fatalf("unexpected interaction history: %+v", history)
	}

	if got := stages.TaskInstruction(nil); got != "" {
		t.Fatalf("expected empty instruction, got %q", got)
	}
	if got := stages.TaskInstruction(&core.Task{Instruction: "go"}); got != "go" {
		t.Fatalf("unexpected instruction %q", got)
	}
}

func TestStageWrappers(t *testing.T) {
	model := &coverageModel{}
	link := chainer.NewLink("demo", "Inspect {{.Instruction}}", []string{"input"}, "output", nil)

	base := stages.NewLinkStage(&link, model)
	if base == nil {
		t.Fatal("expected link stage")
	}
	if name := base.Name(); name != "demo" {
		t.Fatalf("unexpected base stage name %q", name)
	}

	withOpts := stages.NewLinkStageWithOptions(&link, model, &core.LLMOptions{Model: "test-model"})
	if withOpts == nil || withOpts.LLMOptions == nil || withOpts.LLMOptions.Model != "test-model" {
		t.Fatalf("unexpected stage options: %+v", withOpts)
	}

	summarize := stages.SummarizeStage("summary", []string{"input"}, "summary", model)
	if summarize == nil || summarize.Name() != "summary" {
		t.Fatalf("unexpected summarize stage: %+v", summarize)
	}
	if !strings.Contains(summarize.Contract().Name, "chainer.summary") {
		t.Fatalf("unexpected summarize contract: %+v", summarize.Contract())
	}

	transform := stages.TransformStage("transform", []string{"input"}, "result", func(text string) (any, error) {
		return strings.ToUpper(text), nil
	}, model)
	if transform == nil || transform.Name() != "transform" {
		t.Fatalf("unexpected transform stage: %+v", transform)
	}
	if !strings.Contains(transform.Contract().Name, "chainer.transform") {
		t.Fatalf("unexpected transform contract: %+v", transform.Contract())
	}
}

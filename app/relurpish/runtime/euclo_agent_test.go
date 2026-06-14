package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo"
)

type slice1Model struct{}

func (slice1Model) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func (slice1Model) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- "ok"
	close(ch)
	return ch, nil
}

func (slice1Model) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func (slice1Model) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func TestInstantiateAgentReturnsEuclo(t *testing.T) {
	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
		Model:    slice1Model{},
	}

	agent, err := instantiateAgent(deps)
	if err != nil {
		t.Fatalf("instantiateAgent returned error: %v", err)
	}
	if agent == nil {
		t.Fatal("instantiateAgent returned nil agent")
	}
	if _, ok := agent.(*euclo.Agent); !ok {
		t.Fatalf("instantiateAgent returned %T, want *euclo.Agent", agent)
	}
}

func TestInstantiateAgentNilRegistry(t *testing.T) {
	deps := &paradigm.Deps{
		Model: slice1Model{},
	}

	agent, err := instantiateAgent(deps)
	if err == nil {
		t.Fatal("instantiateAgent returned nil error")
	}
	if agent != nil {
		t.Fatalf("instantiateAgent returned %T, want nil", agent)
	}
	if !strings.Contains(err.Error(), "registry") {
		t.Fatalf("error %q does not mention registry", err)
	}
}

func TestEucloExecutesWithEmptyRegistry(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
		Model:    slice1Model{},
	}
	agent, err := instantiateAgent(deps)
	if err != nil {
		t.Fatalf("instantiateAgent returned error: %v", err)
	}

	task := &execution.Task{
		ID:          "slice-1-task",
		Type:        "analysis",
		Instruction: "hello",
	}
	env := contextdata.NewEnvelope(task.ID, "slice-1-session")

	res, err := agent.Execute(context.Background(), task, env)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("Execute returned nil result")
	}
}

func TestEucloMalformedRecipeErrors(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	sourceRoot := filepath.Join(tmp, "relurpify_cfg", "euclo")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "bad.euclo"), []byte("thoughtrecipe \x00\x01 garbage\n"), 0o600); err != nil {
		t.Fatalf("write malformed recipe: %v", err)
	}

	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
		Model:    slice1Model{},
	}
	agent, err := instantiateAgent(deps)
	if err != nil {
		t.Fatalf("instantiateAgent returned error: %v", err)
	}

	task := &execution.Task{
		ID:          "slice-1-malformed",
		Type:        "analysis",
		Instruction: "hello",
	}
	env := contextdata.NewEnvelope(task.ID, "slice-1-session")

	res, err := agent.Execute(context.Background(), task, env)
	if err == nil {
		t.Fatal("Execute unexpectedly succeeded with malformed recipe")
	}
	if res != nil {
		t.Fatalf("Execute returned result %#v, want nil", res)
	}
}

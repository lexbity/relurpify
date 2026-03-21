package delegates

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	rexroute "github.com/lexcodex/relurpify/named/rex/route"
)

type stubModel struct{}

func (stubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
func (stubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (stubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}
func (stubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func testEnv(t *testing.T) agentenv.AgentEnvironment {
	t.Helper()
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	return agentenv.AgentEnvironment{
		Model:    stubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config: &core.Config{
			Name:          "rex-test",
			Model:         "stub",
			MaxIterations: 1,
		},
	}
}

func TestResolveReturnsPrimaryDelegate(t *testing.T) {
	registry := NewRegistry(testEnv(t), t.TempDir())
	delegate, err := registry.Resolve(rexroute.ExecutionPlan{PrimaryFamily: rexroute.FamilyReAct})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if delegate.Family() != rexroute.FamilyReAct {
		t.Fatalf("family = %q", delegate.Family())
	}
}

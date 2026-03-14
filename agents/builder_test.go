package agents

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	namedfactory "github.com/lexcodex/relurpify/named/factory"
	"github.com/stretchr/testify/require"
)

type builderStubModel struct{}

func (builderStubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"goal":"test","steps":[],"files":[],"dependencies":{}}`}, nil
}

func (builderStubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (builderStubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}

func (builderStubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}

type builderTestAgent struct{}

func (builderTestAgent) Initialize(*core.Config) error { return nil }
func (builderTestAgent) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	return &core.Result{Success: true}, nil
}
func (builderTestAgent) Capabilities() []core.Capability { return nil }
func (builderTestAgent) BuildGraph(*core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart(done.ID()); err != nil {
		return nil, err
	}
	return g, nil
}

func TestAgentBuilderBuildsAllSupportedAgentTypes(t *testing.T) {
	namedfactory.RegisterNamedAgent("testfu", func(string, AgentEnvironment) graph.WorkflowExecutor {
		return builderTestAgent{}
	})

	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	env := AgentEnvironment{
		Model:    builderStubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config: &core.Config{
			Name:              "builder-test",
			Model:             "stub",
			MaxIterations:     2,
			OllamaToolCalling: true,
		},
	}

	for _, agentType := range []string{
		"react", "architect", "pipeline", "planner", "reflection",
		"chainer", "htn", "blackboard", "rewoo", "goalcon", "testfu", "eternal",
	} {
		t.Run(agentType, func(t *testing.T) {
			agent, err := NewAgentBuilder().WithEnvironment(&env).Build(agentType)
			require.NoError(t, err)
			_, err = agent.BuildGraph(&core.Task{ID: "task-1", Instruction: "test"})
			if agentType == "pipeline" {
				require.ErrorContains(t, err, "pipeline stages not configured")
				return
			}
			require.NoError(t, err)
		})
	}
}

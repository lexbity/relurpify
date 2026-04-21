package factory

import (
	"context"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/rex"
	"github.com/stretchr/testify/require"
)

type factoryStubModel struct{}

func (factoryStubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (factoryStubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (factoryStubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}

func (factoryStubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func testEnv(t *testing.T) agentenv.AgentEnvironment {
	t.Helper()
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	return agentenv.AgentEnvironment{
		Model:    factoryStubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config: &core.Config{
			Name:          "factory-test",
			Model:         "stub",
			MaxIterations: 1,
		},
	}
}

func TestBuildFromSpecRoutesCodingToEuclo(t *testing.T) {
	agent, err := BuildFromSpec(testEnv(t), core.AgentRuntimeSpec{Implementation: "coding"})
	require.NoError(t, err)
	require.IsType(t, &euclo.Agent{}, agent)
}

func TestBuildFromSpecKeepsReactGeneric(t *testing.T) {
	agent, err := BuildFromSpec(testEnv(t), core.AgentRuntimeSpec{Implementation: "react"})
	require.NoError(t, err)
	require.NotEqual(t, reflect.TypeOf(&euclo.Agent{}), reflect.TypeOf(agent))
}

func TestInstantiateByNameRoutesCodingToEuclo(t *testing.T) {
	agent := InstantiateByName(t.TempDir(), "coding", testEnv(t))
	require.IsType(t, &euclo.Agent{}, agent)
}

func TestInstantiateByNameRoutesEucloAliasToEuclo(t *testing.T) {
	agent := InstantiateByName(t.TempDir(), "euclo", testEnv(t))
	require.IsType(t, &euclo.Agent{}, agent)
}

func TestBuildFromSpecRoutesRex(t *testing.T) {
	agent, err := BuildFromSpec(testEnv(t), core.AgentRuntimeSpec{Implementation: "rex"})
	require.NoError(t, err)
	require.IsType(t, &rex.Agent{}, agent)
}

func TestInstantiateByNameRoutesRex(t *testing.T) {
	workspace := t.TempDir()
	agent := InstantiateByName(workspace, "rex", testEnv(t))
	require.IsType(t, &rex.Agent{}, agent)
	rexAgent := agent.(*rex.Agent)
	require.Equal(t, workspace, rexAgent.Workspace)
}

func TestInstantiateByNameKeepsReactSeparate(t *testing.T) {
	agent := InstantiateByName(t.TempDir(), "react", testEnv(t))
	require.NotEqual(t, reflect.TypeOf(&euclo.Agent{}), reflect.TypeOf(agent))
}

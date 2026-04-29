package factory

import (
	"context"
	"reflect"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/blackboard"
	"codeburg.org/lexbit/relurpify/agents/goalcon"
	"codeburg.org/lexbit/relurpify/agents/htn"
	"codeburg.org/lexbit/relurpify/agents/pipeline"
	"codeburg.org/lexbit/relurpify/agents/planner"
	"codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/agents/reflection"
	"codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
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

func testEnv(t *testing.T) agentenv.WorkspaceEnvironment {
	t.Helper()
	return agentenv.WorkspaceEnvironment{
		Model:         factoryStubModel{},
		Registry:      capability.NewRegistry(),
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config: &core.Config{
			Name:          "factory-test",
			Model:         "stub",
			MaxIterations: 1,
		},
	}
}

type namedFactoryStubExecutor struct {
	id string
}

func (n *namedFactoryStubExecutor) Initialize(*core.Config) error { return nil }
func (n *namedFactoryStubExecutor) Execute(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error) {
	return &core.Result{NodeID: n.id, Success: true}, nil
}
func (n *namedFactoryStubExecutor) Capabilities() []string { return nil }
func (n *namedFactoryStubExecutor) BuildGraph(*core.Task) (*agentgraph.Graph, error) {
	return nil, nil
}

func TestBuildFromSpecRoutesKnownTypes(t *testing.T) {
	env := testEnv(t)
	cases := []struct {
		name string
		spec core.AgentRuntimeSpec
		want any
	}{
		{name: "react", spec: core.AgentRuntimeSpec{Implementation: "react"}, want: &react.ReActAgent{}},
		{name: "pipeline", spec: core.AgentRuntimeSpec{Implementation: "pipeline"}, want: &pipeline.PipelineAgent{}},
		{name: "planner", spec: core.AgentRuntimeSpec{Implementation: "planner"}, want: &planner.PlannerAgent{}},
		{name: "reflection", spec: core.AgentRuntimeSpec{Implementation: "reflection"}, want: &reflection.ReflectionAgent{}},
		{name: "htn", spec: core.AgentRuntimeSpec{Implementation: "htn"}, want: &htn.HTNAgent{}},
		{name: "blackboard", spec: core.AgentRuntimeSpec{Implementation: "blackboard"}, want: &blackboard.BlackboardAgent{}},
		{name: "rewoo", spec: core.AgentRuntimeSpec{Implementation: "rewoo"}, want: &rewoo.RewooAgent{}},
		{name: "goalcon", spec: core.AgentRuntimeSpec{Implementation: "goalcon"}, want: &goalcon.GoalConAgent{}},
		{name: "rex", spec: core.AgentRuntimeSpec{Implementation: "rex"}, want: &rex.Agent{}},
		{name: "composition fallback", spec: core.AgentRuntimeSpec{Composition: &core.AgentCompositionSpec{Type: "react"}}, want: &react.ReActAgent{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent, err := BuildFromSpec(env, tc.spec)
			require.NoError(t, err)
			require.Equal(t, reflect.TypeOf(tc.want), reflect.TypeOf(agent))
		})
	}
}

func TestBuildFromSpecRequiresImplementation(t *testing.T) {
	_, err := BuildFromSpec(testEnv(t), core.AgentRuntimeSpec{})
	require.Error(t, err)
}

func TestInstantiateByNameRoutesRegisteredAgent(t *testing.T) {
	t.Cleanup(func() { namedAgentRegistry.Delete("testfu") })
	namedAgentRegistry.Delete("testfu")
	RegisterNamedAgent("testfu", func(workspace string, env ayenitd.WorkspaceEnvironment) agentgraph.WorkflowExecutor {
		return &namedFactoryStubExecutor{id: workspace + ":" + env.Config.Name}
	})

	workspace := t.TempDir()
	env := testEnv(t)
	agent := InstantiateByName(workspace, "testfu", env)
	require.IsType(t, &namedFactoryStubExecutor{}, agent)
	require.Equal(t, workspace+":"+env.Config.Name, agent.(*namedFactoryStubExecutor).id)
}

func TestInstantiateByNameRoutesRexAndUnknowns(t *testing.T) {
	workspace := t.TempDir()
	env := testEnv(t)

	agent := InstantiateByName(workspace, "rex", env)
	require.IsType(t, &rex.Agent{}, agent)
	require.Equal(t, workspace, agent.(*rex.Agent).Workspace)

	unknown := InstantiateByName(workspace, "does-not-exist", env)
	require.IsType(t, &rex.Agent{}, unknown)
}

func TestApplyManifestDefaultsAndWithMemory(t *testing.T) {
	original := &core.AgentRuntimeSpec{Implementation: "react"}
	require.Same(t, original, ApplyManifestDefaults(original))

	spec := ApplyManifestDefaults(nil)
	require.NotNil(t, spec)
	require.Empty(t, spec.Implementation)

	env := testEnv(t)
	mem := memory.NewWorkingMemoryStore()
	scoped := WithMemory(env, mem)
	require.Same(t, mem, scoped.WorkingMemory)
	require.NotSame(t, env.WorkingMemory, scoped.WorkingMemory)
}

package factory

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	architectpkg "github.com/lexcodex/relurpify/agents/architect"
	blackboardpkg "github.com/lexcodex/relurpify/agents/blackboard"
	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	goalconpkg "github.com/lexcodex/relurpify/agents/goalcon"
	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	pipelinepkg "github.com/lexcodex/relurpify/agents/pipeline"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	rewoopkg "github.com/lexcodex/relurpify/agents/rewoo"
	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/rex"
	"github.com/stretchr/testify/require"
)

type namedFactoryStubExecutor struct {
	id string
}

func (n *namedFactoryStubExecutor) Initialize(*core.Config) error { return nil }
func (n *namedFactoryStubExecutor) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	return &core.Result{NodeID: n.id, Success: true}, nil
}
func (n *namedFactoryStubExecutor) Capabilities() []core.Capability { return nil }
func (n *namedFactoryStubExecutor) BuildGraph(*core.Task) (*graph.Graph, error) {
	return nil, nil
}

type factoryStubTool struct {
	name  string
	perms core.ToolPermissions
}

func (t factoryStubTool) Name() string                     { return t.name }
func (t factoryStubTool) Description() string              { return "stub" }
func (t factoryStubTool) Category() string                 { return "test" }
func (t factoryStubTool) Parameters() []core.ToolParameter { return nil }
func (t factoryStubTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t factoryStubTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (t factoryStubTool) Permissions() core.ToolPermissions               { return t.perms }
func (t factoryStubTool) Tags() []string                                  { return nil }

func noPermissionTool(name string) factoryStubTool {
	return factoryStubTool{name: name}
}

func toolWithPermissions(name string, perms *core.PermissionSet) factoryStubTool {
	return factoryStubTool{name: name, perms: core.ToolPermissions{Permissions: perms}}
}

func registryPrecheckCount(t *testing.T, reg *capability.Registry) int {
	t.Helper()
	value := reflect.ValueOf(reg).Elem().FieldByName("prechecks")
	require.True(t, value.IsValid())
	return value.Len()
}

func TestNamedFactoryHelpers(t *testing.T) {
	var env agentenv.AgentEnvironment
	converted := envToWorkspace(env)
	require.Equal(t, ayenitd.WorkspaceEnvironment{}, converted)

	spec := ApplyManifestDefaults(nil)
	require.NotNil(t, spec)
	require.Empty(t, spec.Implementation)

	original := &core.AgentRuntimeSpec{Implementation: "react"}
	require.Same(t, original, ApplyManifestDefaults(original))

	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	withMemory := WithMemory(testEnv(t), memStore)
	require.NotNil(t, withMemory.Memory)
	require.NotEqual(t, withMemory.Memory, env.Memory)
}

func TestRegisterNamedAgentAndInstantiateRegisteredNamedAgent(t *testing.T) {
	t.Cleanup(func() { namedAgentRegistry.Delete("testfu") })
	namedAgentRegistry.Delete("testfu")

	require.NotPanics(t, func() {
		RegisterNamedAgent("", nil)
		RegisterNamedAgent("testfu", nil)
	})

	workspace := t.TempDir()
	env := testEnv(t)
	_, ok := instantiateRegisteredNamedAgent(workspace, "testfu", envToWorkspace(env))
	require.False(t, ok)

	namedAgentRegistry.Store("testfu", "bad-value")
	_, ok = instantiateRegisteredNamedAgent(workspace, "testfu", envToWorkspace(env))
	require.False(t, ok)

	namedAgentRegistry.Delete("testfu")
	RegisterNamedAgent("  TestFu  ", func(workspace string, env ayenitd.WorkspaceEnvironment) graph.WorkflowExecutor {
		return &namedFactoryStubExecutor{id: workspace + ":" + env.Config.Name}
	})
	agent, ok := instantiateRegisteredNamedAgent(workspace, " testfu ", envToWorkspace(env))
	require.True(t, ok)
	require.IsType(t, &namedFactoryStubExecutor{}, agent)
	require.Equal(t, workspace+":"+env.Config.Name, agent.(*namedFactoryStubExecutor).id)
}

func TestScopeRegistryFiltersToolsAndAddsPrecheck(t *testing.T) {
	base := capability.NewRegistry()
	require.NoError(t, base.Register(noPermissionTool("plain")))
	require.NoError(t, base.Register(toolWithPermissions("write", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "/tmp/**"}},
	})))
	require.NoError(t, base.Register(toolWithPermissions("exec", &core.PermissionSet{
		Executables: []core.ExecutablePermission{{Binary: "git"}},
	})))
	require.NoError(t, base.Register(toolWithPermissions("network", &core.PermissionSet{
		Network: []core.NetworkPermission{{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443}},
	})))
	require.NoError(t, base.Register(toolWithPermissions("read", &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/tmp/**"}},
	})))

	nilScoped := ScopeRegistry(nil, ToolScope{})
	require.NotNil(t, nilScoped)

	scoped := ScopeRegistry(base, ToolScope{})
	_, ok := scoped.Get("plain")
	require.True(t, ok)
	_, ok = scoped.Get("read")
	require.True(t, ok)
	_, ok = scoped.Get("write")
	require.False(t, ok)
	_, ok = scoped.Get("exec")
	require.False(t, ok)
	_, ok = scoped.Get("network")
	require.False(t, ok)

	allowAll := ScopeRegistry(base, ToolScope{
		AllowWrite:   true,
		AllowExecute: true,
		AllowNetwork: true,
		WritePathGlobs: []string{
			"**/*.md",
		},
	})
	_, ok = allowAll.Get("write")
	require.True(t, ok)
	_, ok = allowAll.Get("exec")
	require.True(t, ok)
	_, ok = allowAll.Get("network")
	require.True(t, ok)
	require.Equal(t, 1, registryPrecheckCount(t, allowAll))
}

func TestBuildFromSpecRoutesAndErrors(t *testing.T) {
	env := testEnv(t)

	cases := []struct {
		name string
		spec core.AgentRuntimeSpec
		want any
	}{
		{name: "react", spec: core.AgentRuntimeSpec{Implementation: "react"}, want: &reactpkg.ReActAgent{}},
		{name: "coding", spec: core.AgentRuntimeSpec{Implementation: "coding"}, want: &euclo.Agent{}},
		{name: "rex", spec: core.AgentRuntimeSpec{Implementation: "rex"}, want: &rex.Agent{}},
		{name: "architect", spec: core.AgentRuntimeSpec{Implementation: "architect"}, want: &architectpkg.ArchitectAgent{}},
		{name: "pipeline", spec: core.AgentRuntimeSpec{Implementation: "pipeline"}, want: &pipelinepkg.PipelineAgent{}},
		{name: "planner", spec: core.AgentRuntimeSpec{Implementation: "planner"}, want: &plannerpkg.PlannerAgent{}},
		{name: "reflection", spec: core.AgentRuntimeSpec{Implementation: "reflection"}, want: &reflectionpkg.ReflectionAgent{}},
		{name: "chainer", spec: core.AgentRuntimeSpec{Implementation: "chainer"}, want: &chainerpkg.ChainerAgent{}},
		{name: "htn", spec: core.AgentRuntimeSpec{Implementation: "htn"}, want: &htnpkg.HTNAgent{}},
		{name: "blackboard", spec: core.AgentRuntimeSpec{Implementation: "blackboard"}, want: &blackboardpkg.BlackboardAgent{}},
		{name: "rewoo", spec: core.AgentRuntimeSpec{Implementation: "rewoo"}, want: &rewoopkg.RewooAgent{}},
		{name: "goalcon", spec: core.AgentRuntimeSpec{Implementation: "goalcon"}, want: &goalconpkg.GoalConAgent{}},
		{name: "composition fallback", spec: core.AgentRuntimeSpec{Composition: &core.AgentCompositionSpec{Type: "react"}}, want: &reactpkg.ReActAgent{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent, err := BuildFromSpec(env, tc.spec)
			require.NoError(t, err)
			require.Equal(t, reflect.TypeOf(tc.want), reflect.TypeOf(agent))
		})
	}

	t.Cleanup(func() { namedAgentRegistry.Delete("testfu") })
	namedAgentRegistry.Delete("testfu")
	_, err := BuildFromSpec(env, core.AgentRuntimeSpec{Implementation: "testfu"})
	require.Error(t, err)

	RegisterNamedAgent("testfu", func(workspace string, env ayenitd.WorkspaceEnvironment) graph.WorkflowExecutor {
		return &namedFactoryStubExecutor{id: strings.Join([]string{workspace, env.Config.Name}, ":")}
	})
	agent, err := BuildFromSpec(env, core.AgentRuntimeSpec{Implementation: "testfu"})
	require.NoError(t, err)
	require.IsType(t, &namedFactoryStubExecutor{}, agent)

	_, err = BuildFromSpec(env, core.AgentRuntimeSpec{})
	require.Error(t, err)
	_, err = BuildFromSpec(env, core.AgentRuntimeSpec{Implementation: "not-real"})
	require.Error(t, err)
}

func TestInstantiateByNameRoutesAndDefaults(t *testing.T) {
	env := testEnv(t)
	workspace := filepath.Join(t.TempDir(), "workspace")

	t.Cleanup(func() { namedAgentRegistry.Delete("testfu") })
	namedAgentRegistry.Delete("testfu")
	RegisterNamedAgent("testfu", func(workspace string, env ayenitd.WorkspaceEnvironment) graph.WorkflowExecutor {
		return &namedFactoryStubExecutor{id: workspace + ":" + env.Config.Name}
	})

	cases := []struct {
		name string
		want any
		check func(t *testing.T, agent graph.WorkflowExecutor)
	}{
		{name: "planner", want: &plannerpkg.PlannerAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, config.New(workspace).CheckpointsDir(), agent.(*plannerpkg.PlannerAgent).CheckpointPath)
		}},
		{name: "react", want: &reactpkg.ReActAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, config.New(workspace).CheckpointsDir(), agent.(*reactpkg.ReActAgent).CheckpointPath)
		}},
		{name: "coding", want: &euclo.Agent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, config.New(workspace).CheckpointsDir(), agent.(*euclo.Agent).CheckpointPath)
		}},
		{name: "euclo", want: &euclo.Agent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, config.New(workspace).CheckpointsDir(), agent.(*euclo.Agent).CheckpointPath)
		}},
		{name: "rex", want: &rex.Agent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, workspace, agent.(*rex.Agent).Workspace)
		}},
		{name: "reflection", want: &reflectionpkg.ReflectionAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			refl := agent.(*reflectionpkg.ReflectionAgent)
			delegate, ok := refl.Delegate.(*reactpkg.ReActAgent)
			require.True(t, ok)
			require.Equal(t, config.New(workspace).CheckpointsDir(), delegate.CheckpointPath)
		}},
		{name: "htn", want: &htnpkg.HTNAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, config.New(workspace).WorkflowStateFile(), agent.(*htnpkg.HTNAgent).CheckpointPath)
		}},
		{name: "rewoo", want: &rewoopkg.RewooAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.NotNil(t, agent)
		}},
		{name: "architect", want: &architectpkg.ArchitectAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			arch := agent.(*architectpkg.ArchitectAgent)
			require.Equal(t, config.New(workspace).CheckpointsDir(), arch.CheckpointPath)
			require.Equal(t, config.New(workspace).WorkflowStateFile(), arch.WorkflowStatePath)
		}},
		{name: "pipeline", want: &pipelinepkg.PipelineAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, config.New(workspace).WorkflowStateFile(), agent.(*pipelinepkg.PipelineAgent).WorkflowStatePath)
		}},
		{name: "chainer", want: &chainerpkg.ChainerAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.NotNil(t, agent)
		}},
		{name: "blackboard", want: &blackboardpkg.BlackboardAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.NotNil(t, agent)
		}},
		{name: "goalcon", want: &goalconpkg.GoalConAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.NotNil(t, agent)
		}},
		{name: "testfu", want: &namedFactoryStubExecutor{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, workspace+":"+env.Config.Name, agent.(*namedFactoryStubExecutor).id)
		}},
		{name: "unknown default", want: &reactpkg.ReActAgent{}, check: func(t *testing.T, agent graph.WorkflowExecutor) {
			require.Equal(t, config.New(workspace).CheckpointsDir(), agent.(*reactpkg.ReActAgent).CheckpointPath)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := tc.name
			if name == "unknown default" {
				name = "does-not-exist"
			}
			agent := InstantiateByName(workspace, name, env)
			require.Equal(t, reflect.TypeOf(tc.want), reflect.TypeOf(agent))
			tc.check(t, agent)
		})
	}
}

package factory

import (
	"fmt"
	"strings"
	"sync"

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
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/eternal"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/rex"
)

var namedAgentRegistry sync.Map

func RegisterNamedAgent(name string, ctor func(workspace string, env agentenv.AgentEnvironment) graph.WorkflowExecutor) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || ctor == nil {
		return
	}
	namedAgentRegistry.Store(name, ctor)
}

func instantiateRegisteredNamedAgent(workspace, name string, env agentenv.AgentEnvironment) (graph.WorkflowExecutor, bool) {
	value, ok := namedAgentRegistry.Load(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return nil, false
	}
	ctor, ok := value.(func(workspace string, env agentenv.AgentEnvironment) graph.WorkflowExecutor)
	if !ok || ctor == nil {
		return nil, false
	}
	return ctor(workspace, env), true
}

type ToolScope struct {
	AllowRead      bool
	AllowWrite     bool
	AllowExecute   bool
	AllowNetwork   bool
	WritePathGlobs []string
}

func ScopeRegistry(registry *capability.Registry, scope ToolScope) *capability.Registry {
	if registry == nil {
		return capability.NewRegistry()
	}
	cloned := registry.CloneFiltered(func(tool core.Tool) bool {
		perms := tool.Permissions()
		if perms.Permissions == nil {
			return true
		}
		for _, fs := range perms.Permissions.FileSystem {
			switch fs.Action {
			case core.FileSystemWrite:
				if !scope.AllowWrite {
					return false
				}
			case core.FileSystemExecute:
				if !scope.AllowExecute {
					return false
				}
			}
		}
		if len(perms.Permissions.Executables) > 0 && !scope.AllowExecute {
			return false
		}
		if len(perms.Permissions.Network) > 0 && !scope.AllowNetwork {
			return false
		}
		return true
	})
	if len(scope.WritePathGlobs) > 0 {
		cloned.AddPrecheck(capability.WritePathPrecheck{Globs: append([]string{}, scope.WritePathGlobs...)})
	}
	return cloned
}

func BuildFromSpec(env agentenv.AgentEnvironment, spec core.AgentRuntimeSpec) (graph.WorkflowExecutor, error) {
	agentType := strings.ToLower(strings.TrimSpace(spec.Implementation))
	if agentType == "" && spec.Composition != nil {
		agentType = strings.ToLower(strings.TrimSpace(spec.Composition.Type))
	}
	if agentType == "" {
		return nil, fmt.Errorf("agent implementation required")
	}
	switch agentType {
	case "react":
		return reactpkg.New(env), nil
	case "coding":
		return euclo.New(env), nil
	case "rex":
		return rex.NewWithWorkspace(env, ""), nil
	case "architect":
		return architectpkg.New(
			env,
			architectpkg.WithPlannerTools(ScopeRegistry(env.Registry, ToolScope{AllowRead: true})),
			architectpkg.WithExecutorTools(env.Registry),
		), nil
	case "pipeline":
		return pipelinepkg.New(env), nil
	case "planner":
		return plannerpkg.New(env), nil
	case "reflection":
		return reflectionpkg.New(env, reactpkg.New(env)), nil
	case "chainer":
		return chainerpkg.New(env), nil
	case "htn":
		return htnpkg.New(env, htnpkg.NewMethodLibrary()), nil
	case "blackboard":
		return blackboardpkg.New(env), nil
	case "rewoo":
		return rewoopkg.New(env), nil
	case "goalcon":
		return goalconpkg.New(env, goalconpkg.NewOperatorRegistry()), nil
	case "eternal":
		return eternal.New(env), nil
	case "testfu":
		if agent, ok := instantiateRegisteredNamedAgent("", "testfu", env); ok {
			return agent, nil
		}
		return nil, fmt.Errorf("unknown agent type %q", agentType)
	default:
		return nil, fmt.Errorf("unknown agent type %q", agentType)
	}
}

func InstantiateByName(workspace, name string, env agentenv.AgentEnvironment) graph.WorkflowExecutor {
	paths := config.New(workspace)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "planner":
		agent := plannerpkg.New(env)
		agent.CheckpointPath = paths.CheckpointsDir()
		return agent
	case "react":
		agent := reactpkg.New(env)
		agent.CheckpointPath = paths.CheckpointsDir()
		return agent
		case "coding", "euclo":
			agent := euclo.New(env)
			agent.CheckpointPath = paths.CheckpointsDir()
			_ = agent.Initialize(env.Config)
			return agent
		case "rex":
			agent := rex.NewWithWorkspace(env, workspace)
			_ = agent.Initialize(env.Config)
			return agent
	case "reflection":
		agent := reflectionpkg.New(env, nil)
		if delegate, ok := agent.Delegate.(*reactpkg.ReActAgent); ok {
			delegate.CheckpointPath = paths.CheckpointsDir()
		}
		return agent
	case "htn":
		agent := htnpkg.New(env, htnpkg.NewMethodLibrary())
		agent.CheckpointPath = paths.WorkflowStateFile()
		return agent
	case "rewoo":
		return rewoopkg.New(env)
	case "architect":
		agent := architectpkg.New(
			env,
			architectpkg.WithPlannerTools(ScopeRegistry(env.Registry, ToolScope{AllowRead: true})),
			architectpkg.WithExecutorTools(env.Registry),
		)
		agent.CheckpointPath = paths.CheckpointsDir()
		agent.WorkflowStatePath = paths.WorkflowStateFile()
		return agent
	case "pipeline":
		agent := pipelinepkg.New(env)
		agent.WorkflowStatePath = paths.WorkflowStateFile()
		return agent
	case "chainer":
		return chainerpkg.New(env)
	case "blackboard":
		return blackboardpkg.New(env)
	case "goalcon":
		return goalconpkg.New(env, goalconpkg.NewOperatorRegistry())
	case "eternal":
		return eternal.New(env)
	case "testfu":
		if agent, ok := instantiateRegisteredNamedAgent(workspace, "testfu", env); ok {
			return agent
		}
		agent := reactpkg.New(env)
		agent.CheckpointPath = paths.CheckpointsDir()
		return agent
	default:
		agent := reactpkg.New(env)
		agent.CheckpointPath = paths.CheckpointsDir()
		return agent
	}
}

func ApplyManifestDefaults(spec *core.AgentRuntimeSpec) *core.AgentRuntimeSpec {
	if spec == nil {
		return &core.AgentRuntimeSpec{}
	}
	return spec
}

func WithMemory(env agentenv.AgentEnvironment, mem memory.MemoryStore) agentenv.AgentEnvironment {
	return env.WithMemory(mem)
}

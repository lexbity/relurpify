package factory

import (
	"fmt"
	"strings"
	"sync"

	architectpkg "codeburg.org/lexbit/relurpify/agents/architect"
	blackboardpkg "codeburg.org/lexbit/relurpify/agents/blackboard"
	chainerpkg "codeburg.org/lexbit/relurpify/agents/chainer"
	goalconpkg "codeburg.org/lexbit/relurpify/agents/goalcon"
	htnpkg "codeburg.org/lexbit/relurpify/agents/htn"
	pipelinepkg "codeburg.org/lexbit/relurpify/agents/pipeline"
	plannerpkg "codeburg.org/lexbit/relurpify/agents/planner"
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	reflectionpkg "codeburg.org/lexbit/relurpify/agents/reflection"
	rewoopkg "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/rex"
)

var namedAgentRegistry sync.Map

// RegisterNamedAgent registers a named agent constructor under the given name.
// The constructor receives ayenitd.WorkspaceEnvironment so it can access all
// workspace services. Named agents that only need the common fields can ignore
// the extra fields; they will be nil/zero when invoked from lower-level callers.
func RegisterNamedAgent(name string, ctor func(workspace string, env ayenitd.WorkspaceEnvironment) graph.WorkflowExecutor) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || ctor == nil {
		return
	}
	namedAgentRegistry.Store(name, ctor)
}

func instantiateRegisteredNamedAgent(workspace, name string, env ayenitd.WorkspaceEnvironment) (graph.WorkflowExecutor, bool) {
	value, ok := namedAgentRegistry.Load(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return nil, false
	}
	ctor, ok := value.(func(workspace string, env ayenitd.WorkspaceEnvironment) graph.WorkflowExecutor)
	if !ok || ctor == nil {
		return nil, false
	}
	return ctor(workspace, env), true
}

// envToWorkspace converts an AgentEnvironment to a WorkspaceEnvironment for
// passing to named agent constructors. Fields not present in AgentEnvironment
// (WorkflowStore, PlanStore, etc.) are left as zero/nil values.
func envToWorkspace(env agentenv.AgentEnvironment) ayenitd.WorkspaceEnvironment {
	ws := ayenitd.WorkspaceEnvironment{
		Config:        env.Config,
		Model:         env.Model,
		Registry:      env.Registry,
		IndexManager:  env.IndexManager,
		SearchEngine:  env.SearchEngine,
		Memory:        env.Memory,
		WorkflowStore: env.WorkflowStore,
		// VerificationPlanner and CompatibilitySurfaceExtractor are different interface
		// types between agentenv and ayenitd packages — left nil here.
		// Callers that need these should pass WorkspaceEnvironment directly.
	}
	// Type assert PlanStore from any to plan.PlanStore
	if env.PlanStore != nil {
		if ps, ok := env.PlanStore.(plan.PlanStore); ok {
			ws.PlanStore = ps
		}
	}
	return ws
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
		return euclo.New(envToWorkspace(env)), nil
	case "rex":
		return rex.NewWithWorkspace(envToWorkspace(env), ""), nil
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
	case "testfu":
		if agent, ok := instantiateRegisteredNamedAgent("", "testfu", envToWorkspace(env)); ok {
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
	case "coding", "euclo", "euclo:debug", "euclo:chat", "euclo:planning":
		agent := euclo.New(envToWorkspace(env))
		agent.CheckpointPath = paths.CheckpointsDir()
		_ = agent.Initialize(env.Config)
		return agent
	case "rex":
		agent := rex.NewWithWorkspace(envToWorkspace(env), workspace)
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
	case "testfu":
		if agent, ok := instantiateRegisteredNamedAgent(workspace, "testfu", envToWorkspace(env)); ok {
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

package factory

import (
	"fmt"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/agents"
	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/rex"
)

var namedAgentRegistry sync.Map

// RegisterNamedAgent registers a named agent constructor under the given name.
// The constructor receives ayenitd.WorkspaceEnvironment so it can access all
// workspace services. Named agents that only need the common fields can ignore
// the extra fields; they will be nil/zero when invoked from lower-level callers.
func RegisterNamedAgent(name string, ctor func(workspace string, env ayenitd.WorkspaceEnvironment) agentgraph.WorkflowExecutor) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || ctor == nil {
		return
	}
	namedAgentRegistry.Store(name, ctor)
}

func instantiateRegisteredNamedAgent(workspace, name string, env ayenitd.WorkspaceEnvironment) (agentgraph.WorkflowExecutor, bool) {
	value, ok := namedAgentRegistry.Load(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return nil, false
	}
	ctor, ok := value.(func(workspace string, env ayenitd.WorkspaceEnvironment) agentgraph.WorkflowExecutor)
	if !ok || ctor == nil {
		return nil, false
	}
	return ctor(workspace, env), true
}

// envToWorkspace normalizes legacy callers onto WorkspaceEnvironment.
func envToWorkspace(env agentenv.WorkspaceEnvironment) ayenitd.WorkspaceEnvironment {
	return ayenitd.WorkspaceEnvironment(env)
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

func BuildFromSpec(env agentenv.WorkspaceEnvironment, spec core.AgentRuntimeSpec) (agentgraph.WorkflowExecutor, error) {
	agentType := strings.ToLower(strings.TrimSpace(spec.Implementation))
	if agentType == "" && spec.Composition != nil {
		agentType = strings.ToLower(strings.TrimSpace(spec.Composition.Type))
	}
	if agentType == "" {
		return nil, fmt.Errorf("agent implementation required")
	}
	switch agentType {
	case "rex":
		return rex.NewWithWorkspace(&env, ""), nil
	default:
		return agents.BuildFromSpec(&env, spec)
	}
}

func InstantiateByName(workspace, name string, env agentenv.WorkspaceEnvironment) agentgraph.WorkflowExecutor {
	if agent, ok := instantiateRegisteredNamedAgent(workspace, name, envToWorkspace(env)); ok {
		return agent
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rex":
		agent := rex.NewWithWorkspace(&env, workspace)
		_ = agent.Initialize(env.Config)
		return agent
	}
	agent, err := BuildFromSpec(env, core.AgentRuntimeSpec{Implementation: name})
	if err != nil {
		agent = rex.NewWithWorkspace(&env, workspace)
		_ = agent.Initialize(env.Config)
	}
	return agent
}

func ApplyManifestDefaults(spec *core.AgentRuntimeSpec) *core.AgentRuntimeSpec {
	if spec == nil {
		return &core.AgentRuntimeSpec{}
	}
	return spec
}

func WithMemory(env agentenv.WorkspaceEnvironment, mem *memory.WorkingMemoryStore) agentenv.WorkspaceEnvironment {
	return env.WithMemory(mem)
}

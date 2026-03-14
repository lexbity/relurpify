package relurpic

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	namedfactory "github.com/lexcodex/relurpify/named/factory"
)

type AgentCapabilityHandler struct {
	env       agentenv.AgentEnvironment
	agentType string
	policy    core.AgentInvocationPolicy
}

func (h *AgentCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return agentCapabilityDescriptor(h.agentType, h.policy)
}

func (h *AgentCapabilityHandler) Invoke(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	if state == nil {
		state = core.NewContext()
	}
	childMemory, err := resolveMemory(h.env.Memory, h.policy.MemoryMode)
	if err != nil {
		return nil, err
	}
	childRegistry := resolveRegistry(h.env.Registry, h.policy.ToolScope)
	childState := resolveState(state, h.policy.StateMode)
	childEnv := h.env.WithRegistry(childRegistry).WithMemory(childMemory)

	agent, err := buildAgentFromEnvironment(childEnv, h.agentType)
	if err != nil {
		return nil, err
	}
	task := taskFromArgs(h.agentType, args)
	result, err := agent.Execute(ctx, task, childState)
	if err != nil {
		return nil, err
	}
	if h.policy.StateMode == core.StateModeCloned {
		state.Merge(childState)
	}
	return toToolResult(result), nil
}

func buildAgentFromEnvironment(env agentenv.AgentEnvironment, agentType string) (graph.Agent, error) {
	var agent graph.Agent
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "react":
		agent = &reactpkg.ReActAgent{}
	case "architect":
		agent = &architectpkg.ArchitectAgent{
			PlannerTools:  readonlyRegistry(env.Registry),
			ExecutorTools: env.Registry,
		}
	case "planner":
		agent = &plannerpkg.PlannerAgent{}
	case "pipeline":
		agent = &pipelinepkg.PipelineAgent{}
	case "reflection":
		agent = &reflectionpkg.ReflectionAgent{Delegate: &reactpkg.ReActAgent{}}
	case "chainer":
		agent = &chainerpkg.ChainerAgent{Chain: &chainerpkg.Chain{Links: []chainerpkg.Link{
			chainerpkg.NewSummarizeLink("default", nil, "chainer.output"),
		}}}
	case "htn":
		agent = &htnpkg.HTNAgent{}
	case "blackboard":
		agent = &blackboardpkg.BlackboardAgent{}
	case "rewoo":
		agent = &rewoopkg.RewooAgent{}
	case "goalcon":
		agent = &goalconpkg.GoalConAgent{}
	case "testfu":
		built, err := namedfactory.BuildFromSpec(env, core.AgentRuntimeSpec{Implementation: "testfu"})
		if err != nil {
			return nil, err
		}
		agent = built
	default:
		return nil, fmt.Errorf("unknown agent type %q", agentType)
	}
	if envAware, ok := agent.(interface {
		InitializeEnvironment(agentenv.AgentEnvironment) error
	}); ok {
		if err := envAware.InitializeEnvironment(env); err != nil {
			return nil, err
		}
		return agent, nil
	}
	if err := agent.Initialize(env.Config); err != nil {
		return nil, err
	}
	return agent, nil
}

func resolveMemory(base memory.MemoryStore, mode core.MemoryMode) (memory.MemoryStore, error) {
	switch mode {
	case "", core.MemoryModeShared:
		return base, nil
	case core.MemoryModeFresh, core.MemoryModeCloned:
		dir, err := os.MkdirTemp("", "relurpify-agent-memory-*")
		if err != nil {
			return nil, err
		}
		store, err := memory.NewHybridMemory(dir)
		if err != nil {
			return nil, err
		}
		return store.WithVectorStore(memory.NewInMemoryVectorStore()), nil
	default:
		return base, nil
	}
}

func resolveRegistry(base *capability.Registry, scope core.ToolScopePolicy) *capability.Registry {
	switch scope {
	case "", core.ToolScopeInherits, core.ToolScopeCustom:
		if base == nil {
			return capability.NewRegistry()
		}
		return base
	case core.ToolScopeScoped:
		return readonlyRegistry(base)
	default:
		if base == nil {
			return capability.NewRegistry()
		}
		return base
	}
}

func resolveState(state *core.Context, mode core.StateMode) *core.Context {
	switch mode {
	case "", core.StateModeShared:
		return state
	case core.StateModeFresh:
		return core.NewContext()
	case core.StateModeCloned, core.StateModeForked:
		return state.Clone()
	default:
		return state
	}
}

func readonlyRegistry(registry *capability.Registry) *capability.Registry {
	if registry == nil {
		return capability.NewRegistry()
	}
	return registry.CloneFiltered(func(tool capability.Tool) bool {
		perms := tool.Permissions()
		if perms.Permissions == nil {
			return true
		}
		for _, fs := range perms.Permissions.FileSystem {
			if fs.Action == core.FileSystemWrite || fs.Action == core.FileSystemExecute {
				return false
			}
		}
		if len(perms.Permissions.Executables) > 0 || len(perms.Permissions.Network) > 0 {
			return false
		}
		return true
	})
}

func taskFromArgs(agentType string, args map[string]interface{}) *core.Task {
	taskType := core.TaskTypeCodeModification
	if strings.EqualFold(agentType, "planner") {
		taskType = core.TaskTypeAnalysis
	}
	task := &core.Task{
		ID:          strings.TrimSpace(fmt.Sprint(args["task_id"])),
		Instruction: strings.TrimSpace(fmt.Sprint(args["instruction"])),
		Type:        taskType,
		Context:     map[string]any{},
	}
	if task.ID == "" {
		task.ID = strings.ReplaceAll(agentType, ":", "-")
	}
	for _, key := range []string{"workflow_id", "context_summary", "artifact_summary"} {
		if value := strings.TrimSpace(fmt.Sprint(args[key])); value != "" {
			task.Context[key] = value
		}
	}
	for _, key := range []string{"acceptance_criteria", "verification_criteria", "args"} {
		if value, ok := args[key]; ok && value != nil {
			task.Context[key] = value
		}
	}
	return task
}

func toToolResult(result *core.Result) *core.CapabilityExecutionResult {
	payload := map[string]any{}
	if result != nil && result.Data != nil {
		for key, value := range result.Data {
			payload[key] = value
		}
	}
	if result != nil {
		payload["success"] = result.Success
		if result.NodeID != "" {
			payload["node_id"] = result.NodeID
		}
		if result.Error != nil {
			payload["error"] = result.Error.Error()
		}
	}
	return &core.CapabilityExecutionResult{
		Success: result == nil || result.Success,
		Data:    payload,
	}
}

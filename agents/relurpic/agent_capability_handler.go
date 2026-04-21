package relurpic

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
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
	seedTaskState(childState, task)
	result, err := agent.Execute(ctx, task, childState)
	if err != nil {
		return nil, err
	}
	if h.policy.StateMode == core.StateModeCloned {
		state.Merge(childState)
	}
	return toToolResult(result), nil
}

func buildAgentFromEnvironment(env agentenv.AgentEnvironment, agentType string) (graph.WorkflowExecutor, error) {
	var agent graph.WorkflowExecutor
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
		// testfu is a named test-runner agent, not a general-purpose subagent type.
		// Use named/factory.BuildFromSpec or named/testfu directly if you need testfu.
		return nil, fmt.Errorf("agent type %q is not available as a relurpic subagent", agentType)
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
	if raw := strings.TrimSpace(fmt.Sprint(args["task_type"])); raw != "" {
		taskType = core.TaskType(raw)
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

func seedTaskState(state *core.Context, task *core.Task) {
	if state == nil || task == nil {
		return
	}
	if task.ID != "" {
		state.Set("task.id", task.ID)
	}
	if task.Instruction != "" {
		state.Set("task.instruction", task.Instruction)
	}
	if task.Type != "" {
		state.Set("task.type", string(task.Type))
	}
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
	out := &core.CapabilityExecutionResult{
		Success: result == nil || result.Success,
		Data:    payload,
	}
	if result != nil {
		if len(result.Metadata) > 0 {
			out.Metadata = make(map[string]any, len(result.Metadata))
			for key, value := range result.Metadata {
				out.Metadata[key] = value
			}
		}
		if result.Error != nil {
			out.Error = result.Error.Error()
		}
	}
	return out
}

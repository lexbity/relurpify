package relurpic

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type plannerPlanCapabilityHandler struct {
	model    core.LanguageModel
	registry *capability.Registry
	config   *core.Config
}

func (h plannerPlanCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:planner.plan",
		"planner.plan",
		"Build a structured execution plan using the built-in planner workflow.",
		core.CapabilityKindTool,
		core.CoordinationRolePlanner,
		[]string{"plan"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		plannerInputSchema(),
		plannerOutputSchema(),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "planner",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h plannerPlanCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	instruction := strings.TrimSpace(fmt.Sprint(args["instruction"]))
	if instruction == "" {
		return nil, fmt.Errorf("instruction required")
	}
	result, err := (&AgentCapabilityHandler{
		env: agentenv.AgentEnvironment{
			Model:    h.model,
			Registry: h.registry,
			Config:   h.plannerConfig(),
		},
		agentType: "planner",
		policy: core.AgentInvocationPolicy{
			MemoryMode: DefaultInvocationPolicies["planner"].MemoryMode,
			StateMode:  core.StateModeShared,
			ToolScope:  DefaultInvocationPolicies["planner"].ToolScope,
		},
	}).Invoke(ctx, env, args)
	if err != nil {
		return nil, err
	}

	plan, _ := env.GetWorkingValue("planner.plan")
	summaryRaw, _ := env.GetWorkingValue("planner.summary")
	summary := ""
	if s, ok := summaryRaw.(string); ok {
		summary = s
	}
	data := map[string]interface{}{
		"goal":         "",
		"steps":        []any{},
		"files":        []any{},
		"dependencies": map[string]any{},
		"summary":      summary,
	}
	if typed, ok := plan.(agentgraph.Plan); ok {
		data["goal"] = typed.Goal
		data["steps"] = planStepsAsAny(typed.Steps)
		data["files"] = planFilesAsAny(typed.Files)
		data["dependencies"] = planDependenciesAsAny(typed.Dependencies)
	}
	if result != nil && len(result.Data) > 0 {
		data["result"] = result.Data
	}
	env.SetWorkingValue("active_plan", result.Data["plan"], contextdata.MemoryClassTask)
	return &core.CapabilityExecutionResult{Success: true, Data: data}, nil
}

func (h plannerPlanCapabilityHandler) plannerConfig() *core.Config {
	if h.config == nil {
		return &core.Config{Name: "planner.plan"}
	}
	cfg := *h.config
	cfg.Name = "planner.plan"
	cfg.AgentSpec = core.MergeAgentSpecs(h.manifest.AgentSpec)
	return &cfg
}

type architectExecuteCapabilityHandler struct {
	model    core.LanguageModel
	registry *capability.Registry
	config   *core.Config
}

func (h architectExecuteCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:architect.execute",
		"architect.execute",
		"Execute a task through the built-in architect workflow with explicit planning and bounded execution.",
		core.CapabilityKindTool,
		core.CoordinationRoleArchitect,
		[]string{"design", "implement"},
		[]core.CoordinationExecutionMode{
			core.CoordinationExecutionModeSync,
			core.CoordinationExecutionModeBackgroundAgent,
		},
		structuredTaskSchema("instruction", "task_id", "workflow_id", "context_summary"),
		structuredObjectSchema(map[string]*core.Schema{
			"summary":       {Type: "string"},
			"workflow_id":   {Type: "string"},
			"run_id":        {Type: "string"},
			"completed":     {Type: "array", Items: &core.Schema{Type: "string"}},
			"plan":          {Type: "object"},
			"planner":       {Type: "object"},
			"result":        {Type: "object"},
			"workflow_mode": {Type: "string"},
		}, "summary"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "architect",
		},
		[]core.RiskClass{core.RiskClassExecute},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h architectExecuteCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	instruction := strings.TrimSpace(fmt.Sprint(args["instruction"]))
	if instruction == "" {
		return nil, fmt.Errorf("instruction required")
	}

	taskID := strings.TrimSpace(fmt.Sprint(args["task_id"]))
	if taskID == "" {
		taskID = "architect.execute"
	}
	requestedWorkflowID := strings.TrimSpace(fmt.Sprint(args["workflow_id"]))
	task := &core.Task{
		ID:          taskID,
		Instruction: instruction,
		Type:        core.TaskTypeCodeModification,
		Context: map[string]any{
			"mode":            "architect",
			"context_summary": strings.TrimSpace(fmt.Sprint(args["context_summary"])),
		},
	}
	if requestedWorkflowID != "" {
		task.Context["workflow_id"] = requestedWorkflowID
	}
	result, err := (&AgentCapabilityHandler{
		env: agentenv.AgentEnvironment{
			Model:    h.model,
			Registry: h.registry,
			Config:   h.architectConfig(),
		},
		agentType: "architect",
		policy:    DefaultInvocationPolicies["architect"],
	}).Invoke(ctx, env, map[string]interface{}{
		"instruction":     task.Instruction,
		"task_id":         task.ID,
		"workflow_id":     requestedWorkflowID,
		"context_summary": task.Context["context_summary"],
	})
	if err != nil {
		return nil, err
	}

	data := map[string]any{
		"summary":       envGetString(env, "architect.summary"),
		"workflow_id":   envGetString(env, "architect.workflow_id"),
		"run_id":        envGetString(env, "architect.run_id"),
		"completed":     planFilesAsAny(envStringSliceFromContext(env, "architect.completed_steps")),
		"workflow_mode": "architect",
	}
	if plan, ok := env.GetWorkingValue("architect.plan"); ok {
		data["plan"] = normalizePlanPayload(plan)
	}
	if result != nil {
		data["result"] = result.Data
		if summary, ok := result.Data["summary"].(string); ok && strings.TrimSpace(summary) != "" {
			data["summary"] = summary
		}
		if planner, ok := result.Data["planner"]; ok {
			data["planner"] = planner
		}
	}
	if summary := strings.TrimSpace(fmt.Sprint(data["summary"])); summary == "" {
		data["summary"] = fmt.Sprintf("Architect executed delegated task %q.", instruction)
	}
	if strings.TrimSpace(fmt.Sprint(data["workflow_id"])) == "" && requestedWorkflowID != "" {
		data["workflow_id"] = requestedWorkflowID
	}
	env.SetWorkingValue("architect_result", result.Data, contextdata.MemoryClassTask)
	return &core.CapabilityExecutionResult{Success: true, Data: data}, nil
}

func (h architectExecuteCapabilityHandler) architectConfig() *core.Config {
	if h.config == nil {
		return &core.Config{Name: "architect.execute", MaxIterations: 3, NativeToolCalling: true}
	}
	cfg := *h.config
	cfg.Name = "architect.execute"
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 3
	}
	if !cfg.NativeToolCalling {
		cfg.NativeToolCalling = true
	}
	cfg.AgentSpec = core.MergeAgentSpecs(h.manifest.AgentSpec)
	return &cfg
}

type executorInvokeCapabilityHandler struct {
	registry *capability.Registry
}

func (h executorInvokeCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:executor.invoke",
		"executor.invoke",
		"Invoke a specific admitted non-coordination capability as a narrow execution target.",
		core.CapabilityKindTool,
		core.CoordinationRoleExecutor,
		[]string{"execute", "invoke"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredObjectSchema(map[string]*core.Schema{
			"capability": {Type: "string"},
			"args":       {Type: "object"},
		}, "capability"),
		structuredObjectSchema(map[string]*core.Schema{
			"summary":       {Type: "string"},
			"capability_id": {Type: "string"},
			"capability":    {Type: "string"},
			"result":        {Type: "object"},
		}, "summary", "capability_id", "capability"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "executor",
			"narrow":              true,
		},
		[]core.RiskClass{core.RiskClassExecute},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h executorInvokeCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	if h.registry == nil {
		return nil, fmt.Errorf("capability registry required")
	}
	targetName := strings.TrimSpace(fmt.Sprint(args["capability"]))
	if targetName == "" {
		return nil, fmt.Errorf("capability required")
	}

	target, ok := h.registry.GetCapability(targetName)
	if !ok {
		return nil, fmt.Errorf("capability %s not found", targetName)
	}
	if target.Coordination != nil && target.Coordination.Target {
		return nil, fmt.Errorf("capability %s is itself a coordination target", targetName)
	}
	if h.registry.EffectiveExposure(target) != core.CapabilityExposureCallable {
		return nil, fmt.Errorf("capability %s is not callable", targetName)
	}

	callArgs, _ := args["args"].(map[string]any)
	if callArgs == nil {
		callArgs = map[string]any{}
	}

	result, err := h.registry.InvokeCapability(ctx, env, target.ID, callArgs)
	if err != nil {
		return nil, err
	}
	execResult := &core.CapabilityExecutionResult{
		Success: result.Success,
		Data: map[string]any{
			"summary":       fmt.Sprintf("Executed delegated capability %s.", target.Name),
			"capability_id": target.ID,
			"capability":    target.Name,
			"result":        result.Data,
		},
		Error: result.Error,
	}
	env.SetWorkingValue("executor_result", execResult.Data, contextdata.MemoryClassTask)
	return execResult, nil
}

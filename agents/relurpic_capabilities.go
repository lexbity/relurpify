package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/agents/pattern"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// RegisterBuiltinRelurpicCapabilities installs framework-native orchestrated
// capabilities that are reusable across agents without treating them as local tools.
func RegisterBuiltinRelurpicCapabilities(registry *capability.Registry, model core.LanguageModel, cfg *core.Config) error {
	if registry == nil || model == nil {
		return nil
	}
	handlers := []core.InvocableCapabilityHandler{
		plannerPlanCapabilityHandler{model: model, registry: registry, config: cfg},
		architectExecuteCapabilityHandler{model: model, registry: registry, config: cfg},
		reviewerReviewCapabilityHandler{model: model, config: cfg},
		verifierVerifyCapabilityHandler{model: model, config: cfg},
		executorInvokeCapabilityHandler{registry: registry},
	}
	for _, handler := range handlers {
		if err := registry.RegisterInvocableCapability(handler); err != nil {
			return err
		}
	}
	return nil
}

type plannerPlanCapabilityHandler struct {
	model    core.LanguageModel
	registry *capability.Registry
	config   *core.Config
}

func (h plannerPlanCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
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

func (h plannerPlanCapabilityHandler) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	instruction := strings.TrimSpace(fmt.Sprint(args["instruction"]))
	if instruction == "" {
		return nil, fmt.Errorf("instruction required")
	}
	planner := &PlannerAgent{
		Model: h.model,
		Tools: h.registry,
	}
	if err := planner.Initialize(h.plannerConfig()); err != nil {
		return nil, err
	}
	task := &core.Task{
		ID:          strings.TrimSpace(fmt.Sprint(args["task_id"])),
		Instruction: instruction,
	}
	state := core.NewContext()
	result, err := planner.Execute(ctx, task, state)
	if err != nil {
		return nil, err
	}
	plan, _ := state.Get("planner.plan")
	summary := state.GetString("planner.summary")
	data := map[string]interface{}{
		"goal":         "",
		"steps":        []any{},
		"files":        []any{},
		"dependencies": map[string]any{},
		"summary":      summary,
	}
	if typed, ok := plan.(core.Plan); ok {
		data["goal"] = typed.Goal
		data["steps"] = planStepsAsAny(typed.Steps)
		data["files"] = planFilesAsAny(typed.Files)
		data["dependencies"] = planDependenciesAsAny(typed.Dependencies)
	}
	if result != nil && len(result.Data) > 0 {
		data["result"] = result.Data
	}
	return &core.CapabilityExecutionResult{Success: true, Data: data}, nil
}

func (h plannerPlanCapabilityHandler) plannerConfig() *core.Config {
	if h.config == nil {
		return &core.Config{Name: "planner.plan"}
	}
	cfg := *h.config
	cfg.Name = "planner.plan"
	cfg.AgentSpec = core.MergeAgentSpecs(h.config.AgentSpec)
	return &cfg
}

type architectExecuteCapabilityHandler struct {
	model    core.LanguageModel
	registry *capability.Registry
	config   *core.Config
}

func (h architectExecuteCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
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

func (h architectExecuteCapabilityHandler) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
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
			"mode":            string(ModeArchitect),
			"context_summary": strings.TrimSpace(fmt.Sprint(args["context_summary"])),
		},
	}
	if requestedWorkflowID != "" {
		task.Context["workflow_id"] = requestedWorkflowID
	}
	state := core.NewContext()
	agent := &ArchitectAgent{
		Model:         h.model,
		PlannerTools:  h.registry,
		ExecutorTools: h.registry,
	}
	if err := agent.Initialize(h.architectConfig()); err != nil {
		return nil, err
	}
	result, err := agent.Execute(ctx, task, state)
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"summary":       state.GetString("architect.summary"),
		"workflow_id":   state.GetString("architect.workflow_id"),
		"run_id":        state.GetString("architect.run_id"),
		"completed":     planFilesAsAny(core.StringSliceFromContext(state, "architect.completed_steps")),
		"workflow_mode": "architect",
	}
	if plan, ok := state.Get("architect.plan"); ok {
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
	return &core.CapabilityExecutionResult{Success: true, Data: data}, nil
}

type reviewerReviewCapabilityHandler struct {
	model  core.LanguageModel
	config *core.Config
}

func (h reviewerReviewCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:reviewer.review",
		"reviewer.review",
		"Review a provided change summary or artifact bundle and return structured findings.",
		core.CapabilityKindTool,
		core.CoordinationRoleReviewer,
		[]string{"review"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredTaskSchema("instruction", "artifact_summary", "acceptance_criteria"),
		structuredObjectSchema(map[string]*core.Schema{
			"summary": {Type: "string"},
			"approve": {Type: "boolean"},
			"findings": {
				Type: "array",
				Items: &core.Schema{
					Type: "object",
					Properties: map[string]*core.Schema{
						"severity":    {Type: "string"},
						"description": {Type: "string"},
						"suggestion":  {Type: "string"},
					},
					Required: []string{"severity", "description"},
				},
			},
		}, "summary", "approve", "findings"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "review",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h reviewerReviewCapabilityHandler) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	instruction := strings.TrimSpace(fmt.Sprint(args["instruction"]))
	if instruction == "" {
		return nil, fmt.Errorf("instruction required")
	}
	prompt := fmt.Sprintf(`You are a code and artifact reviewer.
Task: %s
Artifact summary: %s
Acceptance criteria: %s
Return valid JSON: {"summary":string,"approve":bool,"findings":[{"severity":"high|medium|low","description":string,"suggestion":string}]}.`,
		instruction,
		strings.TrimSpace(fmt.Sprint(args["artifact_summary"])),
		stringifyContextValue(args["acceptance_criteria"]),
	)
	return invokeStructuredReasoner(ctx, h.model, modelName(h.config), prompt)
}

type verifierVerifyCapabilityHandler struct {
	model  core.LanguageModel
	config *core.Config
}

func (h verifierVerifyCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:verifier.verify",
		"verifier.verify",
		"Verify that a proposed result satisfies explicit criteria and identify remaining gaps.",
		core.CapabilityKindTool,
		core.CoordinationRoleVerifier,
		[]string{"verify"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredTaskSchema("instruction", "artifact_summary", "verification_criteria"),
		structuredObjectSchema(map[string]*core.Schema{
			"summary":       {Type: "string"},
			"verified":      {Type: "boolean"},
			"evidence":      {Type: "array", Items: &core.Schema{Type: "string"}},
			"missing_items": {Type: "array", Items: &core.Schema{Type: "string"}},
		}, "summary", "verified", "evidence", "missing_items"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "verify",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h verifierVerifyCapabilityHandler) Invoke(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	instruction := strings.TrimSpace(fmt.Sprint(args["instruction"]))
	if instruction == "" {
		return nil, fmt.Errorf("instruction required")
	}
	prompt := fmt.Sprintf(`You are a verification specialist.
Task: %s
Artifact summary: %s
Verification criteria: %s
Return valid JSON: {"summary":string,"verified":bool,"evidence":[string],"missing_items":[string]}.`,
		instruction,
		strings.TrimSpace(fmt.Sprint(args["artifact_summary"])),
		stringifyContextValue(args["verification_criteria"]),
	)
	return invokeStructuredReasoner(ctx, h.model, modelName(h.config), prompt)
}

type executorInvokeCapabilityHandler struct {
	registry *capability.Registry
}

func (h executorInvokeCapabilityHandler) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
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

func (h executorInvokeCapabilityHandler) Invoke(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
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
	if state == nil {
		state = core.NewContext()
	}
	result, err := h.registry.InvokeCapability(ctx, state, target.ID, callArgs)
	if err != nil {
		return nil, err
	}
	return &core.CapabilityExecutionResult{
		Success: result.Success,
		Data: map[string]any{
			"summary":       fmt.Sprintf("Executed delegated capability %s.", target.Name),
			"capability_id": target.ID,
			"capability":    target.Name,
			"result":        result.Data,
		},
		Error: result.Error,
	}, nil
}

func coordinatedRelurpicDescriptor(id, name, description string, kind core.CapabilityKind, role core.CoordinationRole, taskTypes []string, executionModes []core.CoordinationExecutionMode, input, output *core.Schema, annotations map[string]any, riskClasses []core.RiskClass, effectClasses []core.EffectClass) core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            id,
		Kind:          kind,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          name,
		Description:   description,
		Category:      "relurpic-orchestration",
		Tags:          []string{"coordination", "relurpic", string(role)},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   append([]core.RiskClass{}, riskClasses...),
		EffectClasses: append([]core.EffectClass{}, effectClasses...),
		InputSchema:   input,
		OutputSchema:  output,
		Coordination: &core.CoordinationTargetMetadata{
			Target:                 true,
			Role:                   role,
			TaskTypes:              taskTypes,
			ExecutionModes:         executionModes,
			LongRunning:            false,
			MaxDepth:               1,
			MaxRuntimeSeconds:      600,
			DirectInsertionAllowed: false,
		},
		Availability: core.AvailabilitySpec{Available: true},
		Annotations:  annotations,
	})
}

func structuredTaskSchema(required ...string) *core.Schema {
	properties := map[string]*core.Schema{
		"instruction":           {Type: "string"},
		"task_id":               {Type: "string"},
		"workflow_id":           {Type: "string"},
		"context_summary":       {Type: "string"},
		"artifact_summary":      {Type: "string"},
		"acceptance_criteria":   {Type: "array", Items: &core.Schema{Type: "string"}},
		"verification_criteria": {Type: "array", Items: &core.Schema{Type: "string"}},
	}
	return &core.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func structuredObjectSchema(properties map[string]*core.Schema, required ...string) *core.Schema {
	return &core.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func plannerInputSchema() *core.Schema {
	return structuredTaskSchema("instruction")
}

func plannerOutputSchema() *core.Schema {
	return &core.Schema{
		Type: "object",
		Properties: map[string]*core.Schema{
			"goal": {Type: "string"},
			"steps": {
				Type:  "array",
				Items: &core.Schema{Type: "object"},
			},
			"files": {
				Type:  "array",
				Items: &core.Schema{Type: "string"},
			},
			"dependencies": {Type: "object"},
			"summary":      {Type: "string"},
		},
		Required: []string{"goal", "steps", "files", "dependencies"},
	}
}

func invokeStructuredReasoner(ctx context.Context, model core.LanguageModel, modelName, prompt string) (*core.CapabilityExecutionResult, error) {
	resp, err := model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       modelName,
		Temperature: 0.2,
		MaxTokens:   800,
	})
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(pattern.ExtractJSON(resp.Text)), &payload); err != nil {
		return nil, err
	}
	return &core.CapabilityExecutionResult{Success: true, Data: payload}, nil
}

func (h architectExecuteCapabilityHandler) architectConfig() *core.Config {
	if h.config == nil {
		return &core.Config{Name: "architect.execute", MaxIterations: 3, OllamaToolCalling: true}
	}
	cfg := *h.config
	cfg.Name = "architect.execute"
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 3
	}
	if !cfg.OllamaToolCalling {
		cfg.OllamaToolCalling = true
	}
	cfg.AgentSpec = core.MergeAgentSpecs(h.config.AgentSpec)
	return &cfg
}

func modelName(cfg *core.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Model
}

func stringifyContextValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []string:
		return strings.Join(typed, ", ")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func planStepsAsAny(steps []core.PlanStep) []any {
	out := make([]any, 0, len(steps))
	for _, step := range steps {
		out = append(out, map[string]any{
			"id":           step.ID,
			"description":  step.Description,
			"tool":         step.Tool,
			"params":       step.Params,
			"expected":     step.Expected,
			"verification": step.Verification,
			"status":       step.Status,
			"files":        append([]string{}, step.Files...),
		})
	}
	return out
}

func planFilesAsAny(files []string) []any {
	out := make([]any, 0, len(files))
	for _, file := range files {
		out = append(out, file)
	}
	return out
}

func planDependenciesAsAny(dependencies map[string][]string) map[string]any {
	out := make(map[string]any, len(dependencies))
	for key, values := range dependencies {
		out[key] = planFilesAsAny(values)
	}
	return out
}

func normalizePlanPayload(plan any) any {
	typed, ok := plan.(core.Plan)
	if !ok {
		return plan
	}
	return map[string]any{
		"goal":         typed.Goal,
		"steps":        planStepsAsAny(typed.Steps),
		"files":        planFilesAsAny(typed.Files),
		"dependencies": planDependenciesAsAny(typed.Dependencies),
	}
}

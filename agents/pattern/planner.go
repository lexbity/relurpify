package pattern

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"path/filepath"
	"strings"
)

// PlannerAgent builds a plan before executing. It is intentionally explicit:
// first ask the LLM for a structured plan, then execute tool-backed steps,
// finally verify + summarize. The separation mirrors how human operators would
// tackle unfamiliar tasks and serves as reference implementation for creating
// new multi-step agents.
type PlannerAgent struct {
	Model  core.LanguageModel
	Tools  *toolsys.ToolRegistry
	Memory memory.MemoryStore
	Config *core.Config
}

// Initialize configures the agent.
func (a *PlannerAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = toolsys.NewToolRegistry()
	}
	return nil
}

// Execute runs the planner workflow.
func (a *PlannerAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		graph.SetTelemetry(cfg.Telemetry)
	}
	return graph.Execute(ctx, state)
}

// Capabilities enumerates features.
func (a *PlannerAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
	}
}

// BuildGraph builds planning pipeline with explicit plan→execute→verify stages.
// Returning a Graph instead of hiding the workflow inside Execute keeps the
// system debuggable and allows other packages to analyze the structure.
func (a *PlannerAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("planner agent missing model")
	}
	planNode := &plannerPlanNode{id: "planner_plan", agent: a, task: task}
	execNode := &plannerExecuteNode{id: "planner_execute", agent: a}
	verifyNode := &plannerVerifyNode{id: "planner_verify", agent: a, task: task}
	return graph.BuildPlanExecuteVerifyGraph(planNode, execNode, verifyNode, "planner_done")
}

type plannerPlanNode struct {
	id    string
	agent *PlannerAgent
	task  *core.Task
}

// ID returns the stable graph identifier.
func (n *plannerPlanNode) ID() string { return n.id }

// Type labels the node as a system step for graph visualization.
func (n *plannerPlanNode) Type() graph.NodeType { return graph.NodeTypeSystem }

// Execute prompts the LLM for a machine-readable plan. The JSON schema is small
// enough that contributors can tweak it without retraining anything.
func (n *plannerPlanNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("planning")
	extraPrompt := ""
	if n.agent != nil && n.agent.Config != nil && n.agent.Config.AgentSpec != nil {
		extraPrompt = strings.TrimSpace(n.agent.Config.AgentSpec.Prompt)
	}
	if extraPrompt != "" {
		extraPrompt = fmt.Sprintf("Additional Guidance:\n%s\n\n", extraPrompt)
	}
	if policy := plannerSkillHints(n.agent); policy != "" {
		extraPrompt += "Skill Policy:\n" + policy + "\n\n"
	}
	prompt := fmt.Sprintf(`You are a planning agent. Break this task into steps with dependencies.
%sTask: %s
Return valid JSON Plan struct with fields goal, steps (array of {id, description, tool, params, expected, verification, files}), dependencies (map of step id -> [step id]), files.
Use string step ids (UUID-safe).
`, extraPrompt, n.task.Instruction)
	resp, err := n.agent.Model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       n.agent.Config.Model,
		Temperature: 0.2,
		MaxTokens:   800,
	})
	if err != nil {
		return nil, err
	}
	state.AddInteraction("assistant", resp.Text, map[string]interface{}{"node": n.id})
	plan, err := parsePlan(resp.Text)
	if err != nil {
		return nil, err
	}
	plan, adjustments := normalizePlannerPlan(n.agent, n.task, plan)
	state.Set("planner.plan", plan)
	if len(adjustments) > 0 {
		state.Set("planner.plan_adjustments", adjustments)
	}
	if n.agent.Memory != nil {
		_ = n.agent.Memory.Remember(ctx, NewUUID(), map[string]interface{}{
			"type":        "plan",
			"plan":        plan,
			"adjustments": adjustments,
		}, memory.MemoryScopeSession)
	}
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{
		"plan":        plan,
		"plan_steps":  plan.Steps,
		"files":       plan.Files,
		"adjustments": adjustments,
	}}, nil
}

type plannerExecuteNode struct {
	id    string
	agent *PlannerAgent
}

// ID returns the identifier seen by the framework.
func (n *plannerExecuteNode) ID() string { return n.id }

// Type signals to the graph visualizer that this step consumes tools.
func (n *plannerExecuteNode) Type() graph.NodeType { return graph.NodeTypeTool }

// Execute iterates the generated plan and calls the requested tool for each
// actionable step. Empty or unregistered tool names are skipped, which keeps
// the planner tolerant of reasoning-only or partially-grounded steps the LLM
// might propose before the step executor handles the real work.
func (n *plannerExecuteNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("executing")
	value, ok := state.Get("planner.plan")
	if !ok {
		return nil, fmt.Errorf("plan not available")
	}
	plan, _ := value.(core.Plan)
	var stepResults []map[string]interface{}
	var skippedTools []map[string]string
	for _, step := range plan.Steps {
		if step.Tool == "" {
			continue
		}
		tool, ok := n.agent.Tools.Get(step.Tool)
		if !ok {
			skippedTools = append(skippedTools, map[string]string{
				"id":     step.ID,
				"tool":   step.Tool,
				"reason": "tool not registered",
			})
			continue
		}
		result, err := tool.Execute(ctx, state, step.Params)
		if err != nil {
			return nil, err
		}
		stepResults = append(stepResults, map[string]interface{}{
			"id":     step.ID,
			"output": result.Data,
		})
		state.Set(fmt.Sprintf("planner.step.%s", step.ID), result.Data)
	}
	state.Set("planner.results", stepResults)
	if len(skippedTools) > 0 {
		state.Set("planner.skipped_tools", skippedTools)
	}
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{
		"results":       stepResults,
		"skipped_tools": skippedTools,
	}}, nil
}

type plannerVerifyNode struct {
	id    string
	agent *PlannerAgent
	task  *core.Task
}

// ID returns the verifying node identifier.
func (n *plannerVerifyNode) ID() string { return n.id }

// Type marks this node as an observation/validation phase.
func (n *plannerVerifyNode) Type() graph.NodeType { return graph.NodeTypeObservation }

// Execute packages the observed tool outputs into a short summary so downstream
// systems (CLI, LSP, tests) can display human-friendly “what just happened”
// messages without parsing the entire state map.
func (n *plannerVerifyNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("validating")
	results, _ := state.Get("planner.results")
	planVal, _ := state.Get("planner.plan")
	plan, _ := planVal.(core.Plan)
	summary := fmt.Sprintf("Executed plan for task '%s' with %d steps.", n.task.Instruction, len(plan.Steps))
	state.Set("planner.summary", summary)
	if n.agent.Memory != nil {
		_ = n.agent.Memory.Remember(ctx, NewUUID(), map[string]interface{}{
			"type":    "verification",
			"summary": summary,
			"results": results,
		}, memory.MemoryScopeSession)
	}
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]interface{}{
			"summary": summary,
		},
	}, nil
}

// parsePlan pulls the JSON payload out of the model response. The helper keeps
// PlannerAgent.Execute easy to read and doubles as a seam for unit tests.
func parsePlan(raw string) (core.Plan, error) {
	var plan core.Plan
	if err := json.Unmarshal([]byte(ExtractJSON(raw)), &plan); err != nil {
		return plan, err
	}
	if plan.Dependencies == nil {
		plan.Dependencies = make(map[string][]string)
	}
	if plan.Files == nil {
		plan.Files = make([]string, 0)
	}
	return plan, nil
}

func plannerSkillHints(agent *PlannerAgent) string {
	if agent == nil || agent.Config == nil || agent.Config.AgentSpec == nil {
		return ""
	}
	policy := toolsys.ResolveEffectiveSkillPolicy(nil, agent.Config.AgentSpec, agent.Tools).Policy
	return toolsys.RenderPlanningPolicy(policy, toolsys.PlanningRenderOptions{
		IncludePhaseTools:          true,
		IncludeVerificationSuccess: true,
	})
}

func normalizePlannerPlan(agent *PlannerAgent, task *core.Task, plan core.Plan) (core.Plan, []string) {
	if agent == nil {
		return ensurePlannerPlanDefaults(plan), nil
	}
	var fallback *core.AgentRuntimeSpec
	if agent.Config != nil {
		fallback = agent.Config.AgentSpec
	}
	effective := toolsys.ResolveEffectiveSkillPolicy(task, fallback, agent.Tools)
	if effective.Spec == nil {
		return ensurePlannerPlanDefaults(plan), nil
	}
	policy := effective.Policy
	plan = ensurePlannerPlanDefaults(plan)
	var adjustments []string
	if added := assignMissingPlanStepIDs(&plan); added > 0 {
		adjustments = append(adjustments, fmt.Sprintf("assigned ids to %d plan steps", added))
	}
	firstEdit := firstPlannerEditStepIndex(plan.Steps, policy)
	insertAt := 0
	for _, toolName := range policy.Planning.RequiredBeforeEdit {
		if toolName == "" || planHasToolBefore(plan.Steps, firstEdit, toolName) {
			continue
		}
		step, ok := synthesizedPlannerStep(agent, task, plan, toolName, "discover")
		if !ok {
			continue
		}
		plan.Steps = insertPlannerStep(plan.Steps, insertAt, step)
		insertAt++
		firstEdit++
		adjustments = append(adjustments, fmt.Sprintf("inserted required discovery step for %s", toolName))
	}
	if policy.Planning.RequireVerificationStep && !planHasVerificationStep(plan, policy) {
		toolName := plannerVerificationTool(policy)
		if step, ok := synthesizedPlannerStep(agent, task, plan, toolName, "verify"); ok {
			plan.Steps = append(plan.Steps, step)
			adjustments = append(adjustments, fmt.Sprintf("appended verification step for %s", toolName))
		}
	}
	return plan, adjustments
}

func ensurePlannerPlanDefaults(plan core.Plan) core.Plan {
	if plan.Dependencies == nil {
		plan.Dependencies = make(map[string][]string)
	}
	if plan.Files == nil {
		plan.Files = make([]string, 0)
	}
	if plan.Steps == nil {
		plan.Steps = make([]core.PlanStep, 0)
	}
	return plan
}

func assignMissingPlanStepIDs(plan *core.Plan) int {
	if plan == nil {
		return 0
	}
	used := make(map[string]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		if id := strings.TrimSpace(step.ID); id != "" {
			used[id] = struct{}{}
		}
	}
	added := 0
	for i := range plan.Steps {
		if strings.TrimSpace(plan.Steps[i].ID) != "" {
			continue
		}
		base := fmt.Sprintf("plan-step-%d", i+1)
		id := base
		suffix := 1
		for {
			if _, exists := used[id]; !exists {
				break
			}
			suffix++
			id = fmt.Sprintf("%s-%d", base, suffix)
		}
		plan.Steps[i].ID = id
		used[id] = struct{}{}
		added++
	}
	return added
}

func firstPlannerEditStepIndex(steps []core.PlanStep, policy toolsys.ResolvedSkillPolicy) int {
	for i, step := range steps {
		if plannerStepLooksLikeEdit(step, policy) {
			return i
		}
	}
	return len(steps)
}

func plannerStepLooksLikeEdit(step core.PlanStep, policy toolsys.ResolvedSkillPolicy) bool {
	if toolInSet(step.Tool, policy.Planning.PreferredEditTools) {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(step.Tool))
	if strings.Contains(name, "write") || strings.Contains(name, "edit") || strings.Contains(name, "create") || strings.Contains(name, "delete") {
		return true
	}
	desc := strings.ToLower(strings.TrimSpace(step.Description))
	return strings.Contains(desc, "edit") || strings.Contains(desc, "modify") || strings.Contains(desc, "refactor") || strings.Contains(desc, "update")
}

func planHasToolBefore(steps []core.PlanStep, limit int, toolName string) bool {
	if limit > len(steps) {
		limit = len(steps)
	}
	for i := 0; i < limit; i++ {
		if strings.EqualFold(strings.TrimSpace(steps[i].Tool), strings.TrimSpace(toolName)) {
			return true
		}
	}
	return false
}

func planHasVerificationStep(plan core.Plan, policy toolsys.ResolvedSkillPolicy) bool {
	verifyTools := make([]string, 0, len(policy.Planning.PreferredVerifyTools)+len(policy.VerificationSuccessTools))
	verifyTools = append(verifyTools, policy.Planning.PreferredVerifyTools...)
	verifyTools = append(verifyTools, policy.VerificationSuccessTools...)
	for _, step := range plan.Steps {
		if toolInSet(step.Tool, verifyTools) {
			return true
		}
		if strings.TrimSpace(step.Verification) != "" {
			return true
		}
		desc := strings.ToLower(strings.TrimSpace(step.Description))
		if strings.Contains(desc, "verify") || strings.Contains(desc, "test") || strings.Contains(desc, "build") || strings.Contains(desc, "check") {
			return true
		}
	}
	return false
}

func plannerVerificationTool(policy toolsys.ResolvedSkillPolicy) string {
	for _, toolName := range policy.Planning.PreferredVerifyTools {
		if strings.TrimSpace(toolName) != "" {
			return toolName
		}
	}
	for _, toolName := range policy.VerificationSuccessTools {
		if strings.TrimSpace(toolName) != "" {
			return toolName
		}
	}
	for _, toolName := range policy.PhaseTools["verify"] {
		if strings.TrimSpace(toolName) != "" {
			return toolName
		}
	}
	return ""
}

func synthesizedPlannerStep(agent *PlannerAgent, task *core.Task, plan core.Plan, toolName, kind string) (core.PlanStep, bool) {
	if agent == nil || agent.Tools == nil || strings.TrimSpace(toolName) == "" {
		return core.PlanStep{}, false
	}
	tool, ok := agent.Tools.Get(toolName)
	if !ok || tool == nil {
		return core.PlanStep{}, false
	}
	params, ok := plannerToolArgs(tool, task, plan)
	if !ok {
		return core.PlanStep{}, false
	}
	step := core.PlanStep{
		ID:          NewUUID(),
		Tool:        toolName,
		Params:      params,
		Description: plannerStepDescription(kind, toolName),
	}
	switch kind {
	case "verify":
		step.Verification = fmt.Sprintf("Run %s successfully", toolName)
		step.Expected = fmt.Sprintf("%s completes without errors", toolName)
	default:
		step.Expected = fmt.Sprintf("Collect context with %s before editing", toolName)
	}
	return step, true
}

func plannerStepDescription(kind, toolName string) string {
	switch kind {
	case "verify":
		return fmt.Sprintf("Verify the changes using %s", toolName)
	default:
		return fmt.Sprintf("Gather required context using %s", toolName)
	}
}

func plannerToolArgs(tool core.Tool, task *core.Task, plan core.Plan) (map[string]interface{}, bool) {
	args := map[string]interface{}{}
	required := map[string]bool{}
	for _, param := range tool.Parameters() {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			continue
		}
		required[name] = param.Required
		switch name {
		case "working_directory":
			args[name] = plannerWorkingDirectory(task, plan)
		case "path":
			if path := plannerPrimaryPath(task, plan); path != "" {
				args[name] = path
			} else {
				args[name] = "."
			}
		case "database_path":
			if db := plannerDatabasePath(task, plan); db != "" {
				args[name] = db
			}
		case "action":
			args[name] = "list_symbols"
		case "category":
			args[name] = "function"
		case "query":
			if strings.Contains(strings.ToLower(tool.Name()), "sqlite") {
				args[name] = "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name LIMIT 20;"
			}
		}
		if _, ok := args[name]; !ok && param.Default != nil {
			args[name] = param.Default
		}
	}
	for name, need := range required {
		if need {
			if _, ok := args[name]; !ok {
				return nil, false
			}
		}
	}
	if len(args) == 0 {
		return map[string]interface{}{}, true
	}
	return args, true
}

func plannerPrimaryPath(task *core.Task, plan core.Plan) string {
	for _, path := range plannerTaskPaths(task) {
		if path != "" {
			return path
		}
	}
	for _, path := range plan.Files {
		if strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path)
		}
	}
	for _, step := range plan.Steps {
		for _, path := range step.Files {
			if strings.TrimSpace(path) != "" {
				return strings.TrimSpace(path)
			}
		}
	}
	return ""
}

func plannerWorkingDirectory(task *core.Task, plan core.Plan) string {
	for _, key := range []string{"working_directory", "workdir", "directory"} {
		if task != nil && task.Context != nil {
			if value := strings.TrimSpace(fmt.Sprint(task.Context[key])); value != "" && value != "<nil>" {
				return value
			}
		}
	}
	if path := plannerPrimaryPath(task, plan); path != "" && path != "." {
		clean := filepath.Clean(path)
		if filepath.Ext(clean) != "" {
			return filepath.Dir(clean)
		}
		return clean
	}
	return "."
}

func plannerDatabasePath(task *core.Task, plan core.Plan) string {
	for _, path := range plannerTaskPaths(task) {
		if isSQLiteFailurePath(path) {
			return path
		}
	}
	for _, path := range plan.Files {
		if isSQLiteFailurePath(path) {
			return path
		}
	}
	return ""
}

func plannerTaskPaths(task *core.Task) []string {
	if task == nil {
		return nil
	}
	var paths []string
	for _, value := range task.Metadata {
		path := strings.TrimSpace(value)
		if path != "" && path != "<nil>" {
			paths = append(paths, path)
		}
	}
	if task.Context != nil {
		for _, key := range []string{"path", "file", "file_path", "target_path", "manifest_path", "database_path"} {
			if value := strings.TrimSpace(fmt.Sprint(task.Context[key])); value != "" && value != "<nil>" {
				paths = append(paths, value)
			}
		}
	}
	return paths
}

func insertPlannerStep(steps []core.PlanStep, index int, step core.PlanStep) []core.PlanStep {
	if index < 0 {
		index = 0
	}
	if index > len(steps) {
		index = len(steps)
	}
	steps = append(steps, core.PlanStep{})
	copy(steps[index+1:], steps[index:])
	steps[index] = step
	return steps
}

func toolInSet(toolName string, tools []string) bool {
	for _, candidate := range tools {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(toolName)) {
			return true
		}
	}
	return false
}

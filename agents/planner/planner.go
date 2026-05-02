package planner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pl "codeburg.org/lexbit/relurpify/agents/plan"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TaskPayload retrieves workflow retrieval payload from task context.
// This replaces the workflowutil.TaskPayload stub.
func TaskPayload(task *core.Task, key string) []byte {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context[key]
	if !ok || raw == nil {
		return nil
	}
	if bytes, ok := raw.([]byte); ok {
		return bytes
	}
	// Try to marshal if it's not already bytes
	if data, err := json.Marshal(raw); err == nil {
		return data
	}
	return nil
}

// PlannerAgent builds a plan before executing. It is intentionally explicit:
// first ask the LLM for a structured plan, then execute tool-backed steps,
// finally verify + summarize. The separation mirrors how human operators would
// tackle unfamiliar tasks and serves as reference implementation for creating
// new multi-step agents.
type PlannerAgent struct {
	Model           contracts.LanguageModel
	Tools           *capability.Registry
	Memory          *memory.WorkingMemoryStore
	Config          *core.Config
	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int
}

// Initialize configures the agent.
func (a *PlannerAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	return nil
}

// Execute runs the planner workflow.
func (a *PlannerAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		graph.SetTelemetry(cfg.Telemetry)
	}
	result, err := graph.Execute(ctx, env)
	preservePlannerExecutionResult(env, result)
	mirrorPlannerSummaryReference(env)
	mirrorPlannerCheckpointReference(env)
	compactPlannerResultsStateInContext(env)
	return result, err
}

func envGetString(env *contextdata.Envelope, key string) string {
	val, _ := env.GetWorkingValue(key)
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

func preservePlannerExecutionResult(env *contextdata.Envelope, result *core.Result) {
	if env == nil || result == nil {
		return
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	if raw, ok := env.GetWorkingValue("planner.results"); ok {
		result.Data["results"] = raw
	}
	if raw, ok := env.GetWorkingValue("planner.skipped_tools"); ok {
		result.Data["skipped_tools"] = raw
	}
	if summary := strings.TrimSpace(envGetString(env, "planner.summary")); summary != "" {
		result.Data["summary"] = summary
	}
}

func mirrorPlannerSummaryReference(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if strings.TrimSpace(envGetString(env, "planner.summary")) == "" {
		return
	}
	if rawRef, ok := env.GetWorkingValue("graph.summary_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			env.SetWorkingValue("planner.summary_ref", ref, contextdata.MemoryClassTask)
		}
	}
	if summary := strings.TrimSpace(envGetString(env, "graph.summary")); summary != "" {
		env.SetWorkingValue("planner.summary_artifact_summary", summary, contextdata.MemoryClassTask)
	}
}

func mirrorPlannerCheckpointReference(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if rawRef, ok := env.GetWorkingValue("graph.checkpoint_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			env.SetWorkingValue("planner.checkpoint_ref", ref, contextdata.MemoryClassTask)
		}
	}
}

func compactPlannerResultsStateInContext(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("planner.summary_ref"); !ok {
		return
	}
	raw, ok := env.GetWorkingValue("planner.results")
	if !ok {
		return
	}
	if compact := compactPlannerResultsState(raw); compact != nil {
		env.SetWorkingValue("planner.results", compact, contextdata.MemoryClassTask)
	}
	if rawSkipped, ok := env.GetWorkingValue("planner.skipped_tools"); ok {
		if compact := compactPlannerSkippedToolsState(rawSkipped); compact != nil {
			env.SetWorkingValue("planner.skipped_tools", compact, contextdata.MemoryClassTask)
		}
	}
}

func compactPlannerResultsState(raw any) map[string]any {
	results, ok := raw.([]map[string]interface{})
	if !ok {
		return nil
	}
	value := map[string]any{
		"result_count": len(results),
	}
	if len(results) == 0 {
		return value
	}
	steps := make([]map[string]any, 0, len(results))
	for _, result := range results {
		step := map[string]any{
			"id": result["id"],
		}
		if output, ok := result["output"]; ok && output != nil {
			step["has_output"] = true
		}
		steps = append(steps, step)
	}
	value["steps"] = steps
	value["last_step"] = steps[len(steps)-1]
	return value
}

func compactPlannerSkippedToolsState(raw any) map[string]any {
	skipped, ok := raw.([]map[string]string)
	if !ok {
		return nil
	}
	value := map[string]any{
		"count": len(skipped),
	}
	if len(skipped) == 0 {
		return value
	}
	last := skipped[len(skipped)-1]
	value["last"] = map[string]any{
		"id":     last["id"],
		"tool":   last["tool"],
		"reason": last["reason"],
	}
	return value
}

// Capabilities enumerates features.
func (a *PlannerAgent) Capabilities() []string {
	return []string{"planner"}
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
	streamNode := a.streamTriggerNode(task)
	done := graph.NewTerminalNode("planner_done")
	g := graph.NewGraph()
	if a.Tools != nil {
		catalog := a.Tools.CaptureExecutionCatalogSnapshot()
		if catalog != nil && len(catalog.InspectableCapabilities()) > 0 {
			g.SetCapabilityCatalog(catalog)
		}
	}
	nodes := make([]graph.Node, 0, 5)
	if streamNode != nil {
		nodes = append(nodes, streamNode)
	}
	nodes = append(nodes, planNode, execNode, verifyNode, done)
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	startID := planNode.ID()
	if streamNode != nil {
		startID = streamNode.ID()
	}
	if err := g.SetStart(startID); err != nil {
		return nil, err
	}
	if streamNode != nil {
		if err := g.AddEdge(streamNode.ID(), planNode.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(planNode.ID(), execNode.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(execNode.ID(), verifyNode.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(verifyNode.ID(), done.ID(), nil, false); err != nil {
		return nil, err
	}
	return g, nil
}

func (a *PlannerAgent) streamTriggerNode(task *core.Task) graph.Node {
	if a == nil {
		return nil
	}
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	node := graph.NewContextStreamNode("planner_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	node.BudgetShortfallPolicy = "emit_partial"
	node.Metadata = map[string]any{
		"agent": "planner",
		"stage": "pre_plan",
	}
	return node
}

func (a *PlannerAgent) streamQuery(task *core.Task) string {
	if a == nil {
		return ""
	}
	if query := strings.TrimSpace(a.StreamQuery); query != "" {
		return query
	}
	query := strings.TrimSpace(taskInstructionText(task))
	if query == "" {
		return "planner context"
	}
	return "planning context: " + query
}

func (a *PlannerAgent) streamMode() contextstream.Mode {
	if a == nil || a.StreamMode == "" {
		return contextstream.ModeBlocking
	}
	return a.StreamMode
}

func (a *PlannerAgent) streamMaxTokens() int {
	if a == nil || a.StreamMaxTokens <= 0 {
		return 512
	}
	return a.StreamMaxTokens
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
func (n *plannerPlanNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	env.SetWorkingValue("execution_phase", "planning", contextdata.MemoryClassTask)
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
	if trimmed, _ := env.GetWorkingValue("contextstream.trimmed"); trimmed == true {
		shortfall := 0
		if raw, ok := env.GetWorkingValue("contextstream.shortfall_tokens"); ok {
			if value, ok := raw.(int); ok {
				shortfall = value
			}
		}
		extraPrompt += fmt.Sprintf("Streaming note: context was trimmed to fit budget (shortfall=%d tokens).\n\n", shortfall)
	}
	if n.task != nil && n.task.Context != nil {
		if payload := TaskPayload(n.task, "workflow_retrieval"); len(payload) > 0 {
			var data map[string]any
			if err := json.Unmarshal(payload, &data); err == nil {
				if formatted := formatPlannerWorkflowRetrieval(data); formatted != "" {
					extraPrompt += "Workflow Retrieval:\n" + formatted + "\n\n"
				}
			}
		} else if raw, ok := n.task.Context["workflow_retrieval"]; ok && raw != nil {
			encoded, err := json.MarshalIndent(raw, "", "  ")
			if err == nil {
				extraPrompt += "Workflow Retrieval:\n" + string(encoded) + "\n\n"
			}
		}
	}
	if streamed := formatPlannerStreamedContext(env); streamed != "" {
		extraPrompt += "Streamed Context:\n" + streamed + "\n\n"
	}
	prompt := fmt.Sprintf(`You are a planning agent. Break this task into steps with dependencies.
%sTask: %s
Return valid JSON Plan struct with fields goal, steps (array of {id, description, tool, params, expected, verification, files}), dependencies (map of step id -> [step id]), files.
Use string step ids (UUID-safe).
`, extraPrompt, n.task.Instruction)
	resp, err := n.agent.Model.Generate(ctx, prompt, &contracts.LLMOptions{
		Model:       n.agent.Config.Model,
		Temperature: 0.2,
		MaxTokens:   800,
	})
	if err != nil {
		return nil, err
	}
	env.AddInteraction(map[string]interface{}{
		"role":    "assistant",
		"content": resp.Text,
		"node":    n.id,
	})
	plan, err := parsePlan(resp.Text)
	if err != nil {
		return nil, err
	}
	plan, adjustments := normalizePlannerPlan(n.agent, n.task, plan)
	env.SetWorkingValue("planner.plan", plan, contextdata.MemoryClassTask)
	if len(adjustments) > 0 {
		env.SetWorkingValue("planner.plan_adjustments", adjustments, contextdata.MemoryClassTask)
	}
	if n.agent.Memory != nil {
		scope := n.agent.Memory.Scope(plannerUUID())
		scope.Set("planner.plan", plan, core.MemoryClassWorking)
		scope.Set("planner.plan_adjustments", adjustments, core.MemoryClassWorking)
	}
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{
		"plan":        plan,
		"plan_steps":  plan.Steps,
		"files":       plan.Files,
		"adjustments": adjustments,
	}}, nil
}

func formatPlannerWorkflowRetrieval(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	var sections []string
	if query := strings.TrimSpace(fmt.Sprint(payload["query"])); query != "" && query != "<nil>" {
		sections = append(sections, "Query: "+query)
	}
	if scope := strings.TrimSpace(fmt.Sprint(payload["scope"])); scope != "" && scope != "<nil>" {
		sections = append(sections, "Scope: "+scope)
	}
	if cacheTier := strings.TrimSpace(fmt.Sprint(payload["cache_tier"])); cacheTier != "" && cacheTier != "<nil>" {
		sections = append(sections, "Cache tier: "+cacheTier)
	}
	results, ok := payload["results"].([]map[string]any)
	if !ok || len(results) == 0 {
		return strings.Join(sections, "\n")
	}
	lines := make([]string, 0, len(results))
	for i, result := range results {
		text := strings.TrimSpace(fmt.Sprint(result["text"]))
		if text == "" || text == "<nil>" {
			text = strings.TrimSpace(fmt.Sprint(result["summary"]))
		}
		if text == "" || text == "<nil>" {
			text = "reference only"
		}
		line := fmt.Sprintf("%d. %s", i+1, truncatePlannerPromptText(text, 240))
		if ref := plannerWorkflowReference(result); ref != "" {
			line += "\n   Reference: " + ref
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		sections = append(sections, "Evidence:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n")
}

func formatPlannerStreamedContext(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	streamed := env.ReferencesSnapshot().StreamedContext
	if len(streamed) == 0 {
		return ""
	}
	lines := make([]string, 0, len(streamed))
	for _, ref := range streamed {
		chunkID := strings.TrimSpace(string(ref.ChunkID))
		if chunkID == "" {
			continue
		}
		line := "- " + chunkID
		if ref.Source != "" {
			line += " [" + strings.TrimSpace(ref.Source) + "]"
		}
		if ref.Rank > 0 {
			line += fmt.Sprintf(" rank=%d", ref.Rank)
		}
		if ref.IsSummary {
			line += " summary"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func plannerWorkflowReference(result map[string]any) string {
	raw, ok := result["reference"].(map[string]any)
	if !ok || len(raw) == 0 {
		return ""
	}
	for _, key := range []string{"uri", "id", "detail"} {
		value := strings.TrimSpace(fmt.Sprint(raw[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func truncatePlannerPromptText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

type plannerExecuteNode struct {
	id    string
	agent *PlannerAgent
}

// ID returns the identifier seen by the framework.
func (n *plannerExecuteNode) ID() string { return n.id }

// Type signals to the graph visualizer that this step consumes tools.
func (n *plannerExecuteNode) Type() graph.NodeType { return graph.NodeTypeTool }

// Contract marks the planner executor as a capability-consuming tool stage.
func (n *plannerExecuteNode) Contract() graph.NodeContract {
	return graph.NodeContract{
		RequiredCapabilities: []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
		}},
		SideEffectClass: graph.SideEffectExternal,
		Idempotency:     graph.IdempotencyUnknown,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "planner.plan", "planner.*"},
			WriteKeys:                []string{"planner.results", "planner.step.*", "planner.skipped_tools"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStepMetadata, core.StateDataClassArtifactRef, core.StateDataClassMemoryRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
			PreferArtifactReferences: true,
		},
	}
}

// Execute iterates the generated plan and calls the requested tool for each
// actionable step. Empty or unregistered tool names are skipped, which keeps
// the planner tolerant of reasoning-only or partially-grounded steps the LLM
// might propose before the step executor handles the real work.
func (n *plannerExecuteNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	env.SetWorkingValue("execution_phase", "executing", contextdata.MemoryClassTask)
	value, ok := env.GetWorkingValue("planner.plan")
	if !ok {
		return nil, fmt.Errorf("plan not available")
	}
	plan, _ := value.(pl.Plan)
	var stepResults []map[string]interface{}
	var skippedTools []map[string]string
	for _, step := range plan.Steps {
		step.Params = resolvePlannerStepParams(env, step.Params)
		step, _, _ = repairPlannerStep(n.agent.Tools, step)
		if step.Tool == "" {
			continue
		}
		if !n.agent.Tools.HasCapability(step.Tool) {
			skippedTools = append(skippedTools, map[string]string{
				"id":     step.ID,
				"tool":   step.Tool,
				"reason": "capability not registered",
			})
			continue
		}
		// TODO: envelope equivalent of CapabilityAvailable
		// if !n.agent.Tools.CapabilityAvailable(ctx, env, step.Tool) {
		if false {
			skippedTools = append(skippedTools, map[string]string{
				"id":     step.ID,
				"tool":   step.Tool,
				"reason": "capability unavailable",
			})
			continue
		}
		params := normalizePlannerStepParams(n.agent.Tools, step.Tool, step.Params)
		// TODO: envelope equivalent of InvokeCapability
		// result, err := n.agent.Tools.InvokeCapability(ctx, env, step.Tool, params)
		result, err := n.agent.Tools.InvokeCapability(ctx, nil, step.Tool, params)
		if err != nil {
			return nil, err
		}
		stepResults = append(stepResults, map[string]interface{}{
			"id":     step.ID,
			"output": result.Data,
		})
		env.SetWorkingValue(fmt.Sprintf("planner.step.%s", step.ID), result.Data, contextdata.MemoryClassTask)
	}
	env.SetWorkingValue("planner.results", stepResults, contextdata.MemoryClassTask)
	if len(skippedTools) > 0 {
		env.SetWorkingValue("planner.skipped_tools", skippedTools, contextdata.MemoryClassTask)
	}
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{
		"results":       stepResults,
		"skipped_tools": skippedTools,
	}}, nil
}

func normalizePlannerStepParams(registry *capability.Registry, toolName string, params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return params
	}
	tool, ok := registry.Get(toolName)
	if !ok || tool == nil {
		return params
	}
	normalized := make(map[string]interface{}, len(params))
	for key, value := range params {
		normalized[key] = value
	}
	for _, param := range tool.Parameters() {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			continue
		}
		if value, ok := normalized[name]; ok {
			normalized[name] = normalizePlannerParamValue(name, name, value)
			continue
		}
		for _, alias := range plannerParamAliases(name) {
			if value, ok := normalized[alias]; ok {
				normalized[name] = normalizePlannerParamValue(name, alias, value)
				break
			}
		}
	}
	return normalized
}

func normalizePlannerParamValue(name, alias string, value interface{}) interface{} {
	switch name {
	case "path":
		if path := plannerFirstStepPath(value); path != "" {
			return path
		}
	case "directory":
		if path := plannerFirstStepPath(value); path != "" {
			return path
		}
	}
	return value
}

func plannerParamAliases(name string) []string {
	switch name {
	case "path":
		return []string{"file", "file_path", "target_path", "manifest_path", "database_path", "files", "paths"}
	case "directory":
		return []string{"path", "dir", "working_directory", "workdir", "cwd"}
	case "working_directory":
		return []string{"workdir", "directory", "cwd"}
	default:
		return nil
	}
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
// systems (CLI, LSP, tests) can display human-friendly "what just happened"
// messages without parsing the entire state map.
func (n *plannerVerifyNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	env.SetWorkingValue("execution_phase", "validating", contextdata.MemoryClassTask)
	results, _ := env.GetWorkingValue("planner.results")
	_ = results
	planVal, _ := env.GetWorkingValue("planner.plan")
	plan, _ := planVal.(pl.Plan)
	summary := fmt.Sprintf("Executed plan for task '%s' with %d steps.", n.task.Instruction, len(plan.Steps))
	env.SetWorkingValue("planner.summary", summary, contextdata.MemoryClassTask)
	if n.agent.Memory != nil {
		n.agent.Memory.Scope(plannerUUID()).Set("planner.summary", summary, core.MemoryClassWorking)
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
func parsePlan(raw string) (pl.Plan, error) {
	var plan pl.Plan
	if err := json.Unmarshal([]byte(plannerExtractJSON(raw)), &plan); err != nil {
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
	policy := frameworkskills.ResolveEffectiveSkillPolicy(nil, agent.Config.AgentSpec, agent.Tools).Policy
	return frameworkskills.RenderPlanningPolicy(policy, frameworkskills.PlanningRenderOptions{
		IncludePhaseCapabilities:   true,
		IncludeVerificationSuccess: true,
	})
}

func plannerWorkflowID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func plannerRunID(task *core.Task) string {
	return ""
}

func plannerUsesExplicitCheckpointNodes(cfg *core.Config) bool {
	_ = cfg
	return true
}

func plannerUsesStructuredPersistence(cfg *core.Config) bool {
	_ = cfg
	return true
}

func normalizePlannerPlan(agent *PlannerAgent, task *core.Task, plan pl.Plan) (pl.Plan, []string) {
	if agent == nil {
		return ensurePlannerPlanDefaults(plan), nil
	}
	plan = ensurePlannerPlanDefaults(plan)
	var adjustments []string
	if added := assignMissingPlanStepIDs(&plan); added > 0 {
		adjustments = append(adjustments, fmt.Sprintf("assigned ids to %d plan steps", added))
	}
	repairPlannerSteps(agent.Tools, &plan, &adjustments)
	var fallback *agentspec.AgentRuntimeSpec
	if agent.Config != nil {
		fallback = agent.Config.AgentSpec
	}
	effective := frameworkskills.ResolveEffectiveSkillPolicy(task, fallback, agent.Tools)
	if effective.Spec == nil {
		return plan, adjustments
	}
	policy := effective.Policy
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

func ensurePlannerPlanDefaults(plan pl.Plan) pl.Plan {
	if plan.Dependencies == nil {
		plan.Dependencies = make(map[string][]string)
	}
	if plan.Files == nil {
		plan.Files = make([]string, 0)
	}
	if plan.Steps == nil {
		plan.Steps = make([]pl.PlanStep, 0)
	}
	return plan
}

func assignMissingPlanStepIDs(plan *pl.Plan) int {
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

func firstPlannerEditStepIndex(steps []pl.PlanStep, policy frameworkskills.ResolvedSkillPolicy) int {
	for i, step := range steps {
		if plannerStepLooksLikeEdit(step, policy) {
			return i
		}
	}
	return len(steps)
}

func plannerStepLooksLikeEdit(step pl.PlanStep, policy frameworkskills.ResolvedSkillPolicy) bool {
	if toolInSet(step.Tool, policy.Planning.PreferredEditCapabilities) {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(step.Tool))
	if strings.Contains(name, "write") || strings.Contains(name, "edit") || strings.Contains(name, "create") || strings.Contains(name, "delete") {
		return true
	}
	desc := strings.ToLower(strings.TrimSpace(step.Description))
	return strings.Contains(desc, "edit") || strings.Contains(desc, "modify") || strings.Contains(desc, "refactor") || strings.Contains(desc, "update")
}

func planHasToolBefore(steps []pl.PlanStep, limit int, toolName string) bool {
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

func planHasVerificationStep(plan pl.Plan, policy frameworkskills.ResolvedSkillPolicy) bool {
	verifyTools := make([]string, 0, len(policy.Planning.PreferredVerifyCapabilities)+len(policy.VerificationSuccessCapabilities))
	verifyTools = append(verifyTools, policy.Planning.PreferredVerifyCapabilities...)
	verifyTools = append(verifyTools, policy.VerificationSuccessCapabilities...)
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

func plannerVerificationTool(policy frameworkskills.ResolvedSkillPolicy) string {
	for _, toolName := range policy.Planning.PreferredVerifyCapabilities {
		if strings.TrimSpace(toolName) != "" {
			return toolName
		}
	}
	for _, toolName := range policy.VerificationSuccessCapabilities {
		if strings.TrimSpace(toolName) != "" {
			return toolName
		}
	}
	for _, toolName := range policy.PhaseCapabilities["verify"] {
		if strings.TrimSpace(toolName) != "" {
			return toolName
		}
	}
	return ""
}

func synthesizedPlannerStep(agent *PlannerAgent, task *core.Task, plan pl.Plan, toolName, kind string) (pl.PlanStep, bool) {
	if agent == nil || agent.Tools == nil || strings.TrimSpace(toolName) == "" {
		return pl.PlanStep{}, false
	}
	tool, ok := agent.Tools.Get(toolName)
	if !ok || tool == nil {
		return pl.PlanStep{}, false
	}
	params, ok := plannerToolArgs(tool, task, plan)
	if !ok {
		return pl.PlanStep{}, false
	}
	step := pl.PlanStep{
		ID:          plannerUUID(),
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

func plannerToolArgs(tool contracts.Tool, task *core.Task, plan pl.Plan) (map[string]interface{}, bool) {
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

// PlannerSkillHints exposes the effective planning guidance for external
// callers without requiring them to duplicate planner internals.
func PlannerSkillHints(agent *PlannerAgent) string {
	return plannerSkillHints(agent)
}

func telemetryForConfig(cfg *core.Config) core.Telemetry {
	if cfg == nil {
		return nil
	}
	return cfg.Telemetry
}

func taskID(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.ID)
}

func taskInstructionText(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Instruction)
}

func plannerUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func plannerExtractJSON(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end >= start {
		return raw[start : end+1]
	}
	return "{}"
}

func isSQLiteFailurePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite3")
}

func plannerPrimaryPath(task *core.Task, plan pl.Plan) string {
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

func plannerWorkingDirectory(task *core.Task, plan pl.Plan) string {
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

func plannerDatabasePath(task *core.Task, plan pl.Plan) string {
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
		path := strings.TrimSpace(fmt.Sprint(value))
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

func insertPlannerStep(steps []pl.PlanStep, index int, step pl.PlanStep) []pl.PlanStep {
	if index < 0 {
		index = 0
	}
	if index > len(steps) {
		index = len(steps)
	}
	steps = append(steps, pl.PlanStep{})
	copy(steps[index+1:], steps[index:])
	steps[index] = step
	return steps
}

func repairPlannerSteps(registry *capability.Registry, plan *pl.Plan, adjustments *[]string) {
	if registry == nil || plan == nil {
		return
	}
	for i := range plan.Steps {
		repaired, changed, note := repairPlannerStep(registry, plan.Steps[i])
		if !changed {
			continue
		}
		plan.Steps[i] = repaired
		if adjustments != nil && strings.TrimSpace(note) != "" {
			*adjustments = append(*adjustments, note)
		}
	}
}

func repairPlannerStep(registry *capability.Registry, step pl.PlanStep) (pl.PlanStep, bool, string) {
	switch strings.TrimSpace(step.Tool) {
	case "file_search":
		if _, hasPattern := step.Params["pattern"]; hasPattern {
			return step, false, ""
		}
		if path := plannerStepParamString(step.Params, "path", "file", "file_path", "target_path", "manifest_path"); path != "" && registry.HasCapability("file_read") {
			step.Tool = "file_read"
			step.Params = map[string]interface{}{"path": path}
			return step, true, fmt.Sprintf("rewrote step %s from file_search to file_read using path", plannerStepID(step))
		}
		if dir := plannerStepParamString(step.Params, "directory", "path", "dir", "working_directory", "workdir", "cwd"); dir != "" && registry.HasCapability("file_list") {
			step.Tool = "file_list"
			step.Params = map[string]interface{}{"directory": dir}
			return step, true, fmt.Sprintf("rewrote step %s from file_search to file_list using directory", plannerStepID(step))
		}
	case "code_analysis":
		if path := plannerStepParamString(step.Params, "path", "file", "file_path", "target_path"); path != "" && registry.HasCapability("file_read") {
			step.Tool = "file_read"
			step.Params = map[string]interface{}{"path": path}
			return step, true, fmt.Sprintf("rewrote step %s from code_analysis to file_read using path", plannerStepID(step))
		}
		if path := plannerFirstStepPath(step.Params["files"]); path != "" && registry.HasCapability("file_read") {
			step.Tool = "file_read"
			step.Params = map[string]interface{}{"path": path}
			return step, true, fmt.Sprintf("rewrote step %s from code_analysis to file_read using files", plannerStepID(step))
		}
	case "file_read":
		if _, ok := step.Params["path"]; ok {
			return step, false, ""
		}
		if path := plannerStepParamString(step.Params, "file", "file_path", "target_path", "manifest_path"); path != "" {
			if step.Params == nil {
				step.Params = map[string]interface{}{}
			}
			step.Params["path"] = path
			return step, true, fmt.Sprintf("normalized step %s file_read path alias", plannerStepID(step))
		}
		if path := plannerFirstStepPath(step.Params["files"]); path != "" {
			if step.Params == nil {
				step.Params = map[string]interface{}{}
			}
			step.Params["path"] = path
			return step, true, fmt.Sprintf("normalized step %s file_read files -> path", plannerStepID(step))
		}
	}
	return step, false, ""
}

func plannerStepParamString(params map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(params[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func plannerFirstStepPath(raw interface{}) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		if len(typed) > 0 {
			return strings.TrimSpace(typed[0])
		}
	case []interface{}:
		if len(typed) > 0 {
			return plannerFirstStepPath(typed[0])
		}
	case map[string]any:
		if files, ok := typed["files"]; ok {
			return plannerFirstStepPath(files)
		}
		if path, ok := typed["path"]; ok {
			return plannerFirstStepPath(path)
		}
	}
	return ""
}

func resolvePlannerStepParams(env *contextdata.Envelope, params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return params
	}
	resolved := make(map[string]interface{}, len(params))
	for key, value := range params {
		resolved[key] = resolvePlannerParamValue(env, value)
	}
	return resolved
}

func resolvePlannerParamValue(env *contextdata.Envelope, value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		return resolvePlannerParamTemplate(env, typed)
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, resolvePlannerParamValue(env, item))
		}
		return compactPlannerResolvedValue(out)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = resolvePlannerParamValue(env, item)
		}
		return out
	default:
		return value
	}
}

func compactPlannerResolvedValue(value interface{}) interface{} {
	items, ok := value.([]interface{})
	if !ok {
		return value
	}
	if len(items) == 1 {
		switch nested := items[0].(type) {
		case []interface{}:
			return compactPlannerResolvedValue(nested)
		case []string:
			out := make([]interface{}, 0, len(nested))
			for _, item := range nested {
				out = append(out, item)
			}
			return compactPlannerResolvedValue(out)
		}
	}
	return items
}

func resolvePlannerParamTemplate(env *contextdata.Envelope, raw string) interface{} {
	text := strings.TrimSpace(raw)
	if env == nil || text == "" {
		return raw
	}
	if strings.HasPrefix(text, "${") && strings.HasSuffix(text, "}") {
		if value, ok := resolvePlannerOutputReference(env, strings.TrimSuffix(strings.TrimPrefix(text, "${"), "}")); ok {
			return value
		}
	}
	if strings.HasPrefix(text, "{{") && strings.HasSuffix(text, "}}") {
		if value, ok := resolvePlannerOutputReference(env, strings.TrimSuffix(strings.TrimPrefix(text, "{{"), "}}")); ok {
			return value
		}
	}
	return raw
}

func resolvePlannerOutputReference(env *contextdata.Envelope, ref string) (interface{}, bool) {
	if env == nil {
		return nil, false
	}
	ref = strings.TrimSpace(ref)
	parts := strings.Split(ref, ".")
	if len(parts) < 2 {
		return nil, false
	}
	stepID := strings.TrimSpace(parts[0])
	if stepID == "" {
		return nil, false
	}
	value, ok := env.GetWorkingValue("planner.step." + stepID)
	if !ok {
		return nil, false
	}
	if len(parts) == 2 && parts[1] == "output" {
		return value, true
	}
	current := value
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" || part == "output" {
			continue
		}
		typed, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = typed[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func plannerStepID(step pl.PlanStep) string {
	if id := strings.TrimSpace(step.ID); id != "" {
		return id
	}
	return "<unknown>"
}

func toolInSet(toolName string, tools []string) bool {
	for _, candidate := range tools {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(toolName)) {
			return true
		}
	}
	return false
}

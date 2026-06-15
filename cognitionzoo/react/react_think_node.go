package react

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/model"
)

type reactThinkNode struct {
	id    string
	agent *ReActAgent
	task  *execution.Task
}

// ID returns the think node identifier.
func (n *reactThinkNode) ID() string { return n.id }

// Type marks the think step as an observation node.
func (n *reactThinkNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeObservation }

// Execute drives the "think" portion of the ReAct loop and either emits a tool
// call or final answer instructions.
func (n *reactThinkNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	env.SetWorkingValueWithClass("react.execution_phase", "planning", contextdata.MemoryClassTask)
	n.agent.enforceBudget(env)
	n.agent.manageContextSignals(env)
	if summary := strings.TrimSpace(envGetString(env, "react.verification_latched_summary")); summary != "" {
		decision := decisionPayload{
			Thought:   "verification already succeeded",
			Complete:  true,
			Summary:   summary,
			Timestamp: time.Now().UTC(),
		}
		env.SetWorkingValueWithClass("react.tool_calls", []model.ToolCall{}, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass("react.decision", decision, contextdata.MemoryClassTask)
		return &execution.Result{
			NodeID:  n.id,
			Success: true,
			Data: execution.NewToolResultPayload(map[string]any{
				"decision": decision,
			}),
		}, nil
	}
	var resp *model.LLMResponse
	var err error
	tools := n.agent.availableToolsForPhase(ctx, env, n.task)
	recordActiveToolNames(env, tools)
	useToolCalling := len(tools) > 0
	streamCB := n.streamCallback()
	if useToolCalling {
		messages := n.ensureMessages(env, tools)
		resp, err = n.agent.Model.ChatWithTools(ctx, messages, ports.LLMToolSpecsFromTools(tools), &model.LLMOptions{
			Model:          n.agent.Config.Model,
			Temperature:    0.1,
			MaxTokens:      512,
			StreamCallback: streamCB,
		})
		if err == nil {
			saveReactMessages(env, messages)
		}
	} else {
		prompt := n.resolvePrompt(env, tools)
		resp, err = n.agent.Model.Generate(ctx, prompt, &model.LLMOptions{
			Model:          n.agent.Config.Model,
			Temperature:    0.1,
			MaxTokens:      512,
			StreamCallback: streamCB,
		})
	}
	if err != nil {
		return nil, err
	}
	if useToolCalling {
		appendAssistantMessage(env, resp)
	}
	env.AddInteraction(map[string]any{
		"role":    "assistant",
		"content": resp.Text,
		"node":    n.id,
	})
	n.agent.recordLatestInteraction(env)
	decision, toolCalls, err := n.normalizeDecision(ctx, env, resp, useToolCalling, tools)
	if err != nil {
		return nil, err
	}
	env.SetWorkingValueWithClass("react.tool_calls", toolCalls, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("react.decision", decision, contextdata.MemoryClassTask)
	n.agent.debugf("%s decision=%+v tool_calls=%d", n.id, decision, len(resp.ToolCalls))
	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"decision": decision,
		}),
	}, nil
}

func (n *reactThinkNode) normalizeDecision(ctx context.Context, env *contextdata.Envelope, resp *model.LLMResponse, useToolCalling bool, tools []ports.Tool) (decisionPayload, []model.ToolCall, error) {
	if resp == nil {
		return decisionPayload{}, nil, fmt.Errorf("empty llm response")
	}
	// Apply MaxToolsPerCall limit if model supports ProfiledModel
	maxTools := 0
	if pm, ok := n.agent.Model.(model.ProfiledModel); ok {
		maxTools = pm.MaxToolsPerCall()
	}
	var toolCalls []model.ToolCall
	if len(resp.ToolCalls) > 0 {
		toolCalls = filterToolCalls(resp.ToolCalls)
		if maxTools > 0 && len(toolCalls) > maxTools {
			toolCalls = toolCalls[:maxTools]
		}
		if len(toolCalls) > 0 {
			call := toolCalls[0]
			return decisionPayload{
				Thought:   trimToBudget(resp.Text, 220),
				Tool:      call.Name,
				Arguments: call.Args,
				Complete:  false,
				Timestamp: time.Now().UTC(),
			}, toolCalls, nil
		}
	}
	parsed, err := parseDecision(resp.Text)
	if err == nil && (parsed.Tool != "" || parsed.Complete) {
		return parsed, nil, nil
	}
	// Check repair strategy
	repairStrategy := "heuristic-only"
	if pm, ok := n.agent.Model.(model.ProfiledModel); ok {
		repairStrategy = pm.ToolRepairStrategy()
	}
	var repaired string
	var repairErr error
	if repairStrategy == "llm" {
		repaired, repairErr = n.repairDecision(ctx, tools, resp.Text, useToolCalling)
	}
	if repairErr != nil || repairStrategy != "llm" {
		return decisionPayload{Thought: trimToBudget(resp.Text, 220), Complete: true, Timestamp: time.Now().UTC()}, nil, nil
	}
	parsed, err = parseDecision(repaired)
	if err != nil {
		return decisionPayload{Thought: trimToBudget(resp.Text, 220), Complete: true, Timestamp: time.Now().UTC()}, nil, nil
	}
	if parsed.Tool != "" {
		return parsed, []model.ToolCall{{Name: parsed.Tool, Args: parsed.Arguments}}, nil
	}
	return parsed, nil, nil
}

func (n *reactThinkNode) repairDecision(ctx context.Context, tools []ports.Tool, raw string, useToolCalling bool) (string, error) {
	schema := `Return ONLY valid JSON:
{"thought":"short reasoning","action":"tool|complete","tool":"tool name or empty","arguments":{},"complete":true|false,"summary":"final answer when complete"}`
	prompt := fmt.Sprintf("%s\nAllowed tools: %s\nOriginal response:\n%s", schema, strings.Join(toolNames(tools), ", "), raw)
	resp, err := n.agent.Model.Generate(ctx, prompt, &model.LLMOptions{
		Model:       n.agent.Config.Model,
		Temperature: 0,
		MaxTokens:   256,
	})
	if err != nil {
		return "", err
	}
	_ = useToolCalling
	return resp.Text, nil
}

func filterToolCalls(calls []model.ToolCall) []model.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]model.ToolCall, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" || strings.EqualFold(name, "none") {
			continue
		}
		call.Name = name
		out = append(out, call)
	}
	return out
}

// streamCallback extracts a stream callback from the task context, if present.
func (n *reactThinkNode) streamCallback() func(string) {
	if n.task == nil || n.task.Context == nil {
		return nil
	}
	if cb, ok := n.task.Context["stream_callback"].(func(string)); ok {
		return cb
	}
	return nil
}

// buildVariables seeds runtime variables from the task.
func buildVariables(task *execution.Task) map[string]string {
	vars := make(map[string]string)
	if task != nil {
		vars["instruction"] = task.Instruction
	}
	return vars
}

// buildState seeds state values evaluated by when-expressions.
func buildState(env *contextdata.Envelope, task *execution.Task) map[string]any {
	state := make(map[string]any)

	// React phase state
	if phase := envGetString(env, "react.phase"); phase != "" {
		state["react.phase"] = phase
	}

	// Plan existence state
	if _, ok := envelopeGet(env, "architect.plan"); ok {
		state["architect.plan_exists"] = true
	}
	if _, ok := envelopeGet(env, "planner.plan"); ok {
		state["planner.plan_exists"] = true
	}

	// Current step state
	if task != nil && task.Context != nil {
		if _, ok := task.Context["current_step"]; ok {
			state["react.current_step_exists"] = true
		}
	}

	// Tool observations state
	if _, ok := envelopeGet(env, "react.tool_observations"); ok {
		state["react.has_observations"] = true
	}

	return state
}

// buildRuntimeContext assembles a prompt.RuntimeContext from the agent and task.
func (n *reactThinkNode) buildRuntimeContext(env *contextdata.Envelope, tools []ports.Tool) prompt.RuntimeContext {
	caps := n.agent.Tools.AllCapabilities()
	var agentSpec *agentspec.AgentRuntimeSpec
	if n.agent != nil && n.agent.Config != nil {
		agentSpec = n.agent.Config.AgentSpec
	}
	variables := buildVariables(n.task)
	state := buildState(env, n.task)

	consumerID := "react"
	if n.agent.Config != nil {
		consumerID = n.agent.Config.Name
	}

	return prompt.RuntimeContext{
		Variables:    variables,
		State:        state,
		Envelope:     env,
		Paradigm:     "react",
		ConsumerID:   consumerID,
		Task:         n.task,
		Tools:        tools,
		Capabilities: caps,
		AgentSpec:    agentSpec,
	}
}

// resolvePrompt returns the assembled user prompt for the current iteration.
// When the registry is populated and the prompt resolves, it uses that.
// Otherwise it falls back to a minimal inline prompt so the agent stays functional
// before prompt files are authored.
func (n *reactThinkNode) resolvePrompt(env *contextdata.Envelope, tools []ports.Tool) string {
	reg := n.agent.PromptRegistry
	if reg != nil {
		promptID := promptIDFromTask(n.task)
		rctx := n.buildRuntimeContext(env, tools)
		if text, err := reg.Resolve(promptID, rctx); err == nil && text != "" {
			return text
		}
	}
	return minimalReactUserPrompt(n.task, tools)
}

// resolveSystemPrompt returns the system prompt for chat-based iterations.
func (n *reactThinkNode) resolveSystemPrompt(tools []ports.Tool) string {
	return minimalReactSystemPrompt(tools, n.agent)
}

// ensureMessages seeds or extends the chat history for tool-calling iterations.
func (n *reactThinkNode) ensureMessages(env *contextdata.Envelope, tools []ports.Tool) []model.Message {
	systemPrompt := n.resolveSystemPrompt(tools)
	userPrompt := n.resolvePrompt(env, tools)
	messages := getReactMessages(env)
	if len(messages) == 0 {
		messages = []model.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	} else {
		if messages[0].Role == "system" {
			messages[0].Content = systemPrompt
		} else {
			messages = append([]model.Message{{Role: "system", Content: systemPrompt}}, messages...)
		}
		messages = append(messages, model.Message{Role: "user", Content: userPrompt})
	}
	saveReactMessages(env, messages)
	return messages
}

// promptIDFromTask reads "prompt_id" from task.Context, falling back to the
// default react prompt ID.
func promptIDFromTask(task *execution.Task) string {
	if task != nil && task.Context != nil {
		if id, ok := task.Context["prompt_id"].(string); ok && id != "" {
			return id
		}
	}
	return "agent.react.default"
}

// minimalReactSystemPrompt is the inline fallback used when the registry has
// no matching prompt.
func minimalReactSystemPrompt(tools []ports.Tool, agent *ReActAgent) string {
	var lines []string
	var hasLSP, hasAST bool
	for _, tool := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s", tool.Name(), tool.Description()))
		if strings.HasPrefix(tool.Name(), "lsp_") {
			hasLSP = true
		}
		if strings.HasPrefix(tool.Name(), "ast_") {
			hasAST = true
		}
	}
	var guidance strings.Builder
	if hasLSP || hasAST {
		guidance.WriteString("\n\n### Code Analysis Capabilities\n")
		if hasLSP {
			guidance.WriteString("- Use 'lsp_*' tools to find definitions, references, and type information accurately.\n")
		}
		if hasAST {
			guidance.WriteString("- Use 'ast_*' tools to query the codebase structure (symbols, dependencies) efficiently.\n")
		}
		guidance.WriteString("- Always analyze the code context (definitions/refs) BEFORE attempting edits.\n")
	}
	if agent != nil && agent.Config != nil && agent.Config.AgentSpec != nil {
		if p := strings.TrimSpace(agent.Config.AgentSpec.Prompt); p != "" {
			guidance.WriteString("\n\n### Skill Guidance\n")
			guidance.WriteString(p)
			guidance.WriteRune('\n')
		}
	}
	return fmt.Sprintf(`You are a ReAct agent optimized for small local models.
Think carefully, but keep reasoning short.
Available tools:
%s%s
IMPORTANT: Only call tools listed above. Never invent or use tool names that are not in this list.
When information is missing, read/search before editing.
Return ONLY structured JSON. No prose outside the JSON object.`, strings.Join(lines, "\n"), guidance.String())
}

// minimalReactUserPrompt is the inline fallback prompt body.
func minimalReactUserPrompt(task *execution.Task, tools []ports.Tool) string {
	var toolSection strings.Builder
	for _, tool := range tools {
		if toolSection.Len() > 0 {
			toolSection.WriteString("\n")
		}
		toolSection.WriteString("- ")
		toolSection.WriteString(tool.Name())
		if desc := strings.TrimSpace(tool.Description()); desc != "" {
			toolSection.WriteString(": ")
			toolSection.WriteString(desc)
		}
	}
	instruction := ""
	if task != nil {
		instruction = task.Instruction
	}
	return fmt.Sprintf(`You are a ReAct agent optimized for a small-context local model.
Work step-by-step. Prefer the smallest useful action. Do not restate the task.

%s

Task: %s

Return ONLY JSON with fields:
{"thought":"short reasoning","action":"tool|complete","tool":"tool name or empty","arguments":{},"complete":true|false,"summary":"final answer when complete"}`, toolSection.String(), instruction)
}

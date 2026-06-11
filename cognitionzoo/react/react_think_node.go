package react

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	frameworktools "codeburg.org/lexbit/relurpify/capability/registry"
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
	configNativeTC := n.agent.Config != nil && n.agent.Config.NativeToolCalling
	profileNativeTC := false
	if !configNativeTC {
		if pm, ok := n.agent.Model.(model.ProfiledModel); ok {
			profileNativeTC = pm.UsesNativeToolCalling()
		}
	}
	useToolCalling := len(tools) > 0 && (configNativeTC || profileNativeTC)
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
	detectedCalls := filterToolCalls(frameworktools.ParseToolCallsFromText(resp.Text))
	if len(detectedCalls) > 0 {
		if maxTools > 0 && len(detectedCalls) > maxTools {
			detectedCalls = detectedCalls[:maxTools]
		}
		return decisionPayload{
			Thought:   trimToBudget(resp.Text, 220),
			Complete:  false,
			Timestamp: time.Now().UTC(),
		}, detectedCalls, nil
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
		if textSuggestsPendingToolCall(resp.Text) {
			return decisionPayload{Thought: trimToBudget(resp.Text, 220), Complete: false, Timestamp: time.Now().UTC()}, nil, nil
		}
		return decisionPayload{Thought: trimToBudget(resp.Text, 220), Complete: true, Timestamp: time.Now().UTC()}, nil, nil
	}
	parsed, err = parseDecision(repaired)
	if err != nil {
		if textSuggestsPendingToolCall(resp.Text) {
			return decisionPayload{Thought: trimToBudget(resp.Text, 220), Complete: false, Timestamp: time.Now().UTC()}, nil, nil
		}
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

// textSuggestsPendingToolCall returns true when the raw LLM response text looks
// like it was attempting to call a tool but the JSON could not be parsed.
// Used as a last-resort fallback before declaring the iteration complete so that
// embedded tool calls are not silently dropped when repair also fails.
func textSuggestsPendingToolCall(text string) bool {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, `"tool"`) {
		return false
	}
	if strings.Contains(lower, `"complete":true`) || strings.Contains(lower, `"action":"complete"`) {
		return false
	}
	// Confirm the "tool" key has a non-empty quoted value.
	idx := strings.Index(lower, `"tool"`)
	after := strings.TrimSpace(lower[idx+6:])
	if !strings.HasPrefix(after, ":") {
		return false
	}
	val := strings.TrimSpace(after[1:])
	return strings.HasPrefix(val, `"`) && !strings.HasPrefix(val, `""`)
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

type contextFilePayload struct {
	Path      string
	Content   string
	Summary   string
	Reference *agentgraph.ContextReference
	Truncated bool
}

func renderContextFiles(task *execution.Task, maxBytes int) string {
	files := extractContextFiles(task)
	if len(files) == 0 {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = 4000
	}
	var b strings.Builder
	remaining := maxBytes
	for _, file := range files {
		if file.Path == "" {
			continue
		}
		entry := renderContextFileEntry(file, remaining)
		if entry == "" {
			continue
		}
		if len(entry) > remaining {
			entry = entry[:remaining]
		}
		if len(entry) == 0 {
			break
		}
		b.WriteString(entry)
		if !strings.HasSuffix(entry, "\n") {
			b.WriteString("\n")
		}
		remaining = maxBytes - b.Len()
		if remaining <= 0 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func extractContextFiles(task *execution.Task) []contextFilePayload {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["context_file_contents"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]contextFilePayload, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			path, _ := m["path"].(string)
			content, _ := m["content"].(string)
			summary, _ := m["summary"].(string)
			truncated, _ := m["truncated"].(bool)
			var reference *agentgraph.ContextReference
			if rawRef, ok := m["reference"].(map[string]any); ok {
				reference = &agentgraph.ContextReference{
					Kind:    agentgraph.ContextReferenceKind(strings.TrimSpace(fmt.Sprint(rawRef["kind"]))),
					ID:      strings.TrimSpace(fmt.Sprint(rawRef["id"])),
					URI:     strings.TrimSpace(fmt.Sprint(rawRef["uri"])),
					Version: strings.TrimSpace(fmt.Sprint(rawRef["version"])),
					Detail:  strings.TrimSpace(fmt.Sprint(rawRef["detail"])),
				}
			}
			out = append(out, contextFilePayload{
				Path:      path,
				Content:   content,
				Summary:   summary,
				Reference: reference,
				Truncated: truncated,
			})
		}
		return out
	default:
		return nil
	}
}

func renderContextFileEntry(file contextFilePayload, maxBytes int) string {
	if maxBytes <= 0 || file.Path == "" {
		return ""
	}
	header := fmt.Sprintf("File: %s", file.Path)
	if file.Reference != nil && strings.TrimSpace(file.Reference.Detail) != "" {
		header += fmt.Sprintf(" [detail=%s]", strings.TrimSpace(file.Reference.Detail))
	}
	header += "\n"
	remaining := maxBytes - len(header)
	if remaining <= 0 {
		return ""
	}
	body := strings.TrimSpace(file.Content)
	if body == "" {
		body = strings.TrimSpace(file.Summary)
	}
	if body == "" {
		body = "reference only"
	}
	if len(body) > remaining {
		body = body[:remaining]
	}
	if file.Content != "" {
		return header + "```\n" + body + "\n```\n"
	}
	return header + body
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
	toolSection := frameworktools.RenderToolsToPrompt(tools)
	instruction := ""
	if task != nil {
		instruction = task.Instruction
	}
	return fmt.Sprintf(`You are a ReAct agent optimized for a small-context local model.
Work step-by-step. Prefer the smallest useful action. Do not restate the task.

%s

Task: %s

Return ONLY JSON with fields:
{"thought":"short reasoning","action":"tool|complete","tool":"tool name or empty","arguments":{},"complete":true|false,"summary":"final answer when complete"}`, toolSection, instruction)
}

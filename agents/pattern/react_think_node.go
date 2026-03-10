package pattern

import (
	"context"
	"fmt"
	"strings"
	"time"

	frameworktools "github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

type reactThinkNode struct {
	id    string
	agent *ReActAgent
	task  *core.Task
}

// ID returns the think node identifier.
func (n *reactThinkNode) ID() string { return n.id }

// Type marks the think step as an observation node.
func (n *reactThinkNode) Type() graph.NodeType { return graph.NodeTypeObservation }

// Execute drives the "think" portion of the ReAct loop and either emits a tool
// call or final answer instructions.
func (n *reactThinkNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("planning")
	n.agent.enforceBudget(state)
	n.agent.manageContextSignals(state)
	if summary := strings.TrimSpace(state.GetString("react.verification_latched_summary")); summary != "" {
		decision := decisionPayload{
			Thought:   "verification already succeeded",
			Complete:  true,
			Summary:   summary,
			Timestamp: time.Now().UTC(),
		}
		state.Set("react.tool_calls", []core.ToolCall{})
		state.Set("react.decision", decision)
		return &core.Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]interface{}{
				"decision": decision,
			},
		}, nil
	}
	var resp *core.LLMResponse
	var err error
	tools := n.agent.availableToolsForPhase(state, n.task)
	recordActiveToolNames(state, tools)
	useToolCalling := len(tools) > 0 && (n.agent.Config == nil || n.agent.Config.OllamaToolCalling)
	streamCB := n.streamCallback()
	if useToolCalling {
		messages := n.ensureMessages(state, tools)
		resp, err = n.agent.Model.ChatWithTools(ctx, messages, tools, &core.LLMOptions{
			Model:          n.agent.Config.Model,
			Temperature:    0.1,
			MaxTokens:      512,
			StreamCallback: streamCB,
		})
		if err == nil {
			saveReactMessages(state, messages)
		}
	} else {
		prompt := n.buildPrompt(state)
		resp, err = n.agent.Model.Generate(ctx, prompt, &core.LLMOptions{
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
		appendAssistantMessage(state, resp)
	}
	state.AddInteraction("assistant", resp.Text, map[string]interface{}{"node": n.id})
	n.agent.recordLatestInteraction(state)
	decision, toolCalls, err := n.normalizeDecision(ctx, state, resp, useToolCalling, tools)
	if err != nil {
		return nil, err
	}
	state.Set("react.tool_calls", toolCalls)
	state.Set("react.decision", decision)
	n.agent.debugf("%s decision=%+v tool_calls=%d", n.id, decision, len(resp.ToolCalls))
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]interface{}{
			"decision": decision,
		},
	}, nil
}

func (n *reactThinkNode) normalizeDecision(ctx context.Context, state *core.Context, resp *core.LLMResponse, useToolCalling bool, tools []core.Tool) (decisionPayload, []core.ToolCall, error) {
	if resp == nil {
		return decisionPayload{}, nil, fmt.Errorf("empty llm response")
	}
	if len(resp.ToolCalls) > 0 {
		call := resp.ToolCalls[0]
		return decisionPayload{
			Thought:   truncateForPrompt(resp.Text, 220),
			Tool:      call.Name,
			Arguments: call.Args,
			Complete:  false,
			Timestamp: time.Now().UTC(),
		}, filterToolCalls(resp.ToolCalls), nil
	}
	detectedCalls := filterToolCalls(frameworktools.ParseToolCallsFromText(resp.Text))
	if len(detectedCalls) > 0 {
		return decisionPayload{
			Thought:   truncateForPrompt(resp.Text, 220),
			Complete:  false,
			Timestamp: time.Now().UTC(),
		}, detectedCalls, nil
	}
	parsed, err := parseDecision(resp.Text)
	if err == nil && (parsed.Tool != "" || parsed.Complete) {
		return parsed, nil, nil
	}
	repaired, repairErr := n.repairDecision(ctx, tools, resp.Text, useToolCalling)
	if repairErr != nil {
		return decisionPayload{Thought: truncateForPrompt(resp.Text, 220), Complete: true, Timestamp: time.Now().UTC()}, nil, nil
	}
	parsed, err = parseDecision(repaired)
	if err != nil {
		return decisionPayload{Thought: truncateForPrompt(resp.Text, 220), Complete: true, Timestamp: time.Now().UTC()}, nil, nil
	}
	if parsed.Tool != "" {
		return parsed, []core.ToolCall{{Name: parsed.Tool, Args: parsed.Arguments}}, nil
	}
	return parsed, nil, nil
}

func (n *reactThinkNode) repairDecision(ctx context.Context, tools []core.Tool, raw string, useToolCalling bool) (string, error) {
	schema := `Return ONLY valid JSON:
{"thought":"short reasoning","action":"tool|complete","tool":"tool name or empty","arguments":{},"complete":true|false,"summary":"final answer when complete"}`
	prompt := fmt.Sprintf("%s\nAllowed tools: %s\nOriginal response:\n%s", schema, strings.Join(toolNames(tools), ", "), raw)
	resp, err := n.agent.Model.Generate(ctx, prompt, &core.LLMOptions{
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

func filterToolCalls(calls []core.ToolCall) []core.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]core.ToolCall, 0, len(calls))
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
	Truncated bool
}

func renderContextFiles(task *core.Task, maxBytes int) string {
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
		if file.Path == "" || file.Content == "" {
			continue
		}
		header := fmt.Sprintf("File: %s\n", file.Path)
		if remaining <= len(header) {
			break
		}
		b.WriteString(header)
		remaining -= len(header)
		content := file.Content
		if len(content) > remaining {
			content = content[:remaining]
		}
		b.WriteString("```")
		b.WriteString("\n")
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
		remaining = maxBytes - b.Len()
		if remaining <= 0 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func extractContextFiles(task *core.Task) []contextFilePayload {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context["context_file_contents"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []core.ContextFileContent:
		out := make([]contextFilePayload, 0, len(v))
		for _, file := range v {
			out = append(out, contextFilePayload{
				Path:      file.Path,
				Content:   file.Content,
				Truncated: file.Truncated,
			})
		}
		return out
	case []interface{}:
		out := make([]contextFilePayload, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			path, _ := m["path"].(string)
			content, _ := m["content"].(string)
			truncated, _ := m["truncated"].(bool)
			out = append(out, contextFilePayload{
				Path:      path,
				Content:   content,
				Truncated: truncated,
			})
		}
		return out
	default:
		return nil
	}
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

// buildPrompt returns a textual prompt when tool-calling chat APIs are not
// available.
func (n *reactThinkNode) buildPrompt(state *core.Context) string {
	tools := n.agent.availableToolsForPhase(state, n.task)
	toolSection := frameworktools.RenderToolsToPrompt(tools)
	assembler := newPromptContextAssembler(n.agent, n.task)
	promptBody := assembler.buildPrompt(state, tools, true)
	return fmt.Sprintf(`You are a ReAct agent optimized for a small-context local model.
Work step-by-step. Prefer the smallest useful action. Do not restate the task.

%s

Prompt context:
%s

Return ONLY JSON with fields:
{"thought":"short reasoning","action":"tool|complete","tool":"tool name or empty","arguments":{},"complete":true|false,"summary":"final answer when complete"}`, toolSection, promptBody)
}

// ensureMessages seeds or extends the chat history so each tool-calling
// iteration keeps prior assistant/tool turns while refreshing the current
// prompt context.
func (n *reactThinkNode) ensureMessages(state *core.Context, tools []core.Tool) []core.Message {
	assembler := newPromptContextAssembler(n.agent, n.task)
	systemPrompt := n.buildSystemPrompt(tools)
	userPrompt := assembler.buildPrompt(state, tools, true)
	messages := getReactMessages(state)
	if len(messages) == 0 {
		messages = []core.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	} else {
		if messages[0].Role == "system" {
			messages[0].Content = systemPrompt
		} else {
			messages = append([]core.Message{{Role: "system", Content: systemPrompt}}, messages...)
		}
		messages = append(messages, core.Message{Role: "user", Content: userPrompt})
	}
	saveReactMessages(state, messages)
	return messages
}

// buildSystemPrompt summarizes tool descriptions for the chat-based workflow.
func (n *reactThinkNode) buildSystemPrompt(tools []core.Tool) string {
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

	if n.agent != nil && n.agent.Config != nil && n.agent.Config.AgentSpec != nil {
		prompt := strings.TrimSpace(n.agent.Config.AgentSpec.Prompt)
		if prompt != "" {
			guidance.WriteString("\n\n### Skill Guidance\n")
			guidance.WriteString(prompt)
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

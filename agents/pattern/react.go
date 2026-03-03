package pattern

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"log"
	"strings"
	"time"
)

// ReActAgent implements the Reason+Act pattern.
// ModeRuntimeProfile conveys high-level runtime settings to the agent.
type ModeRuntimeProfile struct {
	Name        string
	Description string
	Temperature float64
	Context     ContextPreferences
}

// ContextPreferences tune context management for a mode.
type ContextPreferences struct {
	PreferredDetailLevel contextmgr.DetailLevel
	MinHistorySize       int
	CompressionThreshold float64
}

// ReActAgent implements the Reason+Act pattern.
type ReActAgent struct {
	Model        core.LanguageModel
	Tools        *toolsys.ToolRegistry
	Memory       memory.MemoryStore
	Config       *core.Config
	IndexManager *ast.IndexManager
	maxIterations int
	contextPolicy *contextmgr.ContextPolicy

	Mode            string
	ModeProfile     ModeRuntimeProfile
	sharedContext   *core.SharedContext
	initialLoadDone bool
}

// Initialize wires configuration.
func (a *ReActAgent) Initialize(config *core.Config) error {
	a.Config = config
	if config.MaxIterations <= 0 {
		a.maxIterations = 8
	} else {
		a.maxIterations = config.MaxIterations
	}
	if a.Tools == nil {
		a.Tools = toolsys.NewToolRegistry()
	}
	if a.Mode == "" {
		a.Mode = "code"
	}
	if a.ModeProfile.Name == "" {
		a.ModeProfile = ModeRuntimeProfile{
			Name:        a.Mode,
			Description: "Reason + Act agent",
			Temperature: 0.2,
			Context: ContextPreferences{
				PreferredDetailLevel: contextmgr.DetailDetailed,
				MinHistorySize:       5,
				CompressionThreshold: 0.8,
			},
		}
	}
	strategy := contextmgr.ContextStrategy(nil)
	if a.contextPolicy != nil {
		strategy = a.contextPolicy.Strategy
	}
	if strategy == nil {
		switch strings.ToLower(a.Mode) {
		case "debug", "ask":
			strategy = contextmgr.NewAggressiveStrategy()
		case "architect":
			strategy = contextmgr.NewConservativeStrategy()
		default:
			strategy = contextmgr.NewAdaptiveStrategy()
		}
	}
	var spec *core.AgentContextSpec
	if config != nil && config.AgentSpec != nil {
		spec = &config.AgentSpec.Context
	}
	if a.contextPolicy == nil {
		a.contextPolicy = contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
			Strategy:     strategy,
			IndexManager: a.IndexManager,
			Preferences: contextmgr.ContextPolicyPreferences{
				PreferredDetailLevel: a.ModeProfile.Context.PreferredDetailLevel,
				MinHistorySize:       a.ModeProfile.Context.MinHistorySize,
				CompressionThreshold: a.ModeProfile.Context.CompressionThreshold,
			},
		}, spec)
	} else {
		a.contextPolicy.Strategy = strategy
		a.contextPolicy.Preferences = contextmgr.ContextPolicyPreferences{
			PreferredDetailLevel: a.ModeProfile.Context.PreferredDetailLevel,
			MinHistorySize:       a.ModeProfile.Context.MinHistorySize,
			CompressionThreshold: a.ModeProfile.Context.CompressionThreshold,
		}
		a.contextPolicy.ApplyAgentContextSpec(spec)
	}
	a.contextPolicy.Budget.SetReservations(1000, 2000, 1000)
	return nil
}

// debugf logs formatted messages whenever agent debug logging is enabled.
func (a *ReActAgent) debugf(format string, args ...interface{}) {
	if a == nil || a.Config == nil || !a.Config.DebugAgent {
		return
	}
	log.Printf("[react] "+format, args...)
}

// Execute runs the task through the workflow graph.
func (a *ReActAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	a.initialLoadDone = false
	a.sharedContext = core.NewSharedContext(state, a.contextPolicy.Budget, a.contextPolicy.Summarizer)
	if a.contextPolicy != nil && task != nil {
		if err := a.contextPolicy.InitialLoad(task); err != nil {
			a.debugf("initial context load failed: %v", err)
		} else {
			a.initialLoadDone = true
		}
	}
	defer func() {
		a.sharedContext = nil
		a.initialLoadDone = false
	}()
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		graph.SetTelemetry(cfg.Telemetry)
	}
	result, err := graph.Execute(ctx, state)
	return result, err
}

// Capabilities describes what the agent can do.
func (a *ReActAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityCode,
		core.CapabilityExplain,
	}
}

// BuildGraph constructs the ReAct workflow.
func (a *ReActAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("react agent missing language model")
	}
	think := &reactThinkNode{
		id:    "react_think",
		agent: a,
		task:  task,
	}
	act := &reactActNode{
		id:    "react_act",
		agent: a,
	}
	observe := &reactObserveNode{
		id:    "react_observe",
		agent: a,
		task:  task,
	}
	return graph.BuildThinkActObserveGraph(
		think,
		act,
		observe,
		func(result *core.Result, ctx *core.Context) bool {
			done, _ := ctx.Get("react.done")
			return done == false || done == nil
		},
		func(result *core.Result, ctx *core.Context) bool {
			done, _ := ctx.Get("react.done")
			return done == true
		},
		"react_done",
	)
}

func (a *ReActAgent) enforceBudget(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	var tools []core.Tool
	if a.Tools != nil {
		tools = a.Tools.All()
	}
	a.contextPolicy.EnforceBudget(state, a.sharedContext, a.Model, tools, a.debugf)
}

func (a *ReActAgent) recordLatestInteraction(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	a.contextPolicy.RecordLatestInteraction(state, a.debugf)
}

func (a *ReActAgent) manageContextSignals(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	lastResult := a.getLastResult(state)
	a.contextPolicy.HandleSignals(state, a.sharedContext, lastResult)
}

func (a *ReActAgent) getLastResult(state *core.Context) *core.Result {
	if state == nil {
		return nil
	}
	if val, ok := state.GetHandle("react.last_result"); ok {
		if res, ok := val.(*core.Result); ok {
			return res
		}
	}
	val, ok := state.Get("react.last_result")
	if ok {
		if res, ok := val.(*core.Result); ok {
			return res
		}
	}
	return nil
}

// --- ReAct Graph nodes ---

type reactThinkNode struct {
	id    string
	agent *ReActAgent
	task  *core.Task
}

// ID returns the think node identifier.
func (n *reactThinkNode) ID() string { return n.id }

// Type marks the think step as an observation node.
func (n *reactThinkNode) Type() graph.NodeType { return graph.NodeTypeObservation }

// Execute drives the “think” portion of the ReAct loop and either emits a tool
// call or final answer instructions.
func (n *reactThinkNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("planning")
	n.agent.enforceBudget(state)
	n.agent.manageContextSignals(state)
	var resp *core.LLMResponse
	var err error
	tools := n.agent.Tools.All()
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
			messages = append(messages, core.Message{
				Role:      "assistant",
				Content:   resp.Text,
				ToolCalls: resp.ToolCalls,
			})
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
	state.AddInteraction("assistant", resp.Text, map[string]interface{}{"node": n.id})
	n.agent.recordLatestInteraction(state)
	var decision decisionPayload
	if len(resp.ToolCalls) > 0 {
		decision = decisionPayload{
			Thought:   resp.Text,
			Tool:      resp.ToolCalls[0].Name,
			Arguments: resp.ToolCalls[0].Args,
			Complete:  false,
		}
		state.Set("react.tool_calls", resp.ToolCalls)
	} else if useToolCalling {
		// Some Ollama models return tool calls as JSON blobs in plain text even
		// when the chat_with_tools API is used. Prefer extracting those calls
		// before falling back to the decision parser.
		detectedCalls := toolsys.ParseToolCallsFromText(resp.Text)
		detectedCalls = filterToolCalls(detectedCalls)
		if len(detectedCalls) > 0 {
			state.Set("react.tool_calls", detectedCalls)
			decision = decisionPayload{
				Thought:   resp.Text,
				Complete:  false,
				Timestamp: time.Now().UTC(),
			}
		} else {
			parsed, err := parseDecision(resp.Text)
			if err == nil && (parsed.Tool != "" || parsed.Complete) {
				decision = parsed
			} else {
				decision = decisionPayload{Thought: resp.Text, Complete: true}
			}
			state.Set("react.tool_calls", []core.ToolCall{})
		}
	} else {
		parsed, err := parseDecision(resp.Text)

		// Fallback: Check if the framework helper finds distinct tool calls (e.g. in markdown blocks)
		// even if the single-object parser failed or found nothing.
		detectedCalls := toolsys.ParseToolCallsFromText(resp.Text)

		if len(detectedCalls) > 0 {
			// Found tools via text parsing
			state.Set("react.tool_calls", detectedCalls)

			// Use thought from parsed if available, else full text
			thought := parsed.Thought
			if thought == "" {
				thought = resp.Text
			}
			decision = decisionPayload{
				Thought:   thought,
				Complete:  false,
				Timestamp: time.Now().UTC(),
			}
		} else {
			if err != nil {
				decision = decisionPayload{Thought: resp.Text, Complete: true}
			} else {
				decision = parsed
			}
			state.Set("react.tool_calls", []core.ToolCall{})
		}
	}
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
	var hasLSP, hasAST bool
	for _, tool := range n.agent.Tools.All() {
		if strings.HasPrefix(tool.Name(), "lsp_") {
			hasLSP = true
		}
		if strings.HasPrefix(tool.Name(), "ast_") {
			hasAST = true
		}
	}
	var last string
	if res, ok := state.Get("react.last_tool_result"); ok {
		last = fmt.Sprint(res)
	}

	toolSection := toolsys.RenderToolsToPrompt(n.agent.Tools.All())

	var guidance strings.Builder
	if hasLSP || hasAST {
		guidance.WriteString("\nCode Analysis:\n")
		if hasLSP {
			guidance.WriteString("- Prefer LSP tools for precise navigation.\n")
		}
		if hasAST {
			guidance.WriteString("- Prefer AST tools for structure queries.\n")
		}
	}
	if val, ok := n.task.Context["plan"]; ok {
		if planJSON, err := json.MarshalIndent(val, "", "  "); err == nil {
			guidance.WriteString("\nPlan:\n")
			guidance.Write(planJSON)
			guidance.WriteRune('\n')
		}
	}
	if contextFiles := renderContextFiles(n.task, 4000); contextFiles != "" {
		guidance.WriteString("\nContext Files:\n")
		guidance.WriteString(contextFiles)
		guidance.WriteRune('\n')
	}
	if n.agent != nil && n.agent.Config != nil && n.agent.Config.AgentSpec != nil {
		prompt := strings.TrimSpace(n.agent.Config.AgentSpec.Prompt)
		if prompt != "" {
			guidance.WriteString("\nSkill Guidance:\n")
			guidance.WriteString(prompt)
			guidance.WriteRune('\n')
		}
	}

	return fmt.Sprintf(`You are a ReAct agent tasked with "%s".
%s
%s
Recent tool results: %s
Provide your response as a JSON object with "thought" and "tool"/"arguments" fields (or "complete": true).`, n.task.Instruction, toolSection, guidance.String(), last)
}

// ensureMessages seeds the chat history when tool calling is enabled so each
// iteration builds on prior reasoning.
func (n *reactThinkNode) ensureMessages(state *core.Context, tools []core.Tool) []core.Message {
	messages := getReactMessages(state)
	if len(messages) > 0 {
		return messages
	}
	systemPrompt := n.buildSystemPrompt(tools)
	userPrompt := fmt.Sprintf("Task: %s", n.task.Instruction)
	messages = []core.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
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

	// Inject Plan if available from Coordinator
	if val, ok := n.task.Context["plan"]; ok {
		// Attempt to marshal plan to JSON for the prompt
		if planJSON, err := json.MarshalIndent(val, "", "  "); err == nil {
			guidance.WriteString("\n\n### Execution Plan\nFollow this plan:\n")
			guidance.Write(planJSON)
			guidance.WriteRune('\n')
		}
	}
	if contextFiles := renderContextFiles(n.task, 4000); contextFiles != "" {
		guidance.WriteString("\n\n### Context Files\n")
		guidance.WriteString(contextFiles)
		guidance.WriteRune('\n')
	}
	if n.agent != nil && n.agent.Config != nil && n.agent.Config.AgentSpec != nil {
		prompt := strings.TrimSpace(n.agent.Config.AgentSpec.Prompt)
		if prompt != "" {
			guidance.WriteString("\n\n### Skill Guidance\n")
			guidance.WriteString(prompt)
			guidance.WriteRune('\n')
		}
	}

	return fmt.Sprintf(`You are a ReAct agent. Think carefully, call tools when required, and finish with a concise summary.
Available tools:
%s%s
If the task asks about the contents of a file (e.g. "Summarize README.md"), you MUST call file_read to fetch it before answering.
When you call a tool, wait for its response before continuing. When the work is complete, provide the final answer as plain text.`, strings.Join(lines, "\n"), guidance.String())
}

type reactActNode struct {
	id    string
	agent *ReActAgent
}

// ID returns the node identifier for the “act” step.
func (n *reactActNode) ID() string { return n.id }

// Type labels the node as a tool execution step.
func (n *reactActNode) Type() graph.NodeType { return graph.NodeTypeTool }

// Execute runs any pending tool calls or directly invokes the requested tool
// referenced in the latest decision payload.
func (n *reactActNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("executing")
	if pending, ok := state.Get("react.tool_calls"); ok {
		if calls, ok := pending.([]core.ToolCall); ok && len(calls) > 0 {
			calls = filterToolCalls(calls)
			if len(calls) == 0 {
				state.Set("react.tool_calls", []core.ToolCall{})
			} else {
				results := make(map[string]interface{})
				toolErrors := make([]string, 0)
				overallSuccess := true
				for _, call := range calls {
					tool, ok := n.agent.Tools.Get(call.Name)
					if !ok {
						return nil, fmt.Errorf("unknown tool %s", call.Name)
					}
					n.agent.debugf("%s executing tool=%s args=%v", n.id, call.Name, call.Args)
					res, err := tool.Execute(ctx, state, call.Args)
					if err != nil {
						return nil, err
					}
					if res != nil {
						results[call.Name] = map[string]interface{}{
							"success": res.Success,
							"data":    res.Data,
							"error":   res.Error,
						}
						appendToolMessage(state, call, res)
						n.agent.debugf("%s tool=%s result=%v", n.id, call.Name, res.Data)
						if !res.Success {
							overallSuccess = false
							if res.Error != "" {
								toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", call.Name, res.Error))
							} else {
								toolErrors = append(toolErrors, fmt.Sprintf("%s failed", call.Name))
							}
						}
					}
				}
				state.Set("react.last_tool_result", results)
				state.Set("react.tool_calls", []core.ToolCall{})
				result := &core.Result{NodeID: n.id, Success: overallSuccess, Data: results}
				if len(toolErrors) > 0 {
					result.Error = fmt.Errorf("%s", strings.Join(toolErrors, "; "))
				}
				state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
				return result, nil
			}
		}
		if n.agent.Config != nil && !n.agent.Config.OllamaToolCalling {
			state.Set("react.tool_calls", []core.ToolCall{})
		}
	}
	val, ok := state.Get("react.decision")
	if !ok {
		return nil, fmt.Errorf("missing decision from think step")
	}
	decision := val.(decisionPayload)
	toolName := strings.TrimSpace(decision.Tool)
	if decision.Complete || toolName == "" || strings.EqualFold(toolName, "none") {
		state.Set("react.last_tool_result", map[string]interface{}{})
		result := &core.Result{NodeID: n.id, Success: true}
		state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
		return result, nil
	}
	tool, ok := n.agent.Tools.Get(toolName)
	if !ok {
		lower := strings.ToLower(toolName)
		if lower == "" || strings.Contains(lower, "none") {
			state.Set("react.last_tool_result", map[string]interface{}{})
			result := &core.Result{NodeID: n.id, Success: true}
			state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
			return result, nil
		}
		return nil, fmt.Errorf("unknown tool %s", toolName)
	}
	res, err := tool.Execute(ctx, state, decision.Arguments)
	if err != nil {
		return nil, err
	}
	appendToolMessage(state, core.ToolCall{
		ID:   NewUUID(),
		Name: decision.Tool,
		Args: decision.Arguments,
	}, res)
	state.Set("react.last_tool_result", res.Data)
	n.agent.debugf("%s tool=%s result=%v", n.id, decision.Tool, res.Data)
	result := &core.Result{
		NodeID:  n.id,
		Success: res.Success,
		Data:    res.Data,
		Error:   parseError(res.Error),
	}
	state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
	return result, nil
}

type reactObserveNode struct {
	id    string
	agent *ReActAgent
	task  *core.Task
}

// ID returns the node identifier for the observe step.
func (n *reactObserveNode) ID() string { return n.id }

// Type marks the step as an observation/validation pass.
func (n *reactObserveNode) Type() graph.NodeType { return graph.NodeTypeObservation }

// Execute captures tool output, tracks loop iterations, and determines whether
// the ReAct loop should continue.
func (n *reactObserveNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("validating")
	iterVal, _ := state.Get("react.iteration")
	iter, _ := iterVal.(int)
	iter++
	state.Set("react.iteration", iter)
	decisionVal, _ := state.Get("react.decision")
	decision, _ := decisionVal.(decisionPayload)
	lastRes, _ := state.Get("react.last_tool_result")
	lastMap, _ := lastRes.(map[string]interface{})
	var diagnostic strings.Builder
	diagnostic.WriteString(fmt.Sprintf("Iteration %d observation.\n", iter))
	if decision.Thought != "" {
		diagnostic.WriteString("Thought: " + decision.Thought + "\n")
	}
	if len(lastMap) > 0 {
		diagnostic.WriteString("Tool Result: ")
		diagnostic.WriteString(fmt.Sprint(lastMap))
		diagnostic.WriteRune('\n')
	}
	completed := decision.Complete || iter >= n.agent.maxIterations
	if res, ok := state.Get("react.tool_calls"); ok {
		if calls, ok := res.([]core.ToolCall); ok && len(calls) > 0 {
			completed = false
		}
	}
	state.Set("react.done", completed)

	if n.agent.Memory != nil {
		_ = n.agent.Memory.Remember(ctx, NewUUID(), map[string]interface{}{
			"task":      n.task.Instruction,
			"iteration": iter,
			"decision":  decision,
		}, memory.MemoryScopeSession)
	}

	if completed {
		state.Set("react.final_output", map[string]interface{}{
			"summary": diagnostic.String(),
			"result":  lastMap,
		})
	}
	n.agent.debugf("%s completed=%v diagnostic=%s", n.id, completed, diagnostic.String())
	result := &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]interface{}{
			"diagnostic": diagnostic.String(),
			"complete":   completed,
		},
	}
	state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
	return result, nil
}

// decisionPayload models the JSON output of the think step.
type decisionPayload struct {
	Thought   string                 `json:"thought"`
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
	Complete  bool                   `json:"complete"`
	Reason    string                 `json:"reason"`
	Timestamp time.Time              `json:"timestamp"`
}

// parseDecision extracts the model's JSON payload (or falls back to the raw
// text) and normalizes it into the decisionPayload struct.
func parseDecision(raw string) (decisionPayload, error) {
	var payload decisionPayload
	snippet := ExtractJSON(raw)
	if snippet == "{}" {
		payload.Thought = strings.TrimSpace(raw)
		payload.Complete = true
		payload.Timestamp = time.Now().UTC()
		return payload, nil
	}
	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(snippet), &generic); err != nil {
		return payload, err
	}
	if thought, ok := generic["thought"].(string); ok && thought != "" {
		payload.Thought = thought
	} else if payload.Thought == "" {
		payload.Thought = strings.TrimSpace(raw)
	}
	if tool, ok := generic["tool"].(string); ok {
		payload.Tool = tool
	} else if name, ok := generic["name"].(string); ok {
		payload.Tool = name
	}
	if args, ok := generic["arguments"]; ok {
		payload.Arguments = normalizeArguments(args)
	}
	if payload.Arguments == nil {
		payload.Arguments = map[string]interface{}{}
	}
	if complete, ok := generic["complete"].(bool); ok {
		payload.Complete = complete
	}
	if reason, ok := generic["reason"].(string); ok {
		payload.Reason = reason
	}
	payload.Timestamp = time.Now().UTC()
	return payload, nil
}

// normalizeArguments coerces stringified JSON arguments into maps so tools
// always receive structured input.
func normalizeArguments(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return v
	case string:
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(v), &obj); err == nil {
			return obj
		}
		return map[string]interface{}{"value": v}
	default:
		return map[string]interface{}{}
	}
}

// parseError converts an error message string into an error value.
func parseError(err string) error {
	if err == "" {
		return nil
	}
	return errors.New(err)
}

const reactMessagesKey = "react.messages"

// getReactMessages reads a copy of the stored chat transcript.
func getReactMessages(state *core.Context) []core.Message {
	raw, ok := state.Get(reactMessagesKey)
	if !ok {
		return nil
	}
	messages, ok := raw.([]core.Message)
	if !ok || len(messages) == 0 {
		return nil
	}
	copyMessages := make([]core.Message, len(messages))
	copy(copyMessages, messages)
	return copyMessages
}

// saveReactMessages overwrites the stored transcript with a defensive copy.
func saveReactMessages(state *core.Context, messages []core.Message) {
	if len(messages) == 0 {
		state.Set(reactMessagesKey, []core.Message{})
		return
	}
	copyMessages := make([]core.Message, len(messages))
	copy(copyMessages, messages)
	state.Set(reactMessagesKey, copyMessages)
}

// appendToolMessage records tool responses in the transcript so the LLM can
// observe prior results when tool calling is used.
func appendToolMessage(state *core.Context, call core.ToolCall, res *core.ToolResult) {
	messages := getReactMessages(state)
	if len(messages) == 0 || res == nil {
		return
	}
	payload := map[string]interface{}{
		"success": res.Success,
	}
	if len(res.Data) > 0 {
		payload["data"] = res.Data
	}
	if res.Error != "" {
		payload["error"] = res.Error
	}
	if len(res.Metadata) > 0 {
		payload["metadata"] = res.Metadata
	}
	encoded, err := json.Marshal(payload)
	content := string(encoded)
	if err != nil {
		content = fmt.Sprintf("success=%t data=%v error=%s", res.Success, res.Data, res.Error)
	}
	messages = append(messages, core.Message{
		Role:       "tool",
		Name:       call.Name,
		Content:    content,
		ToolCallID: call.ID,
	})
	saveReactMessages(state, messages)
}

func reactTaskScope(state *core.Context) string {
	if state == nil {
		return ""
	}
	return state.GetString("task.id")
}

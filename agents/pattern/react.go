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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/persistence"
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
	Model          core.LanguageModel
	Tools          *toolsys.ToolRegistry
	Memory         memory.MemoryStore
	Config         *core.Config
	IndexManager   *ast.IndexManager
	CheckpointPath string
	maxIterations  int
	contextPolicy  *contextmgr.ContextPolicy

	Mode            string
	ModeProfile     ModeRuntimeProfile
	sharedContext   *core.SharedContext
	initialLoadDone bool
}

const (
	contextmgrPhaseExplore = "explore"
	contextmgrPhaseEdit    = "edit"
	contextmgrPhaseVerify  = "verify"
)

type ToolObservation struct {
	Tool      string                 `json:"tool"`
	Phase     string                 `json:"phase"`
	Summary   string                 `json:"summary"`
	Args      map[string]interface{} `json:"args,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Success   bool                   `json:"success"`
	Timestamp time.Time              `json:"timestamp"`
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
	a.initializePhase(state, task)
	if a.CheckpointPath != "" && task != nil && task.ID != "" {
		store := persistence.NewCheckpointStore(filepath.Clean(a.CheckpointPath))
		graph.WithCheckpointing(2, store.Save)
	}
	result, err := graph.Execute(ctx, state)
	if err == nil && result != nil {
		rawLast, _ := state.Get("react.last_tool_result")
		lastMap, _ := rawLast.(map[string]interface{})
		if summary, ok := completionSummaryFromState(a, task, state, lastMap); ok {
			state.Set("react.incomplete_reason", "")
			state.Set("react.synthetic_summary", summary)
			state.Set("react.final_output", map[string]interface{}{
				"summary": summary,
				"result":  lastMap,
			})
			result.Success = true
			result.Error = nil
		} else if summary, ok := finalResultFallbackSummary(task, state); ok {
			state.Set("react.incomplete_reason", "")
			state.Set("react.synthetic_summary", summary)
			state.Set("react.final_output", map[string]interface{}{
				"summary": summary,
				"result":  lastMap,
			})
			result.Success = true
			result.Error = nil
		}
		if final, ok := state.Get("react.final_output"); ok {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["final_output"] = final
			if summary := finalOutputSummary(final); summary != "" {
				result.Data["text"] = summary
			}
		}
		if reason := strings.TrimSpace(state.GetString("react.incomplete_reason")); reason != "" {
			result.Success = false
			result.Error = fmt.Errorf("%s", reason)
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["incomplete_reason"] = reason
		}
	}
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
		task:  task,
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

func (a *ReActAgent) initializePhase(state *core.Context, task *core.Task) {
	if state == nil {
		return
	}
	if phase := state.GetString("react.phase"); phase != "" {
		return
	}
	phase := contextmgrPhaseExplore
	text := taskInstructionText(task)
	if task != nil && task.Context != nil {
		if _, ok := task.Context["current_step"]; ok {
			if strings.Contains(text, "verify") || strings.Contains(text, "test") || strings.Contains(text, "build") {
				phase = contextmgrPhaseVerify
			}
		}
	}
	if !taskNeedsEditing(task) && taskRequiresVerification(task) && len(explicitlyRequestedToolNames(task)) > 0 {
		phase = contextmgrPhaseVerify
	}
	if strings.EqualFold(a.Mode, "debug") && (strings.Contains(text, "test") || strings.Contains(text, "build") || strings.Contains(text, "lint") || strings.Contains(text, "cargo")) {
		phase = contextmgrPhaseVerify
	}
	if strings.EqualFold(a.Mode, "docs") {
		phase = contextmgrPhaseEdit
	}
	state.Set("react.phase", phase)
}

func (a *ReActAgent) availableToolsForPhase(state *core.Context, task *core.Task) []core.Tool {
	if a.Tools == nil {
		return nil
	}
	phase := contextmgrPhaseExplore
	if state != nil {
		if current := state.GetString("react.phase"); current != "" {
			phase = current
		}
	}
	var filtered []core.Tool
	for _, tool := range a.Tools.All() {
		if toolAllowedForPhase(tool, phase, task) || a.recoveryToolAllowed(state, task, tool.Name()) {
			if !a.toolAllowedBySkillConfig(task, phase, tool.Name()) {
				continue
			}
			if !a.toolAllowedByExecutionContext(state, task, phase, tool) {
				continue
			}
			filtered = append(filtered, tool)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name() < filtered[j].Name() })
	return filtered
}

func (a *ReActAgent) toolAllowedByExecutionContext(state *core.Context, task *core.Task, phase string, tool core.Tool) bool {
	if tool == nil {
		return false
	}
	if requested := explicitlyRequestedToolNames(task); len(requested) > 0 && verificationLikeTool(tool) {
		if _, ok := requested[strings.ToLower(strings.TrimSpace(tool.Name()))]; !ok {
			return false
		}
	}
	if phase != contextmgrPhaseEdit {
		return true
	}
	if hasEditObservation(state) {
		return true
	}
	if tool.Name() == "file_read" && repeatedReadTarget(state) != "" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(tool.Name()))
	if strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.Contains(name, "fmt") {
		return false
	}
	if taskNeedsEditing(task) && hasFailureFromState(state) && verificationLikeTool(tool) {
		return false
	}
	return true
}

func (a *ReActAgent) recoveryToolAllowed(state *core.Context, task *core.Task, toolName string) bool {
	if state == nil || !hasFailureFromState(state) {
		return false
	}
	for _, probe := range a.recoveryProbeTools(task) {
		if strings.EqualFold(strings.TrimSpace(probe), toolName) {
			return true
		}
	}
	return false
}

func (a *ReActAgent) toolAllowedBySkillConfig(task *core.Task, phase, toolName string) bool {
	resolved := a.resolvedSkillPolicy(task)
	if len(resolved.PhaseTools) == 0 {
		return true
	}
	allowed, ok := resolved.PhaseTools[phase]
	if !ok || len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		if strings.EqualFold(strings.TrimSpace(entry), toolName) {
			return true
		}
	}
	return false
}

func (a *ReActAgent) resolvedSkillPolicy(task *core.Task) toolsys.ResolvedSkillPolicy {
	return toolsys.ResolveEffectiveSkillPolicy(task, a.effectiveAgentSpec(task), a.Tools).Policy
}

func (a *ReActAgent) recoveryProbeTools(task *core.Task) []string {
	resolved := a.resolvedSkillPolicy(task)
	return append([]string{}, resolved.RecoveryProbeTools...)
}

func (a *ReActAgent) verificationSuccessTools(task *core.Task) []string {
	resolved := a.resolvedSkillPolicy(task)
	return append([]string{}, resolved.VerificationSuccessTools...)
}

func (a *ReActAgent) effectiveAgentSpec(task *core.Task) *core.AgentRuntimeSpec {
	if a == nil || a.Config == nil {
		return toolsys.EffectiveAgentSpec(task, nil)
	}
	return toolsys.EffectiveAgentSpec(task, a.Config.AgentSpec)
}

func toolAllowedForPhase(tool core.Tool, phase string, task *core.Task) bool {
	if tool == nil {
		return false
	}
	name := tool.Name()
	tags := tool.Tags()
	if len(tags) == 0 {
		return true
	}
	hasTag := func(target string) bool {
		for _, tag := range tags {
			if tag == target {
				return true
			}
		}
		return false
	}
	switch phase {
	case contextmgrPhaseEdit:
		if hasTag(core.TagDestructive) {
			return true
		}
		if hasTag(core.TagExecute) {
			return isLanguageExecutionTool(name, task)
		}
		if hasTag(core.TagReadOnly) {
			return strings.HasPrefix(name, "file_") || strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
		}
		return name == "exec_run_code"
	case contextmgrPhaseVerify:
		if hasTag(core.TagExecute) {
			return true
		}
		return strings.Contains(name, "rustfmt") || strings.Contains(name, "format") || strings.HasPrefix(name, "file_read")
	default:
		if hasTag(core.TagReadOnly) {
			return true
		}
		if hasTag(core.TagExecute) {
			return strings.EqualFold(taskMode(task), "debug") && isLanguageExecutionTool(name, task)
		}
		return strings.HasPrefix(name, "ast_") || strings.HasPrefix(name, "lsp_") || strings.Contains(name, "grep")
	}
}

func isLanguageExecutionTool(name string, task *core.Task) bool {
	name = strings.ToLower(name)
	if _, ok := explicitlyRequestedToolNames(task)[name]; ok {
		return true
	}
	if strings.Contains(name, "cargo") || strings.Contains(name, "rustfmt") {
		return true
	}
	if strings.Contains(name, "sqlite") {
		return true
	}
	if strings.Contains(name, "test") || strings.Contains(name, "build") || strings.Contains(name, "lint") || strings.Contains(name, "format") || strings.Contains(name, "check") {
		return true
	}
	if strings.Contains(name, "exec_run_code") {
		return true
	}
	text := ""
	if task != nil {
		text = strings.ToLower(task.Instruction)
	}
	return strings.Contains(text, "test") || strings.Contains(text, "build") || strings.Contains(text, "lint")
}

func taskMode(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(task.Context["mode"]))
}

func activeToolSet(state *core.Context) map[string]struct{} {
	out := map[string]struct{}{}
	if state == nil {
		return out
	}
	raw, ok := state.Get("react.active_tools")
	if !ok {
		return out
	}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			out[value] = struct{}{}
		}
	case []any:
		for _, value := range values {
			out[fmt.Sprint(value)] = struct{}{}
		}
	}
	return out
}

func recordActiveToolNames(state *core.Context, tools []core.Tool) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	state.Set("react.active_tools", names)
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
	detectedCalls := filterToolCalls(toolsys.ParseToolCallsFromText(resp.Text))
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
	toolSection := toolsys.RenderToolsToPrompt(tools)
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

type reactActNode struct {
	id    string
	agent *ReActAgent
	task  *core.Task
}

// ID returns the node identifier for the “act” step.
func (n *reactActNode) ID() string { return n.id }

// Type labels the node as a tool execution step.
func (n *reactActNode) Type() graph.NodeType { return graph.NodeTypeTool }

// Execute runs any pending tool calls or directly invokes the requested tool
// referenced in the latest decision payload.
func (n *reactActNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("executing")
	activeTools := activeToolSet(state)
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
					tool, ok := n.lookupTool(call.Name, activeTools)
					if !ok {
						errResult := &core.ToolResult{
							Success: false,
							Error:   fmt.Sprintf("tool %q does not exist. Only use tools from the available list.", call.Name),
						}
						n.recordObservation(state, call, errResult)
						overallSuccess = false
						toolErrors = append(toolErrors, fmt.Sprintf("unknown tool %s", call.Name))
						continue
					}
					n.agent.debugf("%s executing tool=%s args=%v", n.id, call.Name, call.Args)
					res, err := tool.Execute(ctx, state, call.Args)
					if err != nil {
						return nil, err
					}
					if res != nil {
						n.recordObservation(state, call, res)
						n.latchVerificationSuccess(state, call.Name, res)
						results[call.Name] = map[string]interface{}{
							"success": res.Success,
							"data":    res.Data,
							"error":   res.Error,
						}
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
	tool, ok := n.lookupTool(toolName, activeTools)
	if !ok {
		lower := strings.ToLower(toolName)
		if lower == "" || strings.Contains(lower, "none") {
			state.Set("react.last_tool_result", map[string]interface{}{})
			result := &core.Result{NodeID: n.id, Success: true}
			state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
			return result, nil
		}
		// Feed error back to the LLM so it can retry with a valid tool name.
		errMsg := fmt.Sprintf("tool %q does not exist. Only use tools from the available list.", toolName)
		state.Set("react.last_tool_result", map[string]interface{}{"error": errMsg})
		result := &core.Result{NodeID: n.id, Success: false, Error: fmt.Errorf("%s", errMsg)}
		state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
		return result, nil
	}
	res, err := tool.Execute(ctx, state, decision.Arguments)
	if err != nil {
		return nil, err
	}
	call := core.ToolCall{
		ID:   NewUUID(),
		Name: decision.Tool,
		Args: decision.Arguments,
	}
	n.recordObservation(state, call, res)
	n.latchVerificationSuccess(state, call.Name, res)
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

func (n *reactActNode) latchVerificationSuccess(state *core.Context, toolName string, res *core.ToolResult) {
	if state == nil || n == nil || n.agent == nil || n.task == nil || res == nil || !res.Success {
		return
	}
	if !taskNeedsEditing(n.task) || !verificationStopAllowed(n.agent, n.task) || !hasEditObservation(state) {
		return
	}
	if !verificationToolMatches(toolName, n.agent.verificationSuccessTools(n.task)) {
		return
	}
	summary := verificationSuccessSummary(toolName, fmt.Sprint(res.Data["stdout"]))
	state.Set("react.verification_latched_summary", summary)
	state.Set("react.synthetic_summary", summary)
	state.Set("react.incomplete_reason", "")
}

func (n *reactActNode) lookupTool(name string, active map[string]struct{}) (core.Tool, bool) {
	if len(active) > 0 {
		if _, ok := active[name]; !ok {
			return nil, false
		}
	}
	return n.agent.Tools.Get(name)
}

func (n *reactActNode) recordObservation(state *core.Context, call core.ToolCall, res *core.ToolResult) {
	appendToolMessage(state, call, res)
	observation := summarizeToolResult(state, call, res)
	history := getToolObservations(state)
	history = append(history, observation)
	limit := toolSummaryBudgetForPhase(state.GetString("react.phase"))
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	state.Set("react.tool_observations", history)
	if n.agent.contextPolicy != nil && n.agent.contextPolicy.ContextManager != nil {
		item := &core.ToolResultContextItem{
			ToolName:     call.Name,
			Result:       &core.ToolResult{Success: res.Success, Data: map[string]interface{}{"summary": observation.Summary}, Error: res.Error},
			LastAccessed: time.Now().UTC(),
			Relevance:    0.9,
			PriorityVal:  1,
		}
		_ = n.agent.contextPolicy.ContextManager.AddItem(item)
		if call.Name == "file_read" {
			path := fmt.Sprint(call.Args["path"])
			snippet := observation.Data["snippet"]
			if path != "" && fmt.Sprint(snippet) != "" {
				_ = n.agent.contextPolicy.ContextManager.UpsertFileItem(&core.FileContextItem{
					Path:         path,
					Content:      fmt.Sprint(snippet),
					Summary:      fmt.Sprint(snippet),
					LastAccessed: time.Now().UTC(),
					Relevance:    1.0,
					PriorityVal:  0,
				})
			}
		}
	}
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
	if summary := strings.TrimSpace(state.GetString("react.verification_latched_summary")); summary != "" {
		state.Set("react.done", true)
		state.Set("react.incomplete_reason", "")
		state.Set("react.final_output", map[string]interface{}{
			"summary": summary,
			"result":  lastMap,
		})
		result := &core.Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]interface{}{
				"diagnostic": "Conclusion: " + summary,
				"complete":   true,
			},
		}
		state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
		return result, nil
	}
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
	n.advancePhase(state, decision, lastMap)
	if n.scheduleRecoveryProbe(state, lastMap) {
		state.Set("react.done", false)
		result := &core.Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]interface{}{
				"diagnostic": diagnostic.String(),
				"complete":   false,
			},
		}
		state.SetHandleScoped("react.last_result", result, reactTaskScope(state))
		return result, nil
	}
	if verificationSummary, ok := verificationSummaryFromSuccess(n.agent, n.task, state, lastMap); ok {
		completed := true
		diagnostic.WriteString("Conclusion: " + verificationSummary + "\n")
		state.Set("react.synthetic_summary", verificationSummary)
		state.Set("react.incomplete_reason", "")
		state.Set("react.done", completed)
		state.Set("react.final_output", map[string]interface{}{
			"summary": verificationSummary,
			"result":  lastMap,
		})
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
	if editSummary, ok := editSummaryFromSuccess(n.task, state, lastMap); ok {
		completed := true
		diagnostic.WriteString("Conclusion: " + editSummary + "\n")
		state.Set("react.synthetic_summary", editSummary)
		state.Set("react.incomplete_reason", "")
		state.Set("react.done", completed)
		state.Set("react.final_output", map[string]interface{}{
			"summary": editSummary,
			"result":  lastMap,
		})
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
	if readOnlySummary, ok := readOnlySummaryFromState(n.task, state, lastMap); ok {
		completed := true
		diagnostic.WriteString("Conclusion: " + readOnlySummary + "\n")
		state.Set("react.synthetic_summary", readOnlySummary)
		state.Set("react.incomplete_reason", "")
		state.Set("react.done", completed)
		state.Set("react.final_output", map[string]interface{}{
			"summary": readOnlySummary,
			"result":  lastMap,
		})
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
	if analysisSummary, ok := analysisSummaryFromFailure(n.task, state, lastMap); ok {
		completed := true
		diagnostic.WriteString("Conclusion: " + analysisSummary + "\n")
		state.Set("react.synthetic_summary", analysisSummary)
		state.Set("react.incomplete_reason", "")
		state.Set("react.done", completed)
		state.Set("react.final_output", map[string]interface{}{
			"summary": analysisSummary,
			"result":  lastMap,
		})
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
	repeated, repeatReason := detectRepeatedToolLoop(state)
	completed := decision.Complete
	if res, ok := state.Get("react.tool_calls"); ok {
		if calls, ok := res.([]core.ToolCall); ok && len(calls) > 0 {
			completed = false
		}
	}
	if repeated {
		if successSummary, ok := completionSummaryFromState(n.agent, n.task, state, lastMap); ok {
			completed = true
			diagnostic.WriteString("Conclusion: " + successSummary + "\n")
			state.Set("react.synthetic_summary", successSummary)
			state.Set("react.incomplete_reason", "")
		} else if analysisSummary, ok := repeatedFailureAnalysis(n.task, state, lastMap); ok {
			completed = true
			diagnostic.WriteString("Conclusion: " + analysisSummary + "\n")
			state.Set("react.synthetic_summary", analysisSummary)
			state.Set("react.incomplete_reason", "")
		} else {
			completed = true
			state.Set("react.incomplete_reason", repeatReason)
		}
	}
	if !completed && iter >= n.agent.maxIterations {
		if successSummary, ok := completionSummaryFromState(n.agent, n.task, state, lastMap); ok {
			completed = true
			diagnostic.WriteString("Conclusion: " + successSummary + "\n")
			state.Set("react.synthetic_summary", successSummary)
			state.Set("react.incomplete_reason", "")
		} else {
			completed = true
			state.Set("react.incomplete_reason", iterationExhaustionReason(n.task, state))
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
		summary := diagnostic.String()
		if synthetic := strings.TrimSpace(state.GetString("react.synthetic_summary")); synthetic != "" {
			summary = synthetic
			state.Set("react.incomplete_reason", "")
		}
		state.Set("react.final_output", map[string]interface{}{
			"summary": summary,
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

func (n *reactObserveNode) scheduleRecoveryProbe(state *core.Context, lastMap map[string]interface{}) bool {
	if state == nil || taskNeedsEditing(n.task) || !hasFailure(lastMap) {
		return false
	}
	if pending, ok := state.Get("react.tool_calls"); ok {
		if calls, ok := pending.([]core.ToolCall); ok && len(calls) > 0 {
			return false
		}
	}
	probes := n.agent.recoveryProbeTools(n.task)
	if len(probes) == 0 {
		return false
	}
	signature := failureSignature(lastMap)
	if signature == "" {
		return false
	}
	used := recoveryProbesForSignature(state, signature)
	for _, probe := range probes {
		probe = strings.TrimSpace(probe)
		if probe == "" || used[probe] {
			continue
		}
		args := recoveryProbeArgs(n.agent, probe, state, n.task, lastMap)
		if args == nil {
			continue
		}
		state.Set("react.tool_calls", []core.ToolCall{{Name: probe, Args: args}})
		recordRecoveryProbeUsage(state, signature, probe)
		return true
	}
	return false
}

func (n *reactObserveNode) advancePhase(state *core.Context, decision decisionPayload, lastMap map[string]interface{}) {
	if state == nil {
		return
	}
	current := state.GetString("react.phase")
	if current == "" {
		current = contextmgrPhaseExplore
	}
	observations := getToolObservations(state)
	lastTool := ""
	if len(observations) > 0 {
		lastTool = observations[len(observations)-1].Tool
	}
	if current == contextmgrPhaseVerify && taskNeedsEditing(n.task) && hasFailureFromState(state) {
		if !strings.Contains(lastTool, "test") &&
			!strings.Contains(lastTool, "build") &&
			!strings.Contains(lastTool, "lint") &&
			!strings.Contains(lastTool, "rustfmt") {
			state.Set("react.phase", contextmgrPhaseEdit)
			return
		}
	}
	switch {
	case strings.Contains(lastTool, "write") || strings.Contains(lastTool, "create") || strings.Contains(lastTool, "delete"):
		state.Set("react.phase", contextmgrPhaseVerify)
	case strings.Contains(lastTool, "test") || strings.Contains(lastTool, "build") || strings.Contains(lastTool, "lint") || strings.Contains(lastTool, "rustfmt"):
		if hasFailure(lastMap) {
			state.Set("react.phase", contextmgrPhaseEdit)
		} else {
			state.Set("react.phase", contextmgrPhaseVerify)
		}
	case current == contextmgrPhaseExplore && lastTool != "":
		if shouldEnterEditPhase(n.task, observations, lastTool, lastMap) {
			state.Set("react.phase", contextmgrPhaseEdit)
		}
	default:
		_ = decision
	}
}

func shouldEnterEditPhase(task *core.Task, observations []ToolObservation, lastTool string, lastMap map[string]interface{}) bool {
	if !taskNeedsEditing(task) {
		return false
	}
	if strings.Contains(lastTool, "test") || strings.Contains(lastTool, "build") || strings.Contains(lastTool, "lint") {
		return hasFailure(lastMap)
	}
	if len(observations) < 2 {
		return false
	}
	return strings.HasPrefix(lastTool, "file_") ||
		strings.HasPrefix(lastTool, "ast_") ||
		strings.HasPrefix(lastTool, "lsp_") ||
		strings.Contains(lastTool, "grep")
}

func hasFailure(lastMap map[string]interface{}) bool {
	return valueIndicatesFailure(lastMap)
}

func valueIndicatesFailure(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return false
	case map[string]interface{}:
		if success, ok := v["success"].(bool); ok && !success {
			return true
		}
		if errText := strings.TrimSpace(fmt.Sprint(v["error"])); errText != "" && errText != "<nil>" {
			return true
		}
		for key, inner := range v {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if lowerKey == "success" || lowerKey == "error" {
				continue
			}
			if valueIndicatesFailure(inner) {
				return true
			}
		}
		return false
	case []interface{}:
		for _, item := range v {
			if valueIndicatesFailure(item) {
				return true
			}
		}
		return false
	case []string:
		for _, item := range v {
			if valueIndicatesFailure(item) {
				return true
			}
		}
		return false
	case string:
		text := strings.ToLower(strings.TrimSpace(v))
		if text == "" {
			return false
		}
		return strings.Contains(text, "failed") ||
			strings.Contains(text, "panic") ||
			strings.Contains(text, "assertion") ||
			strings.Contains(text, "syntaxerror") ||
			strings.Contains(text, "traceback")
	default:
		text := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
		if text == "" || text == "<nil>" {
			return false
		}
		return strings.Contains(text, "failed") ||
			strings.Contains(text, "panic") ||
			strings.Contains(text, "assertion") ||
			strings.Contains(text, "syntaxerror") ||
			strings.Contains(text, "traceback")
	}
}

func hasFailureFromState(state *core.Context) bool {
	if state == nil {
		return false
	}
	raw, _ := state.Get("react.last_tool_result")
	lastMap, _ := raw.(map[string]interface{})
	return hasFailure(lastMap)
}

func taskInstructionText(task *core.Task) string {
	if task == nil {
		return ""
	}
	if task.Context != nil {
		if raw, ok := task.Context["user_instruction"]; ok {
			if text := strings.TrimSpace(fmt.Sprint(raw)); text != "" {
				return strings.ToLower(text)
			}
		}
	}
	return strings.ToLower(task.Instruction)
}

func taskNeedsEditing(task *core.Task) bool {
	if task == nil {
		return false
	}
	text := taskInstructionText(task)
	negativeMarkers := []string{
		"do not modify",
		"don't modify",
		"do not edit",
		"don't edit",
		"without edits",
		"no file changes",
		"without modifying",
	}
	for _, marker := range negativeMarkers {
		if strings.Contains(text, marker) {
			return false
		}
	}
	editPattern := regexp.MustCompile(`\b(implement|fix|modify|edit|write|refactor|update|create|append|add)\b`)
	if editPattern.MatchString(text) {
		return true
	}
	return false
}

func detectRepeatedToolLoop(state *core.Context) (bool, string) {
	observations := getToolObservations(state)
	if len(observations) == 0 {
		return false, ""
	}
	current := observationSignature(observations[len(observations)-1])
	count := 1
	for i := len(observations) - 2; i >= 0; i-- {
		if observationSignature(observations[i]) != current {
			break
		}
		count++
	}
	state.Set("react.repeat_signature", current)
	state.Set("react.repeat_count", count)
	if count < 3 {
		return false, ""
	}
	last := observations[len(observations)-1]
	return true, fmt.Sprintf("stuck repeating %s with the same inputs/results", last.Tool)
}

func repeatedReadTarget(state *core.Context) string {
	observations := getToolObservations(state)
	if len(observations) < 2 {
		return ""
	}
	last := observations[len(observations)-1]
	prev := observations[len(observations)-2]
	if !last.Success || !prev.Success {
		return ""
	}
	if last.Tool != "file_read" || prev.Tool != "file_read" {
		return ""
	}
	lastPath := strings.TrimSpace(fmt.Sprint(last.Args["path"]))
	prevPath := strings.TrimSpace(fmt.Sprint(prev.Args["path"]))
	if lastPath == "" || lastPath != prevPath {
		return ""
	}
	return lastPath
}

func observationSignature(observation ToolObservation) string {
	args, _ := json.Marshal(observation.Args)
	data, _ := json.Marshal(observation.Data)
	return fmt.Sprintf("%s|%s|%s|%t", observation.Tool, string(args), string(data), observation.Success)
}

func iterationExhaustionReason(task *core.Task, state *core.Context) string {
	if taskNeedsEditing(task) && !hasEditObservation(state) {
		return "iteration budget exhausted before making any file changes"
	}
	return "iteration budget exhausted before task completion"
}

func hasEditObservation(state *core.Context) bool {
	for _, observation := range getToolObservations(state) {
		name := observation.Tool
		if strings.Contains(name, "write") || strings.Contains(name, "create") || strings.Contains(name, "delete") {
			return true
		}
	}
	return false
}

func repeatedFailureAnalysis(task *core.Task, state *core.Context, lastMap map[string]interface{}) (string, bool) {
	if taskNeedsEditing(task) || !hasFailure(lastMap) {
		return "", false
	}
	observations := getToolObservations(state)
	if len(observations) == 0 {
		return "", false
	}
	last := observations[len(observations)-1]
	if !strings.Contains(strings.ToLower(last.Tool), "cargo") &&
		!strings.Contains(strings.ToLower(last.Tool), "test") &&
		!strings.Contains(strings.ToLower(last.Tool), "build") {
		return "", false
	}
	reason := strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(last.Data["stderr"])))
	if reason == "" {
		reason = strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(last.Data["stdout"])))
	}
	if reason == "" {
		reason = strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(last.Data["error"])))
	}
	if reason == "" {
		return "", false
	}
	return fmt.Sprintf("%s failed repeatedly: %s", last.Tool, reason), true
}

func analysisSummaryFromFailure(task *core.Task, state *core.Context, lastMap map[string]interface{}) (string, bool) {
	if taskNeedsEditing(task) || !hasFailure(lastMap) {
		return "", false
	}
	observations := getToolObservations(state)
	if len(observations) == 0 {
		return "", false
	}
	last := observations[len(observations)-1]
	toolName := strings.ToLower(last.Tool)
	if !strings.Contains(toolName, "cargo") &&
		!strings.Contains(toolName, "test") &&
		!strings.Contains(toolName, "build") {
		return "", false
	}
	reason := strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(last.Data["stderr"])))
	if reason == "" {
		reason = strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(last.Data["stdout"])))
	}
	if reason == "" {
		reason = strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(last.Data["error"])))
	}
	if reason == "" {
		return "", false
	}
	return fmt.Sprintf("%s failed: %s", last.Tool, reason), true
}

func verificationSummaryFromSuccess(agent *ReActAgent, task *core.Task, state *core.Context, lastMap map[string]interface{}) (string, bool) {
	if !taskNeedsEditing(task) || hasFailure(lastMap) || !hasEditObservation(state) {
		return "", false
	}
	if !verificationStopAllowed(agent, task) {
		return "", false
	}
	observations := getToolObservations(state)
	if len(observations) == 0 {
		return "", false
	}
	var successTools []string
	if agent != nil {
		successTools = agent.verificationSuccessTools(task)
	}
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		toolName := strings.ToLower(observation.Tool)
		if strings.Contains(toolName, "write") || strings.Contains(toolName, "create") || strings.Contains(toolName, "delete") {
			return "", false
		}
		if !observation.Success {
			continue
		}
		if verificationToolMatches(observation.Tool, successTools) {
			return verificationSuccessSummary(observation.Tool, fmt.Sprint(observation.Data["stdout"])), true
		}
	}
	return "", false
}

func verificationSummaryWithoutEdits(agent *ReActAgent, task *core.Task, state *core.Context, lastMap map[string]interface{}) (string, bool) {
	if taskNeedsEditing(task) || hasFailure(lastMap) || !taskRequiresVerification(task) {
		return "", false
	}
	observations := getToolObservations(state)
	if len(observations) == 0 {
		return "", false
	}
	var successTools []string
	if agent != nil {
		successTools = agent.verificationSuccessTools(task)
	}
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		if !observation.Success {
			continue
		}
		if verificationToolMatches(observation.Tool, successTools) {
			return verificationNoEditSummary(observation.Tool, fmt.Sprint(observation.Data["stdout"]), fmt.Sprint(observation.Data["stderr"])), true
		}
	}
	return "", false
}

func completionSummaryFromState(agent *ReActAgent, task *core.Task, state *core.Context, lastMap map[string]interface{}) (string, bool) {
	if summary, ok := verificationSummaryFromSuccess(agent, task, state, lastMap); ok {
		return summary, true
	}
	if summary, ok := verificationSummaryWithoutEdits(agent, task, state, lastMap); ok {
		return summary, true
	}
	if summary, ok := editSummaryFromSuccess(task, state, lastMap); ok {
		return summary, true
	}
	if summary, ok := readOnlySummaryFromState(task, state, lastMap); ok {
		return summary, true
	}
	if summary, ok := directCompletionSummary(task, state); ok {
		return summary, true
	}
	if summary, ok := repeatedReadCompletionSummary(task, state); ok {
		return summary, true
	}
	return "", false
}

func directCompletionSummary(task *core.Task, state *core.Context) (string, bool) {
	observations := getToolObservations(state)
	if len(observations) == 0 || task == nil {
		return "", false
	}
	if !taskNeedsEditing(task) && taskLooksLikeReadOnlySummary(task) {
		for i := len(observations) - 1; i >= 0; i-- {
			observation := observations[i]
			if !observation.Success || observation.Tool != "file_read" {
				continue
			}
			path := strings.TrimSpace(fmt.Sprint(observation.Args["path"]))
			snippet := strings.TrimSpace(fmt.Sprint(observation.Data["snippet"]))
			if snippet == "" {
				snippet = strings.TrimSpace(fmt.Sprint(observation.Summary))
			}
			if snippet == "" {
				continue
			}
			snippet = truncateForPrompt(strings.ReplaceAll(snippet, "\n", " "), 220)
			if path != "" {
				return fmt.Sprintf("Summary of %s: %s", path, snippet), true
			}
			return snippet, true
		}
	}
	if taskNeedsEditing(task) && hasEditObservation(state) && !taskRequiresVerification(task) {
		for i := len(observations) - 1; i >= 0; i-- {
			observation := observations[i]
			if !observation.Success {
				continue
			}
			toolName := strings.ToLower(observation.Tool)
			if strings.Contains(toolName, "write") || strings.Contains(toolName, "create") || strings.Contains(toolName, "delete") {
				return fmt.Sprintf("%s applied the requested changes", observation.Tool), true
			}
		}
	}
	return "", false
}

func repeatedReadCompletionSummary(task *core.Task, state *core.Context) (string, bool) {
	observations := getToolObservations(state)
	if len(observations) < 3 {
		return "", false
	}
	last := observations[len(observations)-1]
	if !last.Success || last.Tool != "file_read" {
		return "", false
	}
	signature := observationSignature(last)
	repeatCount := 1
	for i := len(observations) - 2; i >= 0; i-- {
		if observationSignature(observations[i]) != signature {
			break
		}
		repeatCount++
	}
	if repeatCount < 3 {
		return "", false
	}
	if !taskNeedsEditing(task) && taskLooksLikeReadOnlySummary(task) {
		path := strings.TrimSpace(fmt.Sprint(last.Args["path"]))
		snippet := strings.TrimSpace(fmt.Sprint(last.Data["snippet"]))
		if snippet == "" {
			snippet = strings.TrimSpace(fmt.Sprint(last.Summary))
		}
		if snippet == "" {
			return "", false
		}
		snippet = truncateForPrompt(strings.ReplaceAll(snippet, "\n", " "), 220)
		if path != "" {
			return fmt.Sprintf("Summary of %s: %s", path, snippet), true
		}
		return snippet, true
	}
	if taskNeedsEditing(task) && hasEditObservation(state) && !taskRequiresVerification(task) {
		return fmt.Sprintf("%s confirmed the requested changes", last.Tool), true
	}
	return "", false
}

func finalResultFallbackSummary(task *core.Task, state *core.Context) (string, bool) {
	observations := getToolObservations(state)
	if len(observations) == 0 || task == nil {
		return "", false
	}
	if !taskNeedsEditing(task) && taskLooksLikeReadOnlySummary(task) {
		for i := len(observations) - 1; i >= 0; i-- {
			observation := observations[i]
			if !observation.Success || observation.Tool != "file_read" {
				continue
			}
			path := strings.TrimSpace(fmt.Sprint(observation.Args["path"]))
			snippet := strings.TrimSpace(fmt.Sprint(observation.Data["snippet"]))
			if snippet == "" {
				snippet = strings.TrimSpace(fmt.Sprint(observation.Summary))
			}
			if snippet == "" {
				continue
			}
			snippet = truncateForPrompt(strings.ReplaceAll(snippet, "\n", " "), 220)
			if path != "" {
				return fmt.Sprintf("Summary of %s: %s", path, snippet), true
			}
			return snippet, true
		}
	}
	if taskNeedsEditing(task) && hasEditObservation(state) && !taskRequiresVerification(task) {
		for i := len(observations) - 1; i >= 0; i-- {
			observation := observations[i]
			if !observation.Success {
				continue
			}
			toolName := strings.ToLower(observation.Tool)
			if strings.Contains(toolName, "write") || strings.Contains(toolName, "create") || strings.Contains(toolName, "delete") {
				return fmt.Sprintf("%s applied the requested changes", observation.Tool), true
			}
		}
	}
	return "", false
}

func editSummaryFromSuccess(task *core.Task, state *core.Context, lastMap map[string]interface{}) (string, bool) {
	if !taskNeedsEditing(task) || hasFailure(lastMap) || !hasEditObservation(state) {
		return "", false
	}
	if taskRequiresVerification(task) {
		return "", false
	}
	observations := getToolObservations(state)
	if len(observations) == 0 {
		return "", false
	}
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		if !observation.Success {
			continue
		}
		toolName := strings.ToLower(observation.Tool)
		if strings.Contains(toolName, "write") || strings.Contains(toolName, "create") || strings.Contains(toolName, "delete") {
			return fmt.Sprintf("%s applied the requested changes", observation.Tool), true
		}
	}
	return "", false
}

func readOnlySummaryFromState(task *core.Task, state *core.Context, lastMap map[string]interface{}) (string, bool) {
	if task == nil || taskNeedsEditing(task) || hasFailure(lastMap) {
		return "", false
	}
	if !taskLooksLikeReadOnlySummary(task) {
		return "", false
	}
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		if !observation.Success {
			continue
		}
		if observation.Tool == "file_read" {
			path := strings.TrimSpace(fmt.Sprint(observation.Args["path"]))
			snippet := strings.TrimSpace(fmt.Sprint(observation.Data["snippet"]))
			if snippet == "" {
				snippet = strings.TrimSpace(fmt.Sprint(observation.Data["summary"]))
			}
			if snippet == "" {
				continue
			}
			snippet = truncateForPrompt(strings.ReplaceAll(snippet, "\n", " "), 220)
			if path != "" {
				return fmt.Sprintf("Summary of %s: %s", path, snippet), true
			}
			return snippet, true
		}
		if summary := strings.TrimSpace(observation.Summary); summary != "" {
			return summary, true
		}
	}
	return "", false
}

func taskRequiresVerification(task *core.Task) bool {
	if task == nil {
		return false
	}
	text := taskInstructionText(task)
	phrases := []string{
		"run tests",
		"run the tests",
		"run test",
		"run cli_",
		"verify",
		"confirm",
		"compile",
		"build",
		"lint",
		"cargo test",
		"cargo check",
		"cargo build",
		"go test",
		"pytest",
		"unittest",
	}
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func taskLooksLikeReadOnlySummary(task *core.Task) bool {
	if task == nil {
		return false
	}
	text := taskInstructionText(task)
	markers := []string{"summarize", "summary", "explain", "describe"}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func verificationToolMatches(toolName string, configured []string) bool {
	if len(configured) > 0 {
		for _, tool := range configured {
			if strings.EqualFold(strings.TrimSpace(tool), toolName) {
				return true
			}
		}
		return false
	}
	lower := strings.ToLower(toolName)
	return strings.Contains(lower, "cargo") ||
		strings.Contains(lower, "test") ||
		strings.Contains(lower, "build") ||
		strings.HasPrefix(lower, "cli_go") ||
		strings.HasPrefix(lower, "cli_python") ||
		strings.HasPrefix(lower, "cli_node") ||
		strings.HasPrefix(lower, "cli_sqlite")
}

func verificationSuccessSummary(toolName, stdout string) string {
	stdout = strings.TrimSpace(stdout)
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if strings.Contains(lower, "sqlite") && stdout != "" {
		return stdout
	}
	return fmt.Sprintf("%s succeeded after applying changes", toolName)
}

func verificationNoEditSummary(toolName, stdout, stderr string) string {
	output := strings.TrimSpace(stdout)
	if output == "" {
		output = strings.TrimSpace(stderr)
	}
	if output != "" {
		return truncateForPrompt(output, 220)
	}
	return fmt.Sprintf("%s verification passed", toolName)
}

func skillStopOnSuccess(task *core.Task) bool {
	spec := agentSpecFromTask(task)
	if spec == nil {
		return false
	}
	return spec.SkillConfig.Verification.StopOnSuccess
}

func verificationStopAllowed(agent *ReActAgent, task *core.Task) bool {
	if skillStopOnSuccess(task) {
		return true
	}
	if taskRequiresVerification(task) {
		return true
	}
	if agent == nil {
		return false
	}
	return len(agent.verificationSuccessTools(task)) == 0
}

func verificationLikeTool(tool core.Tool) bool {
	if tool == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(tool.Name()))
	if strings.Contains(name, "test") || strings.Contains(name, "build") || strings.Contains(name, "lint") || strings.Contains(name, "check") || strings.Contains(name, "cargo") || strings.Contains(name, "fmt") || strings.Contains(name, "format") {
		return true
	}
	for _, tag := range tool.Tags() {
		lower := strings.ToLower(strings.TrimSpace(tag))
		if lower == "verify" || lower == "test" || lower == "build" || lower == "lint" || lower == "syntax-check" {
			return true
		}
	}
	return false
}

func explicitlyRequestedToolNames(task *core.Task) map[string]struct{} {
	out := map[string]struct{}{}
	if task == nil {
		return out
	}
	matches := regexp.MustCompile(`\b(?:cli|rust|go|python|node|sqlite)_[a-z0-9_]+\b`).FindAllString(strings.ToLower(task.Instruction), -1)
	for _, match := range matches {
		out[match] = struct{}{}
	}
	return out
}

func recoveryProbeArgs(agent *ReActAgent, toolName string, state *core.Context, task *core.Task, lastMap map[string]interface{}) map[string]interface{} {
	if agent == nil || agent.Tools == nil {
		return nil
	}
	tool, ok := agent.Tools.Get(toolName)
	if !ok || tool == nil {
		return nil
	}
	switch toolName {
	case "file_read":
		if path := primaryFailurePath(state, lastMap); path != "" {
			return map[string]interface{}{"path": path}
		}
		return nil
	case "search_grep", "file_search":
		pattern := primaryFailureSearchPattern(lastMap)
		if pattern == "" {
			return nil
		}
		return map[string]interface{}{
			"directory": primaryFailureDirectory(state, lastMap),
			"pattern":   pattern,
		}
	case "query_ast":
		if symbol := inferFailureSymbol(lastMap); symbol != "" {
			return map[string]interface{}{"action": "get_signature", "symbol": symbol}
		}
		return map[string]interface{}{"action": "list_symbols", "category": "function"}
	}

	args := make(map[string]interface{})
	params := tool.Parameters()
	required := map[string]bool{}
	for _, param := range params {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			continue
		}
		required[name] = param.Required
		switch name {
		case "working_directory":
			args[name] = primaryFailureDirectory(state, lastMap)
		case "path":
			path := primaryFailurePath(state, lastMap)
			if path == "" {
				path = "."
			}
			args[name] = path
		case "database_path":
			if db := inferredPathFromObservations(state, "database_path"); db != "" {
				args[name] = db
			} else if path := primaryFailurePath(state, lastMap); isSQLiteFailurePath(path) {
				args[name] = path
			}
		case "query":
			if strings.Contains(strings.ToLower(tool.Name()), "sqlite") {
				args[name] = "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name LIMIT 20;"
			}
		}
	}
	for name, need := range required {
		if !need {
			continue
		}
		if _, ok := args[name]; ok {
			continue
		}
		_ = task
		return nil
	}
	if len(args) == 0 {
		return nil
	}
	return args
}

func failureSignature(lastMap map[string]interface{}) string {
	return strings.TrimSpace(fmt.Sprint(lastMap))
}

func recoveryProbesForSignature(state *core.Context, signature string) map[string]bool {
	out := map[string]bool{}
	if state == nil || signature == "" {
		return out
	}
	raw, ok := state.Get("react.recovery_probes")
	if !ok || raw == nil {
		return out
	}
	store, ok := raw.(map[string][]string)
	if !ok {
		return out
	}
	for _, name := range store[signature] {
		out[name] = true
	}
	return out
}

func recordRecoveryProbeUsage(state *core.Context, signature, toolName string) {
	if state == nil || signature == "" || toolName == "" {
		return
	}
	store := map[string][]string{}
	if raw, ok := state.Get("react.recovery_probes"); ok && raw != nil {
		if current, ok := raw.(map[string][]string); ok {
			for k, v := range current {
				store[k] = append([]string{}, v...)
			}
		}
	}
	store[signature] = append(store[signature], toolName)
	state.Set("react.recovery_probes", store)
}

func primaryFailureDirectory(state *core.Context, lastMap map[string]interface{}) string {
	if task := state.GetString("react.failure_workdir"); task != "" {
		return task
	}
	if path := primaryFailurePath(state, lastMap); path != "" {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
		return filepath.Dir(path)
	}
	return "."
}

func primaryFailurePath(state *core.Context, lastMap map[string]interface{}) string {
	if state != nil {
		if path := strings.TrimSpace(state.GetString("react.failure_path")); path != "" {
			return path
		}
	}
	if path := inferredPathFromObservations(state, "database_path", "manifest_path", "module_path", "workspace_path", "go_mod"); path != "" {
		return path
	}
	_ = lastMap
	return ""
}

func inferredPathFromObservations(state *core.Context, keys ...string) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		for _, key := range keys {
			if value := strings.TrimSpace(fmt.Sprint(obs.Data[key])); value != "" && value != "<nil>" {
				return value
			}
		}
		if obs.Tool == "file_read" {
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if path != "" && path != "<nil>" {
				for _, key := range keys {
					switch key {
					case "database_path":
						if isSQLiteFailurePath(path) {
							return path
						}
					case "manifest_path", "module_path", "workspace_path", "go_mod":
						if strings.HasSuffix(path, ".toml") || strings.HasSuffix(path, ".mod") || strings.HasSuffix(path, ".work") || strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".cfg") || strings.HasSuffix(path, ".txt") || strings.HasSuffix(path, "Cargo.toml") {
							return path
						}
					}
				}
			}
		}
	}
	return ""
}

func inferredCargoManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.Tool == "rust_workspace_detect" {
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["manifest_path"])); manifest != "" {
				return manifest
			}
		}
		if obs.Tool == "file_read" {
			if path := strings.TrimSpace(fmt.Sprint(obs.Args["path"])); strings.HasSuffix(path, "Cargo.toml") {
				return path
			}
		}
	}
	return ""
}

func inferredPythonManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "python_workspace_detect", "python_project_metadata":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["manifest_path"])); manifest != "" {
				return manifest
			}
		case "file_read":
			if path := strings.TrimSpace(fmt.Sprint(obs.Args["path"])); strings.HasSuffix(path, "pyproject.toml") || strings.HasSuffix(path, "setup.py") || strings.HasSuffix(path, "setup.cfg") || strings.HasSuffix(path, "requirements.txt") {
				return path
			}
		}
	}
	return ""
}

func inferredNodeManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "node_workspace_detect", "node_project_metadata":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["manifest_path"])); manifest != "" {
				return manifest
			}
		case "file_read":
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if strings.HasSuffix(path, "package.json") ||
				strings.HasSuffix(path, "package-lock.json") ||
				strings.HasSuffix(path, "pnpm-lock.yaml") ||
				strings.HasSuffix(path, "yarn.lock") ||
				strings.HasSuffix(path, "tsconfig.json") {
				return path
			}
		}
	}
	return ""
}

func inferredGoManifest(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "go_workspace_detect":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["module_path"])); manifest != "" {
				return manifest
			}
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["workspace_path"])); manifest != "" {
				return manifest
			}
		case "go_module_metadata":
			if manifest := strings.TrimSpace(fmt.Sprint(obs.Data["go_mod"])); manifest != "" {
				return manifest
			}
		case "file_read":
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if strings.HasSuffix(path, "go.mod") || strings.HasSuffix(path, "go.work") {
				return path
			}
		}
	}
	return ""
}

func inferredSQLiteDatabase(state *core.Context) string {
	observations := getToolObservations(state)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		switch obs.Tool {
		case "sqlite_database_detect":
			if db := strings.TrimSpace(fmt.Sprint(obs.Data["database_path"])); db != "" {
				return db
			}
		case "sqlite_query", "sqlite_schema_inspect", "sqlite_integrity_check":
			if db := strings.TrimSpace(fmt.Sprint(obs.Data["database"])); db != "" {
				return db
			}
		case "file_read":
			path := strings.TrimSpace(fmt.Sprint(obs.Args["path"]))
			if isSQLiteFailurePath(path) {
				return path
			}
		}
	}
	return ""
}

func isSQLiteFailurePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite3")
}

func primaryFailureSearchPattern(lastMap map[string]interface{}) string {
	text := strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(lastMap)))
	if text == "" {
		return ""
	}
	return text
}

var rustSymbolPattern = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_:]*)`)

func inferFailureSymbol(lastMap map[string]interface{}) string {
	text := fmt.Sprint(lastMap)
	matches := rustSymbolPattern.FindAllString(text, -1)
	for _, match := range matches {
		lower := strings.ToLower(match)
		if lower == "error" || lower == "warning" || lower == "failed" || lower == "cargo" {
			continue
		}
		return match
	}
	return ""
}

func agentSpecFromTask(task *core.Task) *core.AgentRuntimeSpec {
	return toolsys.EffectiveAgentSpec(task, nil)
}

// decisionPayload models the JSON output of the think step.
type decisionPayload struct {
	Thought   string                 `json:"thought"`
	Action    string                 `json:"action"`
	Tool      string                 `json:"tool"`
	Arguments map[string]interface{} `json:"arguments"`
	Complete  bool                   `json:"complete"`
	Reason    string                 `json:"reason"`
	Summary   string                 `json:"summary"`
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
	if action, ok := generic["action"].(string); ok {
		payload.Action = action
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
	if summary, ok := generic["summary"].(string); ok {
		payload.Summary = summary
	}
	if payload.Action == "complete" {
		payload.Complete = true
	}
	if payload.Action == "tool" && payload.Tool != "" {
		payload.Complete = false
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

func appendAssistantMessage(state *core.Context, resp *core.LLMResponse) {
	if state == nil || resp == nil {
		return
	}
	messages := getReactMessages(state)
	if len(messages) == 0 {
		return
	}
	messages = append(messages, core.Message{
		Role:      "assistant",
		Content:   resp.Text,
		ToolCalls: append([]core.ToolCall{}, resp.ToolCalls...),
	})
	saveReactMessages(state, messages)
}

// appendToolMessage records tool responses in the transcript so the LLM can
// observe prior results when tool calling is used.
func appendToolMessage(state *core.Context, call core.ToolCall, res *core.ToolResult) {
	messages := getReactMessages(state)
	if len(messages) == 0 || res == nil {
		return
	}
	observation := summarizeToolResult(state, call, res)
	messages = append(messages, core.Message{
		Role:       "tool",
		Name:       call.Name,
		Content:    fmt.Sprintf("success=%t %s", res.Success, observation.Summary),
		ToolCallID: call.ID,
	})
	saveReactMessages(state, messages)
}

func getToolObservations(state *core.Context) []ToolObservation {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("react.tool_observations")
	if !ok || raw == nil {
		return nil
	}
	switch values := raw.(type) {
	case []ToolObservation:
		return append([]ToolObservation{}, values...)
	case []any:
		out := make([]ToolObservation, 0, len(values))
		for _, value := range values {
			encoded, err := json.Marshal(value)
			if err != nil {
				continue
			}
			var observation ToolObservation
			if err := json.Unmarshal(encoded, &observation); err == nil {
				out = append(out, observation)
			}
		}
		return out
	default:
		return nil
	}
}

func summarizeToolResult(state *core.Context, call core.ToolCall, res *core.ToolResult) ToolObservation {
	phase := ""
	if state != nil {
		phase = state.GetString("react.phase")
	}
	observation := ToolObservation{
		Tool:      call.Name,
		Phase:     phase,
		Args:      call.Args,
		Success:   res != nil && res.Success,
		Timestamp: time.Now().UTC(),
	}
	if res == nil {
		observation.Summary = fmt.Sprintf("%s returned no result", call.Name)
		return observation
	}
	summary, data := compactToolData(call, res)
	observation.Summary = summary
	observation.Data = data
	return observation
}

func compactToolData(call core.ToolCall, res *core.ToolResult) (string, map[string]interface{}) {
	if res == nil {
		return fmt.Sprintf("%s returned no result", call.Name), nil
	}
	if res.Error != "" {
		stdout := truncateForPrompt(fmt.Sprint(res.Data["stdout"]), 320)
		stderr := truncateForPrompt(fmt.Sprint(res.Data["stderr"]), 320)
		reason := strings.TrimSpace(firstMeaningfulLine(stderr))
		if reason == "" {
			reason = strings.TrimSpace(firstMeaningfulLine(stdout))
		}
		if reason == "" {
			reason = truncateForPrompt(res.Error, 220)
		}
		return fmt.Sprintf("%s failed: %s", call.Name, reason), map[string]interface{}{
			"error":  truncateForPrompt(res.Error, 220),
			"stdout": stdout,
			"stderr": stderr,
		}
	}
	switch call.Name {
	case "file_read":
		path := fmt.Sprint(call.Args["path"])
		content := fmt.Sprint(res.Data["content"])
		snippet := truncateForPrompt(content, 900)
		return fmt.Sprintf("Read %s", path), map[string]interface{}{"path": path, "snippet": snippet}
	case "file_list":
		files := truncateForPrompt(fmt.Sprint(res.Data["files"]), 220)
		return fmt.Sprintf("Listed files: %s", files), map[string]interface{}{"files": files}
	default:
		stdout := truncateForPrompt(fmt.Sprint(res.Data["stdout"]), 320)
		stderr := truncateForPrompt(fmt.Sprint(res.Data["stderr"]), 320)
		if stdout != "" || stderr != "" {
			summary := strings.TrimSpace(strings.Join([]string{firstMeaningfulLine(stderr), firstMeaningfulLine(stdout)}, " | "))
			if summary == "" {
				summary = truncateForPrompt(fmt.Sprintf("stdout=%s stderr=%s", stdout, stderr), 220)
			}
			return fmt.Sprintf("%s: %s", call.Name, summary), map[string]interface{}{"stdout": stdout, "stderr": stderr}
		}
		if len(res.Data) > 0 {
			summary := truncateForPrompt(fmt.Sprint(res.Data), 220)
			return fmt.Sprintf("%s: %s", call.Name, summary), map[string]interface{}{"summary": summary}
		}
		return fmt.Sprintf("%s completed", call.Name), map[string]interface{}{"summary": fmt.Sprintf("%s completed", call.Name)}
	}
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return truncateForPrompt(line, 180)
	}
	return ""
}

func finalOutputSummary(value interface{}) string {
	switch v := value.(type) {
	case map[string]interface{}:
		return strings.TrimSpace(fmt.Sprint(v["summary"]))
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func reactTaskScope(state *core.Context) string {
	if state == nil {
		return ""
	}
	return state.GetString("task.id")
}

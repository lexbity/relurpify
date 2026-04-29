package react

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

type reactObserveNode struct {
	id    string
	agent *ReActAgent
	task  *core.Task
}

// ID returns the node identifier for the observe step.
func (n *reactObserveNode) ID() string { return n.id }

// Type marks the step as an observation/validation pass.
func (n *reactObserveNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeObservation }

// Execute captures tool output, tracks loop iterations, and determines whether
// the ReAct loop should continue.
func (n *reactObserveNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	env.SetWorkingValue("react.execution_phase", "validating", contextdata.MemoryClassTask)
	iterVal, _ := env.GetWorkingValue("react.iteration")
	iter, _ := iterVal.(int)
	iter++
	env.SetWorkingValue("react.iteration", iter, contextdata.MemoryClassTask)
	decisionVal, _ := env.GetWorkingValue("react.decision")
	decision, _ := decisionVal.(decisionPayload)
	lastRes, _ := env.GetWorkingValue("react.last_tool_result")
	lastMap, _ := lastRes.(map[string]interface{})
	if summary := strings.TrimSpace(envGetString(env, "react.verification_latched_summary")); summary != "" {
		env.SetWorkingValue("react.done", true, contextdata.MemoryClassTask)
		env.SetWorkingValue("react.incomplete_reason", "", contextdata.MemoryClassTask)
		env.SetWorkingValue("react.final_output", map[string]interface{}{
			"summary": summary,
			"result":  lastMap,
		}, contextdata.MemoryClassTask)
		result := &core.Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]interface{}{
				"diagnostic": "Conclusion: " + summary,
				"complete":   true,
			},
		}
		env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
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
	n.advancePhase(env, decision, lastMap)
	if n.scheduleRecoveryProbe(env, lastMap) {
		env.SetWorkingValue("react.done", false, contextdata.MemoryClassTask)
		result := &core.Result{
			NodeID:  n.id,
			Success: true,
			Data: map[string]interface{}{
				"diagnostic": diagnostic.String(),
				"complete":   false,
			},
		}
		env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
		return result, nil
	}
	if summary, ok := verificationSummaryFromSuccess(n.agent, n.task, env, lastMap); ok {
		return n.applyCompletionSummary(env, summary, lastMap, &diagnostic), nil
	}
	if summary, ok := editSummaryFromSuccess(n.task, env, lastMap); ok {
		return n.applyCompletionSummary(env, summary, lastMap, &diagnostic), nil
	}
	if summary, ok := readOnlySummaryFromState(n.task, env, lastMap); ok {
		return n.applyCompletionSummary(env, summary, lastMap, &diagnostic), nil
	}
	if summary, ok := analysisSummaryFromFailure(n.task, env, lastMap); ok {
		return n.applyCompletionSummary(env, summary, lastMap, &diagnostic), nil
	}
	repeated, repeatReason := detectRepeatedToolLoop(env, n.task)
	completed := decision.Complete
	if res, ok := env.GetWorkingValue("react.tool_calls"); ok {
		if calls, ok := res.([]core.ToolCall); ok && len(calls) > 0 {
			completed = false
		}
	}
	if repeated {
		if summary, ok := completionSummaryFromState(n.agent, n.task, env, lastMap); ok {
			completed = true
			setSynthesizedConclusion(env, summary, &diagnostic)
		} else if summary, ok := repeatedFailureAnalysis(n.task, env, lastMap); ok {
			completed = true
			setSynthesizedConclusion(env, summary, &diagnostic)
		} else {
			completed = true
			env.SetWorkingValue("react.incomplete_reason", repeatReason, contextdata.MemoryClassTask)
		}
	}
	if !completed && iter >= n.agent.maxIterations {
		if summary, ok := completionSummaryFromState(n.agent, n.task, env, lastMap); ok {
			completed = true
			setSynthesizedConclusion(env, summary, &diagnostic)
		} else {
			completed = true
			env.SetWorkingValue("react.incomplete_reason", iterationExhaustionReason(n.task, env), contextdata.MemoryClassTask)
		}
	}
	env.SetWorkingValue("react.done", completed, contextdata.MemoryClassTask)

	if n.agent.Memory != nil {
		_ = n.agent.Memory.Remember(ctx, NewUUID(), map[string]interface{}{
			"task":      n.task.Instruction,
			"iteration": iter,
			"decision":  decision,
		}, memory.MemoryScopeSession)
	}

	if completed {
		summary := diagnostic.String()
		if synthetic := strings.TrimSpace(envGetString(env, "react.synthetic_summary")); synthetic != "" {
			summary = synthetic
			env.SetWorkingValue("react.incomplete_reason", "", contextdata.MemoryClassTask)
		}
		env.SetWorkingValue("react.final_output", map[string]interface{}{
			"summary": summary,
			"result":  lastMap,
		}, contextdata.MemoryClassTask)
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
	env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
	return result, nil
}

// applyCompletionSummary records a completion outcome in state and returns a
// terminal Result. Use this for branches where the observe loop should stop
// immediately after detecting a conclusive outcome.
func (n *reactObserveNode) applyCompletionSummary(env *contextdata.Envelope, summary string, lastMap map[string]interface{}, diagnostic *strings.Builder) *core.Result {
	diagnostic.WriteString("Conclusion: " + summary + "\n")
	env.SetWorkingValue("react.synthetic_summary", summary, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.incomplete_reason", "", contextdata.MemoryClassTask)
	env.SetWorkingValue("react.done", true, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.final_output", map[string]interface{}{
		"summary": summary,
		"result":  lastMap,
	}, contextdata.MemoryClassTask)
	result := &core.Result{
		NodeID:  n.id,
		Success: true,
		Data: map[string]interface{}{
			"diagnostic": diagnostic.String(),
			"complete":   true,
		},
	}
	env.SetWorkingValue("react.last_result", result, contextdata.MemoryClassTask)
	return result
}

// setSynthesizedConclusion records a synthesized summary in state without
// returning. Use this when the loop has already been determined complete but
// control should fall through to the shared final-output block.
func setSynthesizedConclusion(env *contextdata.Envelope, summary string, diagnostic *strings.Builder) {
	diagnostic.WriteString("Conclusion: " + summary + "\n")
	env.SetWorkingValue("react.synthetic_summary", summary, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.incomplete_reason", "", contextdata.MemoryClassTask)
}

func (n *reactObserveNode) scheduleRecoveryProbe(env *contextdata.Envelope, lastMap map[string]interface{}) bool {
	if env == nil || taskNeedsEditing(n.task) || !hasFailure(lastMap) {
		return false
	}
	if pending, ok := env.GetWorkingValue("react.tool_calls"); ok {
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
	used := recoveryProbesForSignature(env, signature)
	for _, probe := range probes {
		probe = strings.TrimSpace(probe)
		if probe == "" || used[probe] {
			continue
		}
		args := recoveryProbeArgs(n.agent, probe, env, n.task, lastMap)
		if args == nil {
			continue
		}
		env.SetWorkingValue("react.tool_calls", []core.ToolCall{{Name: probe, Args: args}}, contextdata.MemoryClassTask)
		recordRecoveryProbeUsage(env, signature, probe)
		return true
	}
	return false
}

func (n *reactObserveNode) advancePhase(env *contextdata.Envelope, decision decisionPayload, lastMap map[string]interface{}) {
	if env == nil {
		return
	}
	current := envGetString(env, "react.phase")
	if current == "" {
		current = contextmgrPhaseExplore
	}
	observations := getToolObservations(env)
	lastTool := ""
	if len(observations) > 0 {
		lastTool = observations[len(observations)-1].Tool
	}
	if current == contextmgrPhaseVerify && taskNeedsEditing(n.task) && hasFailureFromState(env) {
		if !strings.Contains(lastTool, "test") &&
			!strings.Contains(lastTool, "build") &&
			!strings.Contains(lastTool, "lint") &&
			!strings.Contains(lastTool, "rustfmt") {
			env.SetWorkingValue("react.phase", contextmgrPhaseEdit, contextdata.MemoryClassTask)
			return
		}
	}
	switch {
	case strings.Contains(lastTool, "write") || strings.Contains(lastTool, "create") || strings.Contains(lastTool, "delete"):
		env.SetWorkingValue("react.phase", contextmgrPhaseVerify, contextdata.MemoryClassTask)
	case strings.Contains(lastTool, "test") || strings.Contains(lastTool, "build") || strings.Contains(lastTool, "lint") || strings.Contains(lastTool, "rustfmt"):
		if hasFailure(lastMap) {
			env.SetWorkingValue("react.phase", contextmgrPhaseEdit, contextdata.MemoryClassTask)
		} else {
			env.SetWorkingValue("react.phase", contextmgrPhaseVerify, contextdata.MemoryClassTask)
		}
	case current == contextmgrPhaseExplore && lastTool != "":
		if shouldEnterEditPhase(n.task, observations, lastTool, lastMap) {
			env.SetWorkingValue("react.phase", contextmgrPhaseEdit, contextdata.MemoryClassTask)
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
	if len(observations) == 0 {
		return false
	}
	lastObservation := observations[len(observations)-1]
	if !lastObservation.Success {
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

func hasFailureFromState(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	raw, _ := env.GetWorkingValue("react.last_tool_result")
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

func detectRepeatedToolLoop(env *contextdata.Envelope, task *core.Task) (bool, string) {
	observations := getToolObservations(env)
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
	env.SetWorkingValue("react.repeat_signature", current, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.repeat_count", count, contextdata.MemoryClassTask)
	if count < stallThresholdForTask(task) {
		return false, ""
	}
	last := observations[len(observations)-1]
	return true, fmt.Sprintf("stuck repeating %s with the same inputs/results", last.Tool)
}

func stallThresholdForTask(task *core.Task) int {
	if task == nil {
		return 3
	}
	if task.Type == core.TaskTypeAnalysis {
		return 6 // analysis tasks legitimately re-read the same files before converging
	}
	return 3
}

func repeatedReadTarget(state *contextdata.Envelope) string {
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

func iterationExhaustionReason(task *core.Task, env *contextdata.Envelope) string {
	if taskNeedsEditing(task) && !hasEditObservation(env) {
		return "iteration budget exhausted before making any file changes"
	}
	return "iteration budget exhausted before task completion"
}

func hasEditObservation(env *contextdata.Envelope) bool {
	for _, observation := range getToolObservations(env) {
		name := observation.Tool
		if strings.Contains(name, "write") || strings.Contains(name, "create") || strings.Contains(name, "delete") {
			return true
		}
	}
	return false
}

func repeatedFailureAnalysis(task *core.Task, env *contextdata.Envelope, lastMap map[string]interface{}) (string, bool) {
	if taskNeedsEditing(task) || !hasFailure(lastMap) {
		return "", false
	}
	observations := getToolObservations(env)
	if len(observations) == 0 {
		return "", false
	}
	last := observations[len(observations)-1]
	if !failureAnalysisToolEligible(last.Tool) {
		return "", false
	}
	reason := failureAnalysisReason(last)
	if reason == "" {
		return "", false
	}
	return fmt.Sprintf("%s failed repeatedly: %s", last.Tool, reason), true
}

func analysisSummaryFromFailure(task *core.Task, env *contextdata.Envelope, lastMap map[string]interface{}) (string, bool) {
	if taskNeedsEditing(task) || !hasFailure(lastMap) {
		return "", false
	}
	observations := getToolObservations(env)
	if len(observations) == 0 {
		return "", false
	}
	last := observations[len(observations)-1]
	if !failureAnalysisToolEligible(last.Tool) {
		return "", false
	}
	reason := failureAnalysisReason(last)
	if reason == "" {
		return "", false
	}
	return fmt.Sprintf("%s failed: %s", last.Tool, reason), true
}

func failureAnalysisReason(observation ToolObservation) string {
	for _, raw := range []interface{}{
		observation.Data["stderr"],
		observation.Data["stdout"],
		observation.Data["error"],
	} {
		reason := strings.TrimSpace(firstMeaningfulLine(fmt.Sprint(raw)))
		if reason != "" {
			return reason
		}
	}
	return ""
}

func failureAnalysisToolEligible(toolName string) bool {
	lower := strings.ToLower(toolName)
	return strings.Contains(lower, "cargo") ||
		strings.Contains(lower, "test") ||
		strings.Contains(lower, "build")
}

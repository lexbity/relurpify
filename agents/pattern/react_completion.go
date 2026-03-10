package pattern

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
)

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

func agentSpecFromTask(task *core.Task) *core.AgentRuntimeSpec {
	return frameworkskills.EffectiveAgentSpec(task, nil)
}

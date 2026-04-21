package runtime

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// ClassifyTask infers the TaskType from the task using a rule-based heuristic.
// When the task already carries a non-empty Type, that value is returned
// unchanged. The LLM-fallback path is intentionally omitted: task type
// classification should be cheap and deterministic for small models.
func ClassifyTask(task *core.Task) core.TaskType {
	if task == nil {
		return core.TaskTypeAnalysis
	}
	if task.Type != "" {
		return task.Type
	}
	return classifyByKeyword(task.Instruction)
}

// classifyByKeyword maps instruction keywords to TaskType constants.
func classifyByKeyword(instruction string) core.TaskType {
	lower := strings.ToLower(instruction)

	reviewKeywords := []string{"review", "check", "inspect", "audit", "evaluate"}
	for _, kw := range reviewKeywords {
		if strings.Contains(lower, kw) {
			return core.TaskTypeReview
		}
	}

	planKeywords := []string{"plan", "design", "outline", "strategy"}
	for _, kw := range planKeywords {
		if strings.Contains(lower, kw) {
			return core.TaskTypePlanning
		}
	}

	generateKeywords := []string{"create", "generate", "write", "implement", "add", "build", "new"}
	for _, kw := range generateKeywords {
		if strings.Contains(lower, kw) {
			return core.TaskTypeCodeGeneration
		}
	}

	modifyKeywords := []string{"fix", "refactor", "update", "change", "modify", "patch", "correct", "improve"}
	for _, kw := range modifyKeywords {
		if strings.Contains(lower, kw) {
			return core.TaskTypeCodeModification
		}
	}

	return core.TaskTypeAnalysis
}

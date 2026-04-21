package analysis

import (
	"strings"

	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
)

// ClassifyGoal maps a task instruction to a deterministic goal condition.
func ClassifyGoal(taskInstruction string, _ *types.OperatorRegistry) types.GoalCondition {
	lower := strings.ToLower(strings.TrimSpace(taskInstruction))
	goal := types.GoalCondition{Description: taskInstruction}
	switch {
	case strings.Contains(lower, "fix") || strings.Contains(lower, "patch") || strings.Contains(lower, "modify"):
		goal.Predicates = []types.Predicate{"file_content_known", "edit_plan_known", "file_modified", "test_result_known"}
	case strings.Contains(lower, "analyze") || strings.Contains(lower, "review"):
		goal.Predicates = []types.Predicate{"file_content_known", "edit_plan_known"}
	case strings.Contains(lower, "generate") || strings.Contains(lower, "create") || strings.Contains(lower, "write"):
		goal.Predicates = []types.Predicate{"edit_plan_known", "file_modified"}
	case strings.Contains(lower, "test"):
		goal.Predicates = []types.Predicate{"file_modified", "test_result_known"}
	default:
		goal.Predicates = []types.Predicate{"file_content_known"}
	}
	return goal
}

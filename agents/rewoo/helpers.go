package rewoo

import (
	"fmt"
	"strings"
)

func summarizeRewooStepResults(results []RewooStepResult) string {
	if len(results) == 0 {
		return ""
	}
	parts := make([]string, 0, len(results))
	for _, result := range results {
		label := strings.TrimSpace(result.StepID)
		if label == "" {
			label = strings.TrimSpace(result.Tool)
		}
		if label == "" {
			continue
		}
		status := "ok"
		if !result.Success {
			status = "failed"
		}
		part := fmt.Sprintf("%s:%s", label, status)
		if result.Error != "" {
			part += " - " + strings.TrimSpace(result.Error)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func buildReplanContext(_ any, results []RewooStepResult, _ any) map[string]any {
	failed := make([]string, 0)
	for _, result := range results {
		if !result.Success {
			failed = append(failed, result.StepID)
		}
	}
	return map[string]any{
		"summary":       summarizeRewooStepResults(results),
		"failed_steps":  failed,
		"step_results":  results,
		"replan_reason": "step failure threshold reached",
	}
}

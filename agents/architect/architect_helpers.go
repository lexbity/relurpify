package architect

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

func clearArchitectActiveStepState(state *core.Context) {
	if state == nil {
		return
	}
	state.Set("architect.current_step", map[string]any{})
	state.Set("architect.current_step_id", "")
}

func summarizeStepResult(step core.PlanStep, result *core.Result) string {
	if result == nil {
		return fmt.Sprintf("Step %s completed.", step.ID)
	}
	if result.Error != nil {
		return fmt.Sprintf("Step %s failed: %v", step.ID, result.Error)
	}
	return fmt.Sprintf("Step %s completed: %s", step.ID, step.Description)
}

func mustJSONForArchitect(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func optionalContextString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func knowledgeContents(records []memory.KnowledgeRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		text := strings.TrimSpace(record.Content)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func knowledgeSummary(records []memory.KnowledgeRecord) string {
	return strings.Join(knowledgeContents(records), "\n")
}

func coerceArchitectPlan(raw any) (core.Plan, bool) {
	switch typed := raw.(type) {
	case core.Plan:
		return typed, true
	case *core.Plan:
		if typed == nil {
			return core.Plan{}, false
		}
		return *typed, true
	default:
		encoded, err := json.Marshal(raw)
		if err != nil {
			return core.Plan{}, false
		}
		var decoded core.Plan
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return core.Plan{}, false
		}
		if decoded.Dependencies == nil {
			decoded.Dependencies = map[string][]string{}
		}
		return decoded, true
	}
}

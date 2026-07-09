package agenttest

import (
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func CountToolCalls(events []telemetry.Event) (total int, byTool map[string]int) {
	byTool = make(map[string]int)
	for _, ev := range events {
		if ev.Type != telemetry.EventToolCall {
			continue
		}
		total++
		tool, _ := ev.Metadata["tool"].(string)
		if tool != "" {
			byTool[tool]++
		}
	}
	return total, byTool
}

func CountTokenUsage(events []telemetry.Event) TokenUsageReport {
	var usage TokenUsageReport
	for _, ev := range events {
		if ev.Type != telemetry.EventLLMResponse {
			continue
		}
		rawUsage, ok := ev.Metadata["usage"]
		if !ok {
			continue
		}
		typed, ok := rawUsage.(map[string]any)
		if !ok {
			continue
		}
		usage.LLMCalls++
		prompt := intValue(typed["prompt_tokens"])
		completion := intValue(typed["completion_tokens"])
		total := intValue(typed["total_tokens"])
		if total == 0 {
			total = prompt + completion
		}
		usage.PromptTokens += prompt
		usage.CompletionTokens += completion
		usage.TotalTokens += total
	}
	return usage
}

func intValue(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

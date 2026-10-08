package agenttest

import (
	"codeburg.org/lexbit/relurpify/model"
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
		prompt, completion, total, ok := tokenUsageFromRaw(ev.Metadata["usage"])
		if !ok {
			continue
		}
		usage.LLMCalls++
		if total == 0 {
			total = prompt + completion
		}
		usage.PromptTokens += prompt
		usage.CompletionTokens += completion
		usage.TotalTokens += total
	}
	return usage
}

// tokenUsageFromRaw normalises the several shapes token usage can take before
// it reaches the harness: the typed model.TokenUsage produced by the LLM
// instrumentation, a decoded JSON object, or an integer map from a provider
// client. The second return value reports whether usage was present at all.
func tokenUsageFromRaw(raw any) (prompt, completion, total int, ok bool) {
	switch typed := raw.(type) {
	case model.TokenUsage:
		return typed.PromptTokens, typed.CompletionTokens, typed.TotalTokens, true
	case *model.TokenUsage:
		if typed == nil {
			return 0, 0, 0, false
		}
		return typed.PromptTokens, typed.CompletionTokens, typed.TotalTokens, true
	case map[string]any:
		return intValue(typed["prompt_tokens"]), intValue(typed["completion_tokens"]), intValue(typed["total_tokens"]), true
	case map[string]int:
		return typed["prompt_tokens"], typed["completion_tokens"], typed["total_tokens"], true
	default:
		return 0, 0, 0, false
	}
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

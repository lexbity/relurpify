package llm

import "codeburg.org/lexbit/relurpify/model"

// ToolCallingModeLabel returns the resolved tool-calling mode for a model.
// Unknown or nil models are treated as "unknown"; profiled models are mapped
// to "native" or "fallback" based on their UsesNativeToolCalling contract.
func ToolCallingModeLabel(lm LanguageModel) string {
	if lm == nil {
		return "unknown"
	}
	if profiled, ok := any(lm).(model.ProfiledModel); ok {
		if profiled.UsesNativeToolCalling() {
			return "native"
		}
		return "fallback"
	}
	return "fallback"
}

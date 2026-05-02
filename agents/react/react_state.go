package react

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ToolObservation records a single tool execution result within the ReAct loop.
type ToolObservation struct {
	Tool      string                 `json:"tool"`
	Phase     string                 `json:"phase"`
	Summary   string                 `json:"summary"`
	Args      map[string]interface{} `json:"args,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Success   bool                   `json:"success"`
	Timestamp time.Time              `json:"timestamp"`
}

func mirrorReactFinalOutputReference(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("react.final_output"); !ok {
		return
	}
	if rawRef, ok := env.GetWorkingValue("graph.summary_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			env.SetWorkingValue("react.final_output_ref", ref, contextdata.MemoryClassTask)
		}
	}
	if summary := strings.TrimSpace(envGetString(env, "graph.summary")); summary != "" {
		env.SetWorkingValue("react.final_output_summary", summary, contextdata.MemoryClassTask)
	}
}

func mirrorReactCheckpointReference(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if rawRef, ok := env.GetWorkingValue("graph.checkpoint_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			env.SetWorkingValue("react.checkpoint_ref", ref, contextdata.MemoryClassTask)
		}
	}
}

func compactReactFinalOutputState(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("react.final_output_ref"); !ok {
		return
	}
	raw, ok := env.GetWorkingValue("react.final_output")
	if !ok {
		return
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return
	}
	summary := strings.TrimSpace(fmt.Sprint(payload["summary"]))
	if summary == "" || summary == "<nil>" {
		return
	}
	env.SetWorkingValue("react.final_output", map[string]any{
		"summary": summary,
	}, contextdata.MemoryClassTask)
}

func compactReactToolObservationsState(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("react.final_output_ref"); !ok {
		return
	}
	raw, ok := env.GetWorkingValue("react.tool_observations")
	if !ok {
		return
	}
	observations, ok := raw.([]ToolObservation)
	if !ok {
		return
	}
	env.SetWorkingValue("react.tool_observations", compactReactToolObservations(observations), contextdata.MemoryClassTask)
}

func compactReactToolObservations(observations []ToolObservation) map[string]any {
	value := map[string]any{
		"observation_count": len(observations),
	}
	if len(observations) == 0 {
		return value
	}
	last := observations[len(observations)-1]
	value["last_tool"] = last.Tool
	value["last_success"] = last.Success
	if len(observations) > 0 {
		recent := make([]string, 0, len(observations))
		for _, observation := range observations {
			tool := strings.TrimSpace(observation.Tool)
			if tool == "" {
				continue
			}
			recent = append(recent, tool)
			if len(recent) == 3 {
				break
			}
		}
		if len(recent) > 0 {
			value["recent_tools"] = recent
		}
	}
	return value
}

func compactReactLastToolResultState(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("react.final_output_ref"); !ok {
		return
	}
	raw, ok := env.GetWorkingValue("react.last_tool_result")
	if !ok {
		return
	}
	payload, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	env.SetWorkingValue("react.last_tool_result", compactReactLastToolResult(payload), contextdata.MemoryClassTask)
}

func compactReactLastToolResult(payload map[string]interface{}) map[string]any {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	value := map[string]any{
		"key_count": len(keys),
		"keys":      keys,
	}
	if errText := strings.TrimSpace(fmt.Sprint(payload["error"])); errText != "" && errText != "<nil>" {
		value["error"] = errText
	}
	if success, ok := payload["success"].(bool); ok {
		value["success"] = success
	}
	return value
}

func compactReactLoopState(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	if _, ok := env.GetWorkingValue("react.final_output_ref"); !ok {
		return
	}
	if raw, ok := env.GetWorkingValue("react.decision"); ok && raw != nil {
		env.SetWorkingValue("react.decision", map[string]any{"present": true}, contextdata.MemoryClassTask)
	}
	if raw, ok := env.GetWorkingValue("react.tool_calls"); ok {
		switch calls := raw.(type) {
		case []contracts.ToolCall:
			env.SetWorkingValue("react.tool_calls", map[string]any{"count": len(calls)}, contextdata.MemoryClassTask)
		case []any:
			env.SetWorkingValue("react.tool_calls", map[string]any{"count": len(calls)}, contextdata.MemoryClassTask)
		}
	}
	if _, ok := env.GetWorkingValue("react.last_tool_result_envelope"); ok {
		env.SetWorkingValue("react.last_tool_result_envelope", map[string]any{"present": true}, contextdata.MemoryClassTask)
	}
	if raw, ok := env.GetWorkingValue("react.last_tool_result_envelopes"); ok {
		switch envelopes := raw.(type) {
		case []*core.CapabilityResultEnvelope:
			env.SetWorkingValue("react.last_tool_result_envelopes", map[string]any{"count": len(envelopes)}, contextdata.MemoryClassTask)
		case []any:
			env.SetWorkingValue("react.last_tool_result_envelopes", map[string]any{"count": len(envelopes)}, contextdata.MemoryClassTask)
		}
	}
}

func activeToolSet(env *contextdata.Envelope) map[string]struct{} {
	out := map[string]struct{}{}
	if env == nil {
		return out
	}
	raw, ok := env.GetWorkingValue("react.active_tools")
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

func recordActiveToolNames(env *contextdata.Envelope, tools []contracts.Tool) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	env.SetWorkingValue("react.active_tools", names, contextdata.MemoryClassTask)
}

func (a *ReActAgent) getLastResult(env *contextdata.Envelope) *core.Result {
	if env == nil {
		return nil
	}
	if val, ok := env.GetWorkingValue("react.last_result"); ok {
		if res, ok := val.(*core.Result); ok {
			return res
		}
	}
	return nil
}

func (a *ReActAgent) enforceBudget(env *contextdata.Envelope) {
}

func (a *ReActAgent) recordLatestInteraction(env *contextdata.Envelope) {
}

func (a *ReActAgent) manageContextSignals(env *contextdata.Envelope) {
}

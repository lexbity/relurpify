package react

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
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

func mirrorReactFinalOutputReference(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output"); !ok {
		return
	}
	if rawRef, ok := state.Get("graph.summary_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			state.Set("react.final_output_ref", ref)
		}
	}
	if summary := strings.TrimSpace(state.GetString("graph.summary")); summary != "" {
		state.Set("react.final_output_summary", summary)
	}
}

func mirrorReactCheckpointReference(state *core.Context) {
	if state == nil {
		return
	}
	if rawRef, ok := state.Get("graph.checkpoint_ref"); ok {
		if ref, ok := rawRef.(core.ArtifactReference); ok {
			state.Set("react.checkpoint_ref", ref)
		}
	}
}

func compactReactFinalOutputState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	raw, ok := state.Get("react.final_output")
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
	state.Set("react.final_output", map[string]any{
		"summary": summary,
	})
}

func compactReactToolObservationsState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	raw, ok := state.Get("react.tool_observations")
	if !ok {
		return
	}
	observations, ok := raw.([]ToolObservation)
	if !ok {
		return
	}
	state.Set("react.tool_observations", compactReactToolObservations(observations))
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

func compactReactLastToolResultState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	raw, ok := state.Get("react.last_tool_result")
	if !ok {
		return
	}
	payload, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	state.Set("react.last_tool_result", compactReactLastToolResult(payload))
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

func compactReactLoopState(state *core.Context) {
	if state == nil {
		return
	}
	if _, ok := state.Get("react.final_output_ref"); !ok {
		return
	}
	if raw, ok := state.Get("react.decision"); ok && raw != nil {
		state.Set("react.decision", map[string]any{"present": true})
	}
	if raw, ok := state.Get("react.tool_calls"); ok {
		switch calls := raw.(type) {
		case []core.ToolCall:
			state.Set("react.tool_calls", map[string]any{"count": len(calls)})
		case []any:
			state.Set("react.tool_calls", map[string]any{"count": len(calls)})
		}
	}
	if _, ok := state.Get("react.last_tool_result_envelope"); ok {
		state.Set("react.last_tool_result_envelope", map[string]any{"present": true})
	}
	if raw, ok := state.Get("react.last_tool_result_envelopes"); ok {
		switch envelopes := raw.(type) {
		case []*core.CapabilityResultEnvelope:
			state.Set("react.last_tool_result_envelopes", map[string]any{"count": len(envelopes)})
		case []any:
			state.Set("react.last_tool_result_envelopes", map[string]any{"count": len(envelopes)})
		}
	}
}

func activeToolSet(state *core.Context) map[string]struct{} {
	out := map[string]struct{}{}
	if state == nil {
		return out
	}
	raw, ok := state.Get("react.active_tools")
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

func recordActiveToolNames(state *core.Context, tools []core.Tool) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	state.Set("react.active_tools", names)
}

func (a *ReActAgent) getLastResult(state *core.Context) *core.Result {
	if state == nil {
		return nil
	}
	if val, ok := state.GetHandle("react.last_result"); ok {
		if res, ok := val.(*core.Result); ok {
			return res
		}
	}
	val, ok := state.Get("react.last_result")
	if ok {
		if res, ok := val.(*core.Result); ok {
			return res
		}
	}
	return nil
}

func (a *ReActAgent) enforceBudget(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	var tools []core.Tool
	if catalog := a.executionCapabilityCatalog(); catalog != nil {
		tools = catalog.ModelCallableTools()
	} else if a.Tools != nil {
		tools = a.Tools.ModelCallableTools()
	}
	a.contextPolicy.EnforceBudget(state, a.sharedContext, a.Model, tools, a.debugf)
}

func (a *ReActAgent) recordLatestInteraction(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	a.contextPolicy.RecordLatestInteraction(state, a.debugf)
}

func (a *ReActAgent) manageContextSignals(state *core.Context) {
	if a.contextPolicy == nil {
		return
	}
	lastResult := a.getLastResult(state)
	a.contextPolicy.HandleSignals(state, a.sharedContext, lastResult)
}

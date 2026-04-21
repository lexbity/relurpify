package react

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func (a *ReActAgent) finalizeExecuteResult(ctx context.Context, task *core.Task, state *core.Context, result *core.Result, err error) (*core.Result, error) {
	if err == nil && result != nil {
		if followErr := a.completeExplicitReadOnlyRetrieval(ctx, task, state); followErr != nil {
			a.debugf("explicit retrieval follow-up failed: %v", followErr)
		}
		rawLast, _ := state.Get("react.last_tool_result")
		lastMap, _ := rawLast.(map[string]interface{})
		if summary, ok := completionSummaryFromState(a, task, state, lastMap); ok {
			state.Set("react.incomplete_reason", "")
			state.Set("react.synthetic_summary", summary)
			state.Set("react.final_output", map[string]interface{}{
				"summary": summary,
				"result":  lastMap,
			})
			result.Success = true
			result.Error = nil
		}
		if final, ok := state.Get("react.final_output"); ok {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["final_output"] = final
			if summary := finalOutputSummary(final); summary != "" {
				result.Data["text"] = summary
			}
		}
		mirrorReactFinalOutputReference(state)
		compactReactFinalOutputState(state)
		compactReactLastToolResultState(state)
		compactReactToolObservationsState(state)
		compactReactLoopState(state)
		mirrorReactCheckpointReference(state)
		if reason := strings.TrimSpace(state.GetString("react.incomplete_reason")); reason != "" {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["incomplete_reason"] = reason
			// For non-editing tasks that produced observations, degrade rather than
			// hard-fail - partial analysis output is still useful to the caller.
			if !taskNeedsEditing(task) && len(getToolObservations(state)) > 0 {
				result.Success = true
				result.Data["degraded"] = true
			} else {
				result.Success = false
				result.Error = fmt.Errorf("%s", reason)
			}
		}
	}
	return result, err
}

func (a *ReActAgent) completeExplicitReadOnlyRetrieval(ctx context.Context, task *core.Task, state *core.Context) error {
	if a == nil || a.Tools == nil || task == nil || state == nil || taskNeedsEditing(task) {
		return nil
	}
	requested := explicitlyRequestedToolNames(task)
	if len(requested) == 0 {
		return nil
	}
	if _, wantsSemantic := requested["search_semantic"]; !wantsSemantic {
		return nil
	}
	if _, wantsRead := requested["file_read"]; !wantsRead || requestedReadOnlyToolsSatisfied(task, state) {
		return nil
	}
	rawLast, _ := state.Get("react.last_tool_result")
	lastMap, _ := rawLast.(map[string]interface{})
	path := firstSearchResultPath(lastMap["results"])
	if path == "" {
		return nil
	}
	res, err := a.Tools.InvokeCapability(ctx, state, "file_read", map[string]any{"path": path})
	if err != nil {
		return err
	}
	call := core.ToolCall{
		ID:   NewUUID(),
		Name: "file_read",
		Args: map[string]any{"path": path},
	}
	observation := summarizeToolResult(state, call, res)
	history := append(getToolObservations(state), observation)
	state.Set("react.tool_observations", history)
	state.Set("react.last_tool_result", res.Data)
	return nil
}

func firstSearchResultPath(raw any) string {
	switch items := raw.(type) {
	case []any:
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if path := strings.TrimSpace(fmt.Sprint(entry["file"])); path != "" {
				return path
			}
		}
	case []map[string]any:
		for _, entry := range items {
			if path := strings.TrimSpace(fmt.Sprint(entry["file"])); path != "" {
				return path
			}
		}
	}
	return ""
}

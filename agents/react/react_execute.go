package react

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func (a *ReActAgent) finalizeExecuteResult(ctx context.Context, task *core.Task, env *contextdata.Envelope, result *core.Result, err error) (*core.Result, error) {
	if err == nil && result != nil {
		if followErr := a.completeExplicitReadOnlyRetrieval(ctx, task, env); followErr != nil {
			a.debugf("explicit retrieval follow-up failed: %v", followErr)
		}
		rawLast, _ := env.GetWorkingValue("react.last_tool_result")
		lastMap, _ := rawLast.(map[string]interface{})
		if summary, ok := completionSummaryFromState(a, task, env, lastMap); ok {
			env.SetWorkingValue("react.incomplete_reason", "", contextdata.MemoryClassTask)
			env.SetWorkingValue("react.synthetic_summary", summary, contextdata.MemoryClassTask)
			env.SetWorkingValue("react.final_output", map[string]interface{}{
				"summary": summary,
				"result":  lastMap,
			}, contextdata.MemoryClassTask)
			result.Success = true
			result.Error = ""
		}
		if final, ok := env.GetWorkingValue("react.final_output"); ok {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["final_output"] = final
			if summary := finalOutputSummary(final); summary != "" {
				result.Data["text"] = summary
			}
		}
		mirrorReactFinalOutputReference(env)
		compactReactFinalOutputState(env)
		compactReactLastToolResultState(env)
		compactReactToolObservationsState(env)
		compactReactLoopState(env)
		mirrorReactCheckpointReference(env)
		if reason := strings.TrimSpace(envGetString(env, "react.incomplete_reason")); reason != "" {
			if result.Data == nil {
				result.Data = map[string]any{}
			}
			result.Data["incomplete_reason"] = reason
			// For non-editing tasks that produced observations, degrade rather than
			// hard-fail - partial analysis output is still useful to the caller.
			if !taskNeedsEditing(task) && len(getToolObservations(env)) > 0 {
				result.Success = true
				result.Data["degraded"] = true
			} else {
				result.Success = false
				result.Error = reason
			}
		}
	}
	return result, err
}

func (a *ReActAgent) completeExplicitReadOnlyRetrieval(ctx context.Context, task *core.Task, env *contextdata.Envelope) error {
	if a == nil || a.Tools == nil || task == nil || env == nil || taskNeedsEditing(task) {
		return nil
	}
	requested := explicitlyRequestedToolNames(task)
	if len(requested) == 0 {
		return nil
	}
	if _, wantsSemantic := requested["search_semantic"]; !wantsSemantic {
		return nil
	}
	if _, wantsRead := requested["file_read"]; !wantsRead || requestedReadOnlyToolsSatisfied(task, env) {
		return nil
	}
	rawLast, _ := env.GetWorkingValue("react.last_tool_result")
	lastMap, _ := rawLast.(map[string]interface{})
	path := firstSearchResultPath(lastMap["results"])
	if path == "" {
		return nil
	}
	res, err := a.Tools.InvokeCapability(ctx, env, "file_read", map[string]any{"path": path})
	if err != nil {
		return err
	}
	call := core.ToolCall{
		ID:   NewUUID(),
		Name: "file_read",
		Args: map[string]any{"path": path},
	}
	observation := summarizeToolResult(env, call, res)
	history := append(getToolObservations(env), observation)
	env.SetWorkingValue("react.tool_observations", history, contextdata.MemoryClassTask)
	env.SetWorkingValue("react.last_tool_result", res.Data, contextdata.MemoryClassTask)
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

// envGetString is a helper to get a string value from the envelope working memory.
// This replaces the old state.GetString() method.
func envGetString(env *contextdata.Envelope, key string) string {
	if env == nil {
		return ""
	}
	val, ok := env.GetWorkingValue(key)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(val))
}

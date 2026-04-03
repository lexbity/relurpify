package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func ApplyEditIntentArtifacts(ctx context.Context, registry *capability.Registry, state *core.Context) (*EditExecutionRecord, error) {
	if state == nil {
		return nil, nil
	}
	intents := extractEditIntents(state)
	if len(intents) == 0 {
		record := synthesizeEditExecutionFromPipelineCode(state)
		if record == nil {
			return nil, nil
		}
		state.Set("euclo.edit_execution", *record)
		return record, nil
	}
	record := ExecuteEditIntents(ctx, registry, intents, state)
	state.Set("euclo.edit_execution", record)
	return &record, nil
}

func ExecuteEditIntents(ctx context.Context, registry *capability.Registry, intents []EditIntent, state *core.Context) EditExecutionRecord {
	record := EditExecutionRecord{
		Requested: make([]EditOperationRecord, 0, len(intents)),
		Approved:  make([]EditOperationRecord, 0, len(intents)),
		Executed:  make([]EditOperationRecord, 0, len(intents)),
		Rejected:  make([]EditOperationRecord, 0),
	}
	for _, intent := range intents {
		op := EditOperationRecord{
			Path:      strings.TrimSpace(intent.Path),
			Action:    strings.TrimSpace(strings.ToLower(intent.Action)),
			Summary:   strings.TrimSpace(intent.Summary),
			Requested: true,
			Status:    "requested",
		}
		record.Requested = append(record.Requested, op)
		if registry == nil {
			op.Status = "rejected"
			op.Error = "capability registry unavailable"
			record.Rejected = append(record.Rejected, op)
			continue
		}
		toolName, args, err := capabilityInvocationForIntent(intent)
		if err != nil {
			op.Status = "rejected"
			op.Error = err.Error()
			record.Rejected = append(record.Rejected, op)
			continue
		}
		op.ApprovalStatus = "implicit"
		op.Tool = toolName
		record.Approved = append(record.Approved, op)
		result, invokeErr := registry.InvokeCapability(ctx, state, toolName, args)
		if invokeErr != nil {
			op.Status = "rejected"
			op.Error = invokeErr.Error()
			record.Rejected = append(record.Rejected, op)
			continue
		}
		op.Status = "executed"
		if result != nil {
			op.Result = cloneAnyMap(result.Data)
			if !result.Success && strings.TrimSpace(result.Error) != "" {
				op.Status = "rejected"
				op.Error = strings.TrimSpace(result.Error)
				record.Rejected = append(record.Rejected, op)
				continue
			}
		}
		record.Executed = append(record.Executed, op)
	}
	record.Summary = summarizeEditExecution(record)
	return record
}

func capabilityInvocationForIntent(intent EditIntent) (string, map[string]any, error) {
	path := strings.TrimSpace(intent.Path)
	action := strings.TrimSpace(strings.ToLower(intent.Action))
	if path == "" {
		return "", nil, fmt.Errorf("edit path required")
	}
	switch action {
	case "create", "update":
		content := intent.Content
		if strings.TrimSpace(content) == "" {
			return "", nil, fmt.Errorf("edit content required for %s", action)
		}
		return "file_write", map[string]any{
			"path":    path,
			"content": content,
		}, nil
	case "delete":
		return "file_delete", map[string]any{"path": path}, nil
	default:
		return "", nil, fmt.Errorf("unsupported edit action %q", intent.Action)
	}
}

func extractEditIntents(state *core.Context) []EditIntent {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("pipeline.code")
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case map[string]any:
		return editIntentsFromMap(typed)
	default:
		return nil
	}
}

func editIntentsFromMap(payload map[string]any) []EditIntent {
	rawEdits, ok := payload["edits"]
	if !ok || rawEdits == nil {
		return nil
	}
	items, ok := rawEdits.([]any)
	if !ok {
		if typed, ok := rawEdits.([]map[string]any); ok {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		} else {
			return nil
		}
	}
	intents := make([]EditIntent, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		intent := EditIntent{
			Path:    strings.TrimSpace(fmt.Sprint(entry["path"])),
			Action:  strings.TrimSpace(fmt.Sprint(entry["action"])),
			Content: fmt.Sprint(entry["content"]),
			Summary: strings.TrimSpace(fmt.Sprint(entry["summary"])),
		}
		if intent.Content == "<nil>" {
			intent.Content = ""
		}
		if intent.Path == "" || intent.Action == "" {
			continue
		}
		intents = append(intents, intent)
	}
	return intents
}

func summarizeEditExecution(record EditExecutionRecord) string {
	parts := make([]string, 0, 3)
	if n := len(record.Requested); n > 0 {
		parts = append(parts, fmt.Sprintf("requested=%d", n))
	}
	if n := len(record.Executed); n > 0 {
		parts = append(parts, fmt.Sprintf("executed=%d", n))
	}
	if n := len(record.Rejected); n > 0 {
		parts = append(parts, fmt.Sprintf("rejected=%d", n))
	}
	return strings.Join(parts, " ")
}

func synthesizeEditExecutionFromPipelineCode(state *core.Context) *EditExecutionRecord {
	if state == nil {
		return nil
	}
	raw, ok := state.Get("pipeline.code")
	if !ok || raw == nil {
		return nil
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	finalOutput, ok := payload["final_output"].(map[string]any)
	if !ok {
		return nil
	}
	result, ok := finalOutput["result"].(map[string]any)
	if !ok || len(result) == 0 {
		return nil
	}
	record := EditExecutionRecord{}
	for toolName, rawEntry := range result {
		action, tracksEdit := editActionForToolName(toolName)
		if !tracksEdit {
			continue
		}
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		data, _ := entry["data"].(map[string]any)
		path := ""
		if data != nil {
			path = strings.TrimSpace(fmt.Sprint(data["path"]))
			if path == "<nil>" {
				path = ""
			}
		}
		if path == "" {
			continue
		}
		op := EditOperationRecord{
			Path:      path,
			Action:    action,
			Requested: true,
			Status:    "executed",
			Tool:      toolName,
			Result:    cloneAnyMap(data),
		}
		if summary := strings.TrimSpace(fmt.Sprint(payload["summary"])); summary != "" && summary != "<nil>" {
			op.Summary = summary
		}
		record.Requested = append(record.Requested, op)
		record.Approved = append(record.Approved, op)
		if success, ok := entry["success"].(bool); ok && !success {
			op.Status = "rejected"
			errText := strings.TrimSpace(fmt.Sprint(entry["error"]))
			if errText != "" && errText != "<nil>" {
				op.Error = errText
			}
			record.Rejected = append(record.Rejected, op)
			continue
		}
		record.Executed = append(record.Executed, op)
	}
	if len(record.Requested) == 0 && len(record.Executed) == 0 && len(record.Rejected) == 0 {
		return nil
	}
	record.Summary = summarizeEditExecution(record)
	return &record
}

func editActionForToolName(toolName string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(toolName)) {
	case "file_write":
		return "update", true
	case "file_create":
		return "create", true
	case "file_delete":
		return "delete", true
	default:
		return "", false
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

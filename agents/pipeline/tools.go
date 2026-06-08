package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
)

// CapabilityInvoker routes tool calls through the registered capability path
// with policy evaluation, safety checks, and provenance recording.
type CapabilityInvoker interface {
	InvokeCapability(ctx context.Context, env ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error)
}

type toolObservation struct {
	Name    string         `json:"name"`
	Args    map[string]any `json:"args,omitempty"`
	Success bool           `json:"success"`
	Error   string         `json:"error,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func resolveStageTools(ctx context.Context, env *contextdata.Envelope, stage Stage, available []ports.Tool) []ports.Tool {
	if stage == nil || len(available) == 0 {
		return nil
	}
	scoped, ok := stage.(ToolScopedStage)
	if !ok {
		return nil
	}
	allowed := make(map[string]struct{})
	for _, name := range scoped.AllowedToolNames() {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	tools := make([]ports.Tool, 0, len(allowed))
	for _, tool := range available {
		if tool == nil {
			continue
		}
		if _, ok := allowed[tool.Name()]; ok && tool.IsAvailable(ctx) {
			tools = append(tools, tool)
		}
	}
	return tools
}

func executeToolCalls(ctx context.Context, env *contextdata.Envelope, calls []model.ToolCall, tools []ports.Tool, invoker CapabilityInvoker) ([]toolObservation, error) {
	if len(calls) == 0 || len(tools) == 0 {
		return nil, nil
	}
	index := make(map[string]ports.Tool, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		index[tool.Name()] = tool
	}
	observations := make([]toolObservation, 0, len(calls))
	for _, call := range calls {
		if _, ok := index[call.Name]; !ok {
			return observations, fmt.Errorf("pipeline tool %s not allowed for stage", call.Name)
		}
		var (
			result *ports.ToolResult
			err    error
		)
		if invoker == nil {
			return observations, fmt.Errorf(
				"pipeline stage: capability invoker required — " +
					"direct tool.Execute() bypass has been removed")
		}
		result, err = invoker.InvokeCapability(ctx, env.State(), call.Name, call.Args)
		if err != nil {
			return observations, err
		}
		observation := toolObservation{
			Name:    call.Name,
			Args:    call.Args,
			Success: result != nil && result.Success,
		}
		if result != nil {
			observation.Error = result.Error
			observation.Data = result.Data
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func formatToolObservations(observations []toolObservation) string {
	if len(observations) == 0 {
		return ""
	}
	encoded, err := json.MarshalIndent(observations, "", "  ")
	if err != nil {
		return fmt.Sprint(observations)
	}
	return string(encoded)
}

package capabilities

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// capabilityInvoker matches the framework capability registry invocation contract.
type capabilityInvoker interface {
	InvokeCapability(ctx context.Context, state ports.State, idOrName string, args map[string]any) (*ports.ToolResult, error)
}

// InvokeCapability invokes a capability through the capability registry.
// It adapts the ToolResult to execution.Result.
func InvokeCapability(ctx context.Context, capID string, task *execution.Task, env ports.State, registry capabilityInvoker) (*execution.Result, error) {
	// Extract args from task.Data or task.Context
	args := map[string]any{}
	if task != nil {
		// Prioritize task.Data for capability arguments
		if task.Data != nil {
			for k, v := range task.Data {
				args[k] = v
			}
		}
		// Fall back to task.Context if Data is empty
		if len(args) == 0 && task.Context != nil {
			for k, v := range task.Context {
				args[k] = v
			}
		}
	}

	if registry == nil {
		return &execution.Result{
			NodeID:  capID,
			Success: false,
			Data:    execution.NewErrorResultPayload("capability registry unavailable"),
		}, fmt.Errorf("capability registry unavailable")
	}

	toolResult, err := registry.InvokeCapability(ctx, env, capID, args)
	if err != nil {
		return &execution.Result{
			NodeID:  capID,
			Success: false,
			Data:    execution.NewErrorResultPayload(err.Error()),
		}, err
	}
	if toolResult == nil {
		return &execution.Result{
			NodeID:  capID,
			Success: false,
			Data:    execution.NewErrorResultPayload(fmt.Sprintf("registry returned nil result for capability %s", capID)),
		}, fmt.Errorf("registry returned nil result for capability %s", capID)
	}

	var resultErr error
	if toolResult.Error != "" {
		resultErr = fmt.Errorf("%s", toolResult.Error)
	}
	return &execution.Result{
		NodeID:  capID,
		Success: toolResult.Success,
		Data:    execution.NewToolResultPayload(toolResult.Data),
	}, resultErr
}

// InvokeCapabilitySequence invokes a sequence of capabilities with an operator (AND/OR).
func InvokeCapabilitySequence(ctx context.Context, capabilityIDs []string, operator string, task *execution.Task, env ports.State, registry capabilityInvoker) (*execution.Result, error) {
	if len(capabilityIDs) == 0 {
		return &execution.Result{
			Success: false,
			Data:    execution.NewErrorResultPayload("no capabilities to invoke"),
		}, fmt.Errorf("no capabilities to invoke")
	}

	if operator == "AND" {
		for _, capID := range capabilityIDs {
			result, err := InvokeCapability(ctx, capID, task, env, registry)
			if err != nil || !result.Success {
				return result, err
			}
		}
		return &execution.Result{
			Success: true,
			Data: execution.NewToolResultPayload(map[string]any{
				"sequence_operator": "AND",
				"capabilities":      capabilityIDs,
			}),
		}, nil
	}

	if operator == "OR" {
		var lastError error
		for _, capID := range capabilityIDs {
			result, err := InvokeCapability(ctx, capID, task, env, registry)
			if err == nil && result.Success {
				return result, nil
			}
			lastError = err
		}
		return &execution.Result{
			Success: false,
			Data:    execution.NewErrorResultPayload("all capabilities in OR sequence failed"),
		}, lastError
	}

	return &execution.Result{
		Success: false,
		Data:    execution.NewErrorResultPayload(fmt.Sprintf("unknown operator: %s", operator)),
	}, fmt.Errorf("unknown operator: %s", operator)
}

package capabilities

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// capabilityInvoker matches the framework capability registry invocation contract.
type capabilityInvoker interface {
	InvokeCapability(ctx context.Context, state *contextdata.Envelope, idOrName string, args map[string]interface{}) (*contracts.ToolResult, error)
}

// InvokeCapability invokes a capability through the capability registry.
// It adapts the ToolResult to core.Result.
func InvokeCapability(ctx context.Context, capID string, task *core.Task, env *contextdata.Envelope, registry capabilityInvoker) (*core.Result, error) {
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
		return &core.Result{
			NodeID:  capID,
			Success: false,
			Data:    core.NewErrorResultPayload("capability registry unavailable"),
		}, fmt.Errorf("capability registry unavailable")
	}

	toolResult, err := registry.InvokeCapability(ctx, env, capID, args)
	if err != nil {
		return &core.Result{
			NodeID:  capID,
			Success: false,
			Data:    core.NewErrorResultPayload(err.Error()),
		}, err
	}
	if toolResult == nil {
		return &core.Result{
			NodeID:  capID,
			Success: false,
			Data:    core.NewErrorResultPayload(fmt.Sprintf("registry returned nil result for capability %s", capID)),
		}, fmt.Errorf("registry returned nil result for capability %s", capID)
	}

	var resultErr error
	if toolResult.Error != "" {
		resultErr = fmt.Errorf("%s", toolResult.Error)
	}
	return &core.Result{
		NodeID:  capID,
		Success: toolResult.Success,
		Data:    core.NewToolResultPayload(toolResult.Data),
	}, resultErr
}

// InvokeCapabilitySequence invokes a sequence of capabilities with an operator (AND/OR).
func InvokeCapabilitySequence(ctx context.Context, capabilityIDs []string, operator string, task *core.Task, env *contextdata.Envelope, registry capabilityInvoker) (*core.Result, error) {
	if len(capabilityIDs) == 0 {
		return &core.Result{
			Success: false,
			Data:    core.NewErrorResultPayload("no capabilities to invoke"),
		}, fmt.Errorf("no capabilities to invoke")
	}

	if operator == "AND" {
		for _, capID := range capabilityIDs {
			result, err := InvokeCapability(ctx, capID, task, env, registry)
			if err != nil || !result.Success {
				return result, err
			}
		}
		return &core.Result{
			Success: true,
			Data: core.NewToolResultPayload(map[string]any{
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
		return &core.Result{
			Success: false,
			Data:    core.NewErrorResultPayload("all capabilities in OR sequence failed"),
		}, lastError
	}

	return &core.Result{
		Success: false,
		Data:    core.NewErrorResultPayload(fmt.Sprintf("unknown operator: %s", operator)),
	}, fmt.Errorf("unknown operator: %s", operator)
}

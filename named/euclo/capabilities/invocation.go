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
	_ = task
	args := map[string]any{}

	if registry == nil {
		return &core.Result{
			NodeID:  capID,
			Success: true,
			Data: map[string]any{
				"capability_id": capID,
				"stub":          true,
			},
		}, nil
	}

	toolResult, err := registry.InvokeCapability(ctx, env, capID, args)
	if err != nil {
		return &core.Result{
			NodeID:  capID,
			Success: false,
			Data: map[string]any{
				"error": err.Error(),
			},
		}, err
	}
	if toolResult == nil {
		return &core.Result{
			NodeID:  capID,
			Success: false,
			Data: map[string]any{
				"error": fmt.Sprintf("registry returned nil result for capability %s", capID),
			},
		}, fmt.Errorf("registry returned nil result for capability %s", capID)
	}

	var resultErr error
	if toolResult.Error != "" {
		resultErr = fmt.Errorf("%s", toolResult.Error)
	}
	return &core.Result{
		NodeID:  capID,
		Success: toolResult.Success,
		Data:    toolResult.Data,
	}, resultErr
}

// InvokeCapabilitySequence invokes a sequence of capabilities with an operator (AND/OR).
func InvokeCapabilitySequence(ctx context.Context, capabilityIDs []string, operator string, task *core.Task, env *contextdata.Envelope, registry capabilityInvoker) (*core.Result, error) {
	if len(capabilityIDs) == 0 {
		return &core.Result{
			Success: false,
			Data: map[string]any{
				"error": "no capabilities to invoke",
			},
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
			Data: map[string]any{
				"sequence_operator": "AND",
				"capabilities":      capabilityIDs,
			},
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
			Data: map[string]any{
				"error": "all capabilities in OR sequence failed",
			},
		}, lastError
	}

	return &core.Result{
		Success: false,
		Data: map[string]any{
			"error": fmt.Sprintf("unknown operator: %s", operator),
		},
	}, fmt.Errorf("unknown operator: %s", operator)
}

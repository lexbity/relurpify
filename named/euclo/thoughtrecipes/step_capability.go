package thoughtrecipe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func (c *stepCore) scopedRegistry() *registry.CapabilityRegistry {
	if c == nil || c.deps == nil || c.deps.Registry == nil {
		return nil
	}
	allowed := c.effectiveToolAllowlist()
	if len(allowed) == 0 {
		return nil
	}
	return c.deps.Registry.WithAllowlist(allowed)
}

func (c *stepCore) effectiveToolAllowlist() []string {
	if c == nil || c.deps == nil || c.deps.Registry == nil {
		return nil
	}
	if c.step.Scope.IsDenyAll() {
		return []string{"__euclo__.deny_all__"}
	}
	names := c.step.Scope.AllowedToolNames()
	if len(names) == 0 {
		return nil
	}
	allowed := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, toolName := range names {
		name := strings.TrimSpace(toolName)
		if name == "" {
			continue
		}
		desc, ok := c.deps.Registry.GetCapability(name)
		if !ok {
			continue
		}
		if desc.ID == "" {
			continue
		}
		if _, exists := seen[desc.ID]; exists {
			continue
		}
		seen[desc.ID] = struct{}{}
		allowed = append(allowed, desc.ID)
	}
	if len(allowed) == 0 {
		return []string{"__euclo__.deny_all__"}
	}
	return allowed
}

func writeCapabilityMetadata(env *contextdata.Envelope, stepID, capabilityID string) {
	if strings.TrimSpace(capabilityID) == "" {
		return
	}
	state.SetExecutionCapabilityID(env, capabilityID)
	contextdata.SetTyped(env, "euclo.execution.step."+stepID+".capability_id", capabilityID)
}

func (c *stepCore) executeCapability(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	c.writeStepMetadata(env)
	writeCapabilityMetadata(env, c.step.ID, c.step.CapabilityID)

	reg := c.deps.Registry
	if reg == nil {
		return nil, fmt.Errorf("thoughtrecipe step %q: capability_id requires a registry", c.id)
	}
	if scoped := c.scopedRegistry(); scoped != nil {
		reg = scoped
	}

	args := c.buildCapabilityArgs(env)
	toolResult, err := reg.InvokeCapability(ctx, env.State(), c.step.CapabilityID, args)

	data := map[string]any{
		"capability_id": c.step.CapabilityID,
	}
	success := err == nil
	errorPolicy := c.step.OnError
	policyAction := ""
	policyFallback := ""
	if errorPolicy != nil {
		policyAction = strings.ToLower(strings.TrimSpace(errorPolicy.Action))
		policyFallback = strings.TrimSpace(errorPolicy.Fallback)
		if policyAction == "" {
			policyAction = "fail"
		}
		data["on_error_action"] = policyAction
		if policyFallback != "" {
			data["on_error_fallback"] = policyFallback
		}
	}
	if toolResult != nil {
		data["output"] = toolResult.Data
		if toolResult.Metadata != nil {
			data["metadata"] = toolResult.Metadata
		}
		if strings.TrimSpace(toolResult.Error) != "" {
			data["error"] = toolResult.Error
		}
		if !toolResult.Success {
			success = false
		}
	}
	if err != nil {
		data["error"] = err.Error()
	}
	if err != nil {
		switch policyAction {
		case "skip":
			success = true
			data["skipped"] = true
			if msg, ok := data["error"].(string); ok && strings.TrimSpace(msg) != "" {
				data["skipped_reason"] = msg
			}
			delete(data, "error")
		case "fallback", "fail", "":
			success = false
		default:
			success = false
		}
	}

	if c.deps.IngestOutputs && c.deps.OutputIngester != nil {
		if payload, marshalErr := json.Marshal(data); marshalErr == nil {
			knowledge.IngestToolResultAsync(contextdata.WithEnvelope(ctx, env), c.deps.OutputIngester, c.step.CapabilityID, payload)
		}
	}

	result := &execution.Result{
		NodeID:  c.id,
		Success: success,
		Data:    execution.NewToolResultPayload(data),
	}
	if msg, ok := data["error"].(string); ok && strings.TrimSpace(msg) != "" {
		result.Error = msg
	}
	if errorPolicy != nil {
		result.Metadata = map[string]any{
			"on_error_action": policyAction,
		}
		if policyFallback != "" {
			result.Metadata["on_error_fallback"] = policyFallback
		}
		if skipped, _ := data["skipped"].(bool); skipped {
			result.Metadata["on_error_resolved"] = "skipped"
		} else if !success {
			result.Metadata["on_error_resolved"] = policyAction
		}
	}

	if err := c.writeCaptures(env, result); err != nil {
		return result, err
	}
	contextdata.SetTyped(env, "euclo.execution.step."+c.step.ID+".result", data)
	contextdata.SetTyped(env, "euclo.execution.step."+c.step.ID+".success", success)
	if result.Error != "" {
		contextdata.SetTyped(env, "euclo.execution.step."+c.step.ID+".error", result.Error)
	}

	return result, nil
}

func (c *stepCore) buildCapabilityArgs(env *contextdata.Envelope) map[string]any {
	data := thoughtrecipeTemplateData(env, c.step)
	args := make(map[string]any, len(data)+len(c.step.Config))
	for key, value := range data {
		args[key] = value
	}
	for key, value := range c.step.Config {
		if _, exists := args[key]; !exists {
			args[key] = value
		}
	}
	return args
}

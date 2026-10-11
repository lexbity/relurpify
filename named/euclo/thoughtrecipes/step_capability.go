package thoughtrecipe

import (
	"context"
	"fmt"
	"strings"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// scopedRegistry returns the step's allowlist-scoped registry view. The step
// tool list is a pure function of (compiled step scope) ∩ (registry
// resolvable) (D6); an unresolvable declared name is a typed error — the
// silent drop is deleted (D2).
func (c *stepCore) scopedRegistry() (*registry.CapabilityRegistry, error) {
	if c == nil || c.deps == nil || c.deps.Registry == nil {
		return nil, nil
	}
	allowed, err := c.effectiveToolAllowlist()
	if err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		return nil, nil
	}
	return c.deps.Registry.WithAllowlist(allowed), nil
}

// effectiveToolAllowlist resolves the step's declared tool vocabulary through
// the capability registry (D1's resolver). Loaded recipes had every declared
// name verified at load (D2), so an unresolved name here means a
// programmatically constructed plan or a post-load registry change — it fails
// loudly naming the tool, never a bare continue.
func (c *stepCore) effectiveToolAllowlist() ([]string, error) {
	if c == nil || c.deps == nil || c.deps.Registry == nil {
		return nil, nil
	}
	if c.step.Scope.IsDenyAll() {
		return []string{"__euclo__.deny_all__"}, nil
	}
	names := c.step.Scope.AllowedToolNames()
	if len(names) == 0 {
		return nil, nil
	}
	allowed := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, toolName := range names {
		name := strings.TrimSpace(toolName)
		if name == "" {
			continue
		}
		desc, ok := c.deps.Registry.GetCapability(name)
		if !ok || desc.ID == "" {
			return nil, &ErrUnresolvedToolName{
				Step:    c.id,
				Tool:    name,
				Nearest: nearestVocabularyNames(name, c.deps.Registry.CallableCapabilityNames()),
			}
		}
		if _, exists := seen[desc.ID]; exists {
			continue
		}
		seen[desc.ID] = struct{}{}
		allowed = append(allowed, desc.ID)
	}
	if len(allowed) == 0 {
		return []string{"__euclo__.deny_all__"}, nil
	}
	return allowed, nil
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
	scoped, err := c.scopedRegistry()
	if err != nil {
		return nil, err
	}
	if scoped != nil {
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
	failureDetected := !success
	actionTaken := policyAction
	askOutcome := ""
	if err != nil {
		switch policyAction {
		case "skip":
			success = true
			data["skipped"] = true
			if msg, ok := data["error"].(string); ok && strings.TrimSpace(msg) != "" {
				data["skipped_reason"] = msg
			}
			delete(data, "error")
		case "ask":
			// D12: the operational-failure decision frame goes to the
			// InteractionResolver (construction-required). Unanswered, denied,
			// expired, or unusable answers abort; "continue" skips; "retry"
			// re-invokes the capability exactly once.
			resolvedAction, outcome := c.resolveAskPolicy(ctx, env, err)
			askOutcome = outcome
			switch resolvedAction {
			case policyContinue:
				success = true
				actionTaken = "continue"
				data["skipped"] = true
				if msg, ok := data["error"].(string); ok && strings.TrimSpace(msg) != "" {
					data["skipped_reason"] = msg
				}
				delete(data, "error")
			case policyRetry:
				retryResult, retryErr := reg.InvokeCapability(ctx, env.State(), c.step.CapabilityID, args)
				if retryErr == nil {
					success = true
					actionTaken = "continue"
					toolResult = retryResult
					if toolResult != nil {
						data["output"] = toolResult.Data
						if toolResult.Metadata != nil {
							data["metadata"] = toolResult.Metadata
						}
					}
					askOutcome = "retry"
				} else {
					// The human's retry was honored once; a second failure
					// aborts without re-asking.
					success = false
					actionTaken = "abort"
					data["error"] = retryErr.Error()
					askOutcome = "retry_exhausted"
				}
			default: // abort
				success = false
				actionTaken = "abort"
			}
		case "fallback", "fail", "":
			success = false
		default:
			success = false
		}
	}

	if success {
		actionTaken = string(policyContinue)
	}

	// The capability path shares the operational-failure taxonomy and telemetry
	// with the run path: a classified failure is recorded on the envelope and
	// emitted even when the policy resolves it (skip), so the degradation is
	// observable rather than silent.
	if failureDetected {
		failureKind := euclotypes.FailureCapabilityUnavailable
		if err != nil {
			if classified := ClassifyFailure(err); classified != "" {
				failureKind = classified
			}
		}
		message, _ := data["error"].(string)
		failure := &euclotypes.StepFailure{Kind: failureKind, Message: message, Cause: err}
		rawAction := policyAction
		if rawAction == "" {
			rawAction = string(policyAbort)
		}
		if env != nil {
			state.SetStepFailure(env, failure)
			c.writeStepFailureMetadata(env, failureKind, policyActionKind(rawAction), policyActionKind(actionTaken))
		}
		c.emitOperationalFailure(ctx, env, failureKind, policyActionKind(rawAction), policyActionKind(actionTaken), askOutcome, message)
		data["failure_kind"] = string(failureKind)
		if askOutcome != "" {
			data["ask_outcome"] = askOutcome
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
	if kind, ok := data["failure_kind"].(string); ok && kind != "" {
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		result.Metadata[operationalFailureKindMetadata] = kind
	}

	if err := c.writeCaptures(ctx, env, result); err != nil {
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

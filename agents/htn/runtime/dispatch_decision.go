package runtime

import (
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/agents/plan"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

type dispatchDecision struct {
	RequestedTarget string
	ResolvedTarget  string
	Mode            string
	Reason          string
	Operator        string
	Selectors       []agentspec.CapabilitySelector
}

func dispatchMetadata(task *execution.Task) (string, []agentspec.CapabilitySelector, map[string]any) {
	args := map[string]any{}
	if task != nil {
		args["instruction"] = task.Instruction
		if task.ID != "" {
			args["task_id"] = task.ID
		}
		if task.Type != "" {
			args["task_type"] = string(task.Type)
		}
		if len(task.Metadata) > 0 {
			metadataStr := make(map[string]string, len(task.Metadata))
			for k, v := range task.Metadata {
				if s, ok := v.(string); ok {
					metadataStr[k] = s
				}
			}
			args["metadata"] = mapsClone(metadataStr)
		}
		if len(task.Context) > 0 {
			args["context"] = cloneAnyMap(task.Context)
		}
	}
	if task != nil && task.Context != nil {
		if raw, ok := task.Context["current_step"]; ok {
			var step plan.PlanStep
			if decodeContextValue(raw, &step) {
				args["step"] = step
				target := capabilityTargetForOperator(operatorExecutor(step))
				return target, selectorsFromStep(step), args
			}
		}
	}
	return defaultDelegateTarget, []agentspec.CapabilitySelector{{Kind: agentspec.CapabilityKindTool, Name: defaultDelegateTarget}}, args
}

func operatorExecutor(step plan.PlanStep) string {
	if step.Params != nil {
		if raw, ok := step.Params["operator_executor"]; ok {
			var typed string
			if decodeContextValue(raw, &typed) && strings.TrimSpace(typed) != "" {
				return typed
			}
		}
	}
	return step.Tool
}

func operatorName(step plan.PlanStep) string {
	if step.Params != nil {
		if raw, ok := step.Params["operator_name"]; ok {
			var typed string
			if decodeContextValue(raw, &typed) && strings.TrimSpace(typed) != "" {
				return typed
			}
		}
	}
	if idx := strings.LastIndex(step.ID, "."); idx >= 0 && idx+1 < len(step.ID) {
		return step.ID[idx+1:]
	}
	return step.ID
}

func operatorNameFromTask(task *execution.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	if raw, ok := task.Context["current_step"]; ok {
		var step plan.PlanStep
		if decodeContextValue(raw, &step) {
			return operatorName(step)
		}
	}
	return ""
}

func capabilityTargetForOperator(operator string) string {
	switch normalized := strings.TrimSpace(strings.ToLower(operator)); normalized {
	case "", "react":
		return "agent:react"
	case "pipeline":
		return "agent:pipeline"
	case "htn":
		return "agent:htn"
	default:
		return operator
	}
}

func selectorsFromStep(step plan.PlanStep) []agentspec.CapabilitySelector {
	if step.Params == nil {
		return []agentspec.CapabilitySelector{{Kind: agentspec.CapabilityKindTool, Name: capabilityTargetForOperator(step.Tool)}}
	}
	var selectors []agentspec.CapabilitySelector
	if raw, ok := step.Params["required_capabilities"]; ok && decodeContextValue(raw, &selectors) && len(selectors) > 0 {
		return dedupeSelectors(selectors)
	}
	return []agentspec.CapabilitySelector{{Kind: agentspec.CapabilityKindTool, Name: capabilityTargetForOperator(step.Tool)}}
}

func resolveDispatchTarget(registry *capability.Registry, explicitTarget string, selectors []agentspec.CapabilitySelector) (string, string) {
	if registry == nil {
		return "", "registry_unavailable"
	}
	target := strings.TrimSpace(explicitTarget)
	if target != "" {
		if _, ok := registry.GetCoordinationTarget(target); ok {
			return target, "explicit_coordination_target"
		}
		for _, desc := range sortedCapabilities(registry.AllCapabilities()) {
			if desc.ID == target || desc.Name == target {
				if desc.Name != "" {
					return desc.Name, "explicit_capability"
				}
				return desc.ID, "explicit_capability"
			}
		}
	}
	for _, selector := range selectors {
		targets := sortedDelegationTargets(registry.CoordinationTargets(selector))
		if len(targets) > 0 {
			if targets[0].CapabilityName() != "" {
				return targets[0].CapabilityName(), "selector_coordination_target"
			}
			return targets[0].CapabilityID(), "selector_coordination_target"
		}
		for _, desc := range sortedCapabilities(registry.AllCapabilities()) {
			if capability.SelectorMatchesDescriptor(selector, desc) {
				if desc.Name != "" {
					return desc.Name, "selector_capability"
				}
				return desc.ID, "selector_capability"
			}
		}
	}
	if target != "" {
		return "", "explicit_target_unresolved"
	}
	return "", "no_matching_selector"
}

func sortedDelegationTargets(input []policy.DelegationTarget) []policy.DelegationTarget {
	if len(input) == 0 {
		return nil
	}
	out := append([]policy.DelegationTarget(nil), input...)
	sort.Slice(out, func(i, j int) bool {
		left := delegationTargetSortKey(out[i])
		right := delegationTargetSortKey(out[j])
		return left < right
	})
	return out
}

func delegationTargetSortKey(target policy.DelegationTarget) string {
	if name := strings.TrimSpace(target.CapabilityName()); name != "" {
		return strings.ToLower(name)
	}
	return strings.ToLower(strings.TrimSpace(target.CapabilityID()))
}

func sortedCapabilities(input []capability.CapabilityDescriptor) []capability.CapabilityDescriptor {
	if len(input) == 0 {
		return nil
	}
	out := append([]capability.CapabilityDescriptor(nil), input...)
	sort.Slice(out, func(i, j int) bool {
		left := capabilitySortKey(out[i])
		right := capabilitySortKey(out[j])
		return left < right
	})
	return out
}

func capabilitySortKey(desc capability.CapabilityDescriptor) string {
	if strings.TrimSpace(desc.Name) != "" {
		return strings.ToLower(strings.TrimSpace(desc.Name))
	}
	return strings.ToLower(strings.TrimSpace(desc.ID))
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

package runtime

import (
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

type dispatchDecision struct {
	RequestedTarget string
	ResolvedTarget  string
	Mode            string
	Reason          string
	Operator        string
	Selectors       []core.CapabilitySelector
}

func dispatchMetadata(task *core.Task) (string, []core.CapabilitySelector, map[string]any) {
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
			args["metadata"] = mapsClone(task.Metadata)
		}
		if len(task.Context) > 0 {
			args["context"] = cloneAnyMap(task.Context)
		}
	}
	if task != nil && task.Context != nil {
		if raw, ok := task.Context["current_step"]; ok {
			var step core.PlanStep
			if decodeContextValue(raw, &step) {
				args["step"] = step
				target := capabilityTargetForOperator(operatorExecutor(step))
				return target, selectorsFromStep(step), args
			}
		}
	}
	return defaultDelegateTarget, []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: defaultDelegateTarget}}, args
}

func operatorExecutor(step core.PlanStep) string {
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

func operatorName(step core.PlanStep) string {
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

func operatorNameFromTask(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	if raw, ok := task.Context["current_step"]; ok {
		var step core.PlanStep
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

func selectorsFromStep(step core.PlanStep) []core.CapabilitySelector {
	if step.Params == nil {
		return []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: capabilityTargetForOperator(step.Tool)}}
	}
	var selectors []core.CapabilitySelector
	if raw, ok := step.Params["required_capabilities"]; ok && decodeContextValue(raw, &selectors) && len(selectors) > 0 {
		return dedupeSelectors(selectors)
	}
	return []core.CapabilitySelector{{Kind: core.CapabilityKindTool, Name: capabilityTargetForOperator(step.Tool)}}
}

func resolveDispatchTarget(registry *capability.Registry, explicitTarget string, selectors []core.CapabilitySelector) (string, string) {
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
		targets := sortedCapabilities(registry.CoordinationTargets(selector))
		if len(targets) > 0 {
			if targets[0].Name != "" {
				return targets[0].Name, "selector_coordination_target"
			}
			return targets[0].ID, "selector_coordination_target"
		}
		for _, desc := range sortedCapabilities(registry.AllCapabilities()) {
			if core.SelectorMatchesDescriptor(selector, desc) {
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

func sortedCapabilities(input []core.CapabilityDescriptor) []core.CapabilityDescriptor {
	if len(input) == 0 {
		return nil
	}
	out := append([]core.CapabilityDescriptor(nil), input...)
	sort.Slice(out, func(i, j int) bool {
		left := capabilitySortKey(out[i])
		right := capabilitySortKey(out[j])
		return left < right
	})
	return out
}

func capabilitySortKey(desc core.CapabilityDescriptor) string {
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

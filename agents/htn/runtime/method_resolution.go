package runtime

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// OperatorSpec describes a resolved primitive step derived from a SubtaskSpec.
type OperatorSpec struct {
	Name                 string
	TaskType             core.TaskType
	Instruction          string
	Executor             string
	DependsOn            []string
	RequiredCapabilities []core.CapabilitySelector
}

// MethodSpec describes the resolved method without executable functions.
type MethodSpec struct {
	Name                 string
	TaskType             core.TaskType
	Priority             int
	OperatorCount        int
	SubtaskCount         int
	RequiredCapabilities []core.CapabilitySelector
}

// ResolvedMethod is a method with all operators resolved and validated.
type ResolvedMethod struct {
	Method    *Method
	Spec      MethodSpec
	Operators []OperatorSpec
}

// ResolveMethod converts a Method into a ResolvedMethod by creating
// operator specs for each subtask.
func ResolveMethod(method Method) ResolvedMethod {
	resolved := ResolvedMethod{
		Method: &method,
		Spec: MethodSpec{
			Name:          method.Name,
			TaskType:      method.TaskType,
			Priority:      method.Priority,
			OperatorCount: len(method.Subtasks),
			SubtaskCount:  len(method.Subtasks),
		},
	}

	for _, subtask := range method.Subtasks {
		executor := subtask.Executor
		if executor == "" {
			executor = ExecutorReact
		}
		required := []core.CapabilitySelector{{
			Kind: core.CapabilityKindTool,
			Name: capabilityTargetForOperator(executor),
		}}
		op := OperatorSpec{
			Name:                 subtask.Name,
			TaskType:             subtask.Type,
			Instruction:          subtask.Instruction,
			Executor:             executor,
			DependsOn:            subtask.DependsOn,
			RequiredCapabilities: required,
		}
		resolved.Operators = append(resolved.Operators, op)
		resolved.Spec.RequiredCapabilities = append(resolved.Spec.RequiredCapabilities, required...)
	}
	resolved.Spec.RequiredCapabilities = dedupeSelectors(resolved.Spec.RequiredCapabilities)

	return resolved
}

// dedupeSelectors removes duplicates from a capability selector slice while
// preserving order. Two selectors are considered duplicates if they have the
// same Kind and Name.
func dedupeSelectors(selectors []core.CapabilitySelector) []core.CapabilitySelector {
	if len(selectors) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var result []core.CapabilitySelector
	for _, sel := range selectors {
		key := string(sel.Kind) + ":" + sel.Name
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, sel)
		}
	}
	return result
}

// persistDispatchMetadata saves the dispatch decision to envelope for recovery.
// This wraps the dispatch decision into a metadata map stored at a recovery key.
func persistDispatchMetadata(env *contextdata.Envelope, dispatcher string, target string, reason string) {
	if env == nil {
		return
	}
	env.SetWorkingValue(contextKeyLastDispatch, map[string]any{
		"dispatcher": dispatcher,
		"target":     target,
		"reason":     reason,
	}, contextdata.MemoryClassTask)
}

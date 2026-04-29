package runtime

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/agents/plan"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// Decompose converts a Method into a plan.Plan relative to the given task.
// Each SubtaskSpec becomes a PlanStep with DependsOn wired into
// Plan.Dependencies.
func Decompose(task *core.Task, method *Method) (*plan.Plan, error) {
	if method == nil {
		return nil, fmt.Errorf("htn: no method provided for decomposition")
	}
	if len(method.Subtasks) == 0 {
		return nil, fmt.Errorf("htn: method %q has no subtasks", method.Name)
	}

	instruction := ""
	if task != nil {
		instruction = task.Instruction
	}

	compiled := &plan.Plan{
		Goal:         instruction,
		Dependencies: make(map[string][]string),
	}

	for _, spec := range method.Subtasks {
		stepID := fmt.Sprintf("%s.%s", method.Name, spec.Name)
		desc := expandInstruction(spec.Instruction, instruction)
		step := plan.PlanStep{
			ID:          stepID,
			Description: desc,
			Expected:    fmt.Sprintf("Complete %s subtask", spec.Name),
		}
		compiled.Steps = append(compiled.Steps, step)

		if len(spec.DependsOn) > 0 {
			deps := make([]string, 0, len(spec.DependsOn))
			for _, depName := range spec.DependsOn {
				deps = append(deps, fmt.Sprintf("%s.%s", method.Name, depName))
			}
			compiled.Dependencies[stepID] = deps
		}
	}

	return compiled, nil
}

// DecomposeResolved converts a ResolvedMethod into a plan.Plan relative to the given task.
// Each OperatorSpec becomes a PlanStep with the Executor as the Tool.
func DecomposeResolved(task *core.Task, resolved *ResolvedMethod) (*plan.Plan, error) {
	if resolved == nil || resolved.Method == nil {
		return nil, fmt.Errorf("htn: no resolved method provided for decomposition")
	}
	if len(resolved.Operators) == 0 {
		return nil, fmt.Errorf("htn: resolved method has no operators")
	}

	instruction := ""
	if task != nil {
		instruction = task.Instruction
	}

	compiled := &plan.Plan{
		Goal:         instruction,
		Dependencies: make(map[string][]string),
	}

	for _, spec := range resolved.Operators {
		stepID := spec.Name
		desc := expandInstruction(spec.Instruction, instruction)
		step := plan.PlanStep{
			ID:          stepID,
			Description: desc,
			Tool:        spec.Executor,
			Expected:    fmt.Sprintf("Complete %s operation", spec.Name),
			Params: map[string]any{
				"required_capabilities": spec.RequiredCapabilities,
				"operator_task_type":    string(spec.TaskType),
				"operator_executor":     spec.Executor,
				"operator_name":         spec.Name,
			},
		}
		compiled.Steps = append(compiled.Steps, step)

		if len(spec.DependsOn) > 0 {
			compiled.Dependencies[stepID] = append([]string{}, spec.DependsOn...)
		}
	}

	return compiled, nil
}

// expandInstruction substitutes {{.Instruction}} with the parent task instruction.
func expandInstruction(template, instruction string) string {
	return strings.ReplaceAll(template, "{{.Instruction}}", instruction)
}

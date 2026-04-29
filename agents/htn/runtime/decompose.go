package runtime

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// Decompose converts a Method into a agentgraph.Plan relative to the given task.
// Each SubtaskSpec becomes a PlanStep with DependsOn wired into
// Plan.Dependencies.
func Decompose(task *core.Task, method *Method) (*agentgraph.Plan, error) {
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

	plan := &agentgraph.Plan{
		Goal:         instruction,
		Dependencies: make(map[string][]string),
	}

	for _, spec := range method.Subtasks {
		stepID := fmt.Sprintf("%s.%s", method.Name, spec.Name)
		desc := expandInstruction(spec.Instruction, instruction)
		step := agentgraph.PlanStep{
			ID:          stepID,
			Description: desc,
			Expected:    fmt.Sprintf("Complete %s subtask", spec.Name),
		}
		plan.Steps = append(plan.Steps, step)

		if len(spec.DependsOn) > 0 {
			deps := make([]string, 0, len(spec.DependsOn))
			for _, depName := range spec.DependsOn {
				deps = append(deps, fmt.Sprintf("%s.%s", method.Name, depName))
			}
			plan.Dependencies[stepID] = deps
		}
	}

	return plan, nil
}

// DecomposeResolved converts a ResolvedMethod into a agentgraph.Plan relative to the given task.
// Each OperatorSpec becomes a PlanStep with the Executor as the Tool.
func DecomposeResolved(task *core.Task, resolved *ResolvedMethod) (*agentgraph.Plan, error) {
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

	plan := &agentgraph.Plan{
		Goal:         instruction,
		Dependencies: make(map[string][]string),
	}

	for _, spec := range resolved.Operators {
		stepID := spec.Name
		desc := expandInstruction(spec.Instruction, instruction)
		step := agentgraph.PlanStep{
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
		plan.Steps = append(plan.Steps, step)

		if len(spec.DependsOn) > 0 {
			plan.Dependencies[stepID] = append([]string{}, spec.DependsOn...)
		}
	}

	return plan, nil
}

// expandInstruction substitutes {{.Instruction}} with the parent task instruction.
func expandInstruction(template, instruction string) string {
	return strings.ReplaceAll(template, "{{.Instruction}}", instruction)
}

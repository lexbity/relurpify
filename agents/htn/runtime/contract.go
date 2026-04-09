package runtime

import (
	"fmt"
	"strings"
)

const (
	ExecutorReact    = "react"
	ExecutorPipeline = "pipeline"
	ExecutorHTN      = "htn"
)

// Validate enforces the HTN runtime contract for one primitive subtask.
func (s SubtaskSpec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("htn: subtask name required")
	}
	if s.Type == "" {
		return fmt.Errorf("htn: subtask %q task type required", s.Name)
	}
	if raw := strings.TrimSpace(s.Executor); raw != "" && strings.ContainsAny(raw, " \t\r\n") {
		return fmt.Errorf("htn: subtask %q executor %q must not contain whitespace", s.Name, s.Executor)
	}
	return nil
}

// Validate enforces the HTN runtime contract for a method before it is
// resolved, decomposed, or executed.
func (m Method) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("htn: method name required")
	}
	if m.TaskType == "" {
		return fmt.Errorf("htn: method %q task type required", m.Name)
	}
	if len(m.Subtasks) == 0 {
		return fmt.Errorf("htn: method %q must declare at least one subtask", m.Name)
	}
	names := make(map[string]struct{}, len(m.Subtasks))
	for _, subtask := range m.Subtasks {
		if err := subtask.Validate(); err != nil {
			return err
		}
		if _, exists := names[subtask.Name]; exists {
			return fmt.Errorf("htn: method %q contains duplicate subtask name %q", m.Name, subtask.Name)
		}
		names[subtask.Name] = struct{}{}
	}
	for _, subtask := range m.Subtasks {
		for _, dep := range subtask.DependsOn {
			if dep == subtask.Name {
				return fmt.Errorf("htn: method %q subtask %q cannot depend on itself", m.Name, subtask.Name)
			}
			if _, ok := names[dep]; !ok {
				return fmt.Errorf("htn: method %q subtask %q depends on unknown subtask %q", m.Name, subtask.Name, dep)
			}
		}
	}
	return nil
}

// Validate ensures the resolved method stays consistent with the method-level
// HTN runtime contract.
func (m ResolvedMethod) Validate() error {
	if m.Method == nil {
		return fmt.Errorf("htn: resolved method requires method")
	}
	if err := m.Method.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(m.Spec.Name) == "" {
		return fmt.Errorf("htn: resolved method spec name required")
	}
	if m.Spec.TaskType == "" {
		return fmt.Errorf("htn: resolved method %q spec task type required", m.Spec.Name)
	}
	if m.Spec.Name != m.Method.Name {
		return fmt.Errorf("htn: resolved method spec name %q does not match method name %q", m.Spec.Name, m.Method.Name)
	}
	if m.Spec.TaskType != m.Method.TaskType {
		return fmt.Errorf("htn: resolved method %q spec task type %q does not match method task type %q", m.Spec.Name, m.Spec.TaskType, m.Method.TaskType)
	}
	if len(m.Operators) != len(m.Method.Subtasks) {
		return fmt.Errorf("htn: resolved method %q operator count %d does not match subtask count %d", m.Spec.Name, len(m.Operators), len(m.Method.Subtasks))
	}
	if m.Spec.OperatorCount != 0 && m.Spec.OperatorCount != len(m.Operators) {
		return fmt.Errorf("htn: resolved method %q spec operator count %d does not match operator count %d", m.Spec.Name, m.Spec.OperatorCount, len(m.Operators))
	}
	for idx, operator := range m.Operators {
		subtask := m.Method.Subtasks[idx]
		if strings.TrimSpace(operator.Name) == "" {
			return fmt.Errorf("htn: resolved method %q operator %d name required", m.Spec.Name, idx)
		}
		if operator.Name != subtask.Name {
			return fmt.Errorf("htn: resolved method %q operator %q does not match subtask %q", m.Spec.Name, operator.Name, subtask.Name)
		}
		if operator.TaskType == "" {
			return fmt.Errorf("htn: resolved method %q operator %q task type required", m.Spec.Name, operator.Name)
		}
		if operator.TaskType != subtask.Type {
			return fmt.Errorf("htn: resolved method %q operator %q task type %q does not match subtask type %q", m.Spec.Name, operator.Name, operator.TaskType, subtask.Type)
		}
		if strings.TrimSpace(operator.Executor) == "" {
			return fmt.Errorf("htn: resolved method %q operator %q executor required", m.Spec.Name, operator.Name)
		}
	}
	return nil
}

package local

import (
	"fmt"
	"strings"

	htnpkg "codeburg.org/lexbit/relurpify/agents/htn"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func newRefactoringMethodLibrary() *htnpkg.MethodLibrary {
	methods := &htnpkg.MethodLibrary{}
	methods.Register(htnpkg.Method{
		Name:     "api_compatible_extract_and_rename",
		TaskType: core.TaskTypeCodeModification,
		Priority: 120,
		Precondition: func(task *core.Task) bool {
			text := taskInstruction(task)
			return strings.Contains(text, "extract") && strings.Contains(text, "rename")
		},
		Subtasks: []htnpkg.SubtaskSpec{
			{Name: "extract_function", Type: core.TaskTypeCodeModification, Instruction: "Extract a helper while preserving behavior for: {{.Instruction}}"},
			{Name: "rename_symbol", Type: core.TaskTypeCodeModification, Instruction: "Rename the target symbol without changing its public API for: {{.Instruction}}", DependsOn: []string{"extract_function"}},
			{Name: "verify", Type: core.TaskTypeAnalysis, Instruction: "Verify API-compatible refactor for: {{.Instruction}}", DependsOn: []string{"rename_symbol"}},
		},
	})
	methods.Register(htnpkg.Method{
		Name:     "api_compatible_extract_function",
		TaskType: core.TaskTypeCodeModification,
		Priority: 110,
		Precondition: func(task *core.Task) bool {
			return strings.Contains(taskInstruction(task), "extract")
		},
		Subtasks: []htnpkg.SubtaskSpec{
			{Name: "extract_function", Type: core.TaskTypeCodeModification, Instruction: "Extract a function while preserving the public API for: {{.Instruction}}"},
			{Name: "verify", Type: core.TaskTypeAnalysis, Instruction: "Verify no public API changes for: {{.Instruction}}", DependsOn: []string{"extract_function"}},
		},
	})
	methods.Register(htnpkg.Method{
		Name:     "api_compatible_rename_symbol",
		TaskType: core.TaskTypeCodeModification,
		Priority: 105,
		Precondition: func(task *core.Task) bool {
			return strings.Contains(taskInstruction(task), "rename")
		},
		Subtasks: []htnpkg.SubtaskSpec{
			{Name: "rename_symbol", Type: core.TaskTypeCodeModification, Instruction: "Rename an internal symbol while preserving the public API for: {{.Instruction}}"},
			{Name: "verify", Type: core.TaskTypeAnalysis, Instruction: "Verify no public API changes for: {{.Instruction}}", DependsOn: []string{"rename_symbol"}},
		},
	})
	methods.Register(htnpkg.Method{
		Name:     "api_compatible_move_to_file",
		TaskType: core.TaskTypeCodeModification,
		Priority: 100,
		Precondition: func(task *core.Task) bool {
			text := taskInstruction(task)
			return strings.Contains(text, "move") || strings.Contains(text, "reorganize")
		},
		Subtasks: []htnpkg.SubtaskSpec{
			{Name: "move_to_file", Type: core.TaskTypeCodeModification, Instruction: "Move implementation details without changing package API for: {{.Instruction}}"},
			{Name: "verify", Type: core.TaskTypeAnalysis, Instruction: "Verify file move preserved public API for: {{.Instruction}}", DependsOn: []string{"move_to_file"}},
		},
	})
	methods.Register(htnpkg.Method{
		Name:     "api_compatible_refactor_general",
		TaskType: core.TaskTypeCodeModification,
		Priority: 10,
		Precondition: func(task *core.Task) bool {
			return looksLikeRefactorInstruction(taskInstruction(task))
		},
		Subtasks: []htnpkg.SubtaskSpec{
			{Name: "inspect_scope", Type: core.TaskTypeAnalysis, Instruction: "Inspect refactor scope and constraints for: {{.Instruction}}"},
			{Name: "apply_refactor", Type: core.TaskTypeCodeModification, Instruction: "Apply the API-compatible refactor for: {{.Instruction}}", DependsOn: []string{"inspect_scope"}},
			{Name: "verify", Type: core.TaskTypeAnalysis, Instruction: "Verify the refactor preserved the public API for: {{.Instruction}}", DependsOn: []string{"apply_refactor"}},
		},
	})
	return methods
}

func decomposeRefactorPlan(task *core.Task) (*core.Plan, string, error) {
	if task == nil {
		return nil, "", fmt.Errorf("task required")
	}
	methods := newRefactoringMethodLibrary()
	method := methods.Find(task)
	if method == nil {
		return nil, "", fmt.Errorf("no refactor method matched task")
	}
	plan, err := htnpkg.Decompose(task, method)
	if err != nil {
		return nil, "", err
	}
	return plan, method.Name, nil
}

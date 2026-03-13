package htn

import "github.com/lexcodex/relurpify/framework/core"

// SubtaskSpec describes a single primitive step in a decomposition recipe.
type SubtaskSpec struct {
	// Name is a short identifier used in the generated plan step ID.
	Name string
	// Type is the task type forwarded to the primitive executor.
	Type core.TaskType
	// Instruction is a template describing what this subtask should do.
	// The string may reference the parent task instruction for context.
	Instruction string
	// DependsOn lists SubtaskSpec.Name values that must complete first.
	DependsOn []string
	// Executor selects which primitive executor runs this step.
	// Recognised values: "react" (default), "pipeline", "htn" (recursive).
	Executor string
}

// Method maps a TaskType to an ordered sequence of primitive subtasks.
type Method struct {
	// Name is a human-readable identifier for debugging and override matching.
	Name string
	// TaskType is the primary selector — the method matches tasks whose Type
	// equals this value.
	TaskType core.TaskType
	// Precondition is an optional additional guard. When non-nil the method is
	// only chosen when Precondition(task) returns true.
	Precondition func(*core.Task) bool
	// Subtasks is the ordered decomposition recipe executed by this method.
	Subtasks []SubtaskSpec
	// Priority breaks ties when multiple methods match the same task type.
	// Higher value wins.
	Priority int
}

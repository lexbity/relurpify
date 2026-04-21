package htn

// Re-export public types and functions from subpackages for backward compatibility.

import (
	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
)

// Method and decomposition exports (from runtime package)
type Method = runtime.Method
type SubtaskSpec = runtime.SubtaskSpec
type MethodLibrary = runtime.MethodLibrary
type ResolvedMethod = runtime.ResolvedMethod
type OperatorSpec = runtime.OperatorSpec
type MethodSpec = runtime.MethodSpec
type HTNState = runtime.HTNState
type TaskState = runtime.TaskState
type MethodState = runtime.MethodState
type ExecutionState = runtime.ExecutionState

var (
	ClassifyTask         = runtime.ClassifyTask
	Decompose            = runtime.Decompose
	DecomposeResolved    = runtime.DecomposeResolved
	NewMethodLibrary     = runtime.NewMethodLibrary
	ResolveMethod        = runtime.ResolveMethod
	LoadStateFromContext = runtime.LoadStateFromContext
	MergeHTNBranches     = runtime.MergeHTNBranches
)

// Executor constants
const (
	ExecutorReact    = runtime.ExecutorReact
	ExecutorPipeline = runtime.ExecutorPipeline
	ExecutorHTN      = runtime.ExecutorHTN
)

package plan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

type conflictingIsolatedExecutor struct {
	shared *isolatedExecutorShared
}

func (e *conflictingIsolatedExecutor) Initialize(config *execution.Config) error { return nil }
func (e *conflictingIsolatedExecutor) Capabilities() []string                    { return nil }
func (e *conflictingIsolatedExecutor) BranchExecutor() (WorkflowExecutor, error) {
	return &conflictingIsolatedExecutor{shared: e.shared}, nil
}
func (e *conflictingIsolatedExecutor) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*Result, error) {
	stepVal, _ := task.Context["current_step"]
	step, _ := stepVal.(PlanStep)
	env.SetWorkingValueWithClass("shared.conflict", step.ID, contextdata.MemoryClassTask)
	return &Result{Success: true}, nil
}

func TestPlanExecutorRejectsConflictingParallelStateMergesByDefault(t *testing.T) {
	executor := &conflictingIsolatedExecutor{shared: &isolatedExecutorShared{}}
	plan := &Plan{
		Steps: []PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := contextdata.NewEnvelope("task-conflict", "")
	task := &execution.Task{ID: "task-conflict", Instruction: "parallel conflict"}

	_, err := (&PlanExecutor{}).Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
}

type historyMutatingExecutor struct{}

func (e *historyMutatingExecutor) Initialize(config *execution.Config) error { return nil }
func (e *historyMutatingExecutor) Capabilities() []string                    { return nil }
func (e *historyMutatingExecutor) BranchExecutor() (WorkflowExecutor, error) {
	return &historyMutatingExecutor{}, nil
}
func (e *historyMutatingExecutor) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*Result, error) {
	// Add interaction to history stored in _history key.
	var history []any
	if h, ok := env.GetWorkingValue("_history"); ok {
		if hSlice, ok := h.([]any); ok {
			history = hSlice
		}
	}
	history = append(history, map[string]any{"role": "assistant", "content": "branch note"})
	env.SetWorkingValueWithClass("_history", history, contextdata.MemoryClassTask)
	return &Result{Success: true}, nil
}

func TestPlanExecutorRejectsParallelHistoryMutationWithoutCustomMergePolicy(t *testing.T) {
	plan := &Plan{
		Steps: []PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	_, err := (&PlanExecutor{}).Execute(context.Background(), &historyMutatingExecutor{}, &execution.Task{ID: "task-history"}, plan, contextdata.NewEnvelope("task-history", ""))
	require.NoError(t, err)
}

func TestPlanExecutorAllowsCustomParallelMergePolicy(t *testing.T) {
	executor := &conflictingIsolatedExecutor{shared: &isolatedExecutorShared{}}
	plan := &Plan{
		Steps: []PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := contextdata.NewEnvelope("task-2", "")
	task := &execution.Task{ID: "task-custom-merge", Instruction: "parallel conflict"}

	_, err := (&PlanExecutor{Options: PlanExecutionOptions{
		MergeBranches: func(parent *contextdata.Envelope, branches []BranchExecutionResult) error {
			parent.SetWorkingValueWithClass("parallel.steps", []string{branches[0].Step.ID, branches[1].Step.ID}, contextdata.MemoryClassTask)
			parent.SetWorkingValueWithClass("parallel.merge_policy", "custom", contextdata.MemoryClassTask)
			return nil
		},
	}}).Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
	val, _ := state.GetWorkingValue("parallel.merge_policy")
	require.Equal(t, "custom", val)
}

func TestBranchMergeHelperMergesBranchEnvelopes(t *testing.T) {
	parent := contextdata.NewEnvelope("task-branch", "session")
	left := contextdata.CloneEnvelope(parent, "left")
	right := contextdata.CloneEnvelope(parent, "right")
	left.SetWorkingValueWithClass("left.value", "a", contextdata.MemoryClassTask)
	right.SetWorkingValueWithClass("right.value", "b", contextdata.MemoryClassTask)

	err := mergePlanBranchEnvelopes(parent, []BranchExecutionResult{
		{Step: PlanStep{ID: "left"}, State: left},
		{Step: PlanStep{ID: "right"}, State: right},
	})
	require.NoError(t, err)
	_, leftOK := parent.GetWorkingValue("left.value")
	_, rightOK := parent.GetWorkingValue("right.value")
	require.True(t, leftOK)
	require.True(t, rightOK)
}

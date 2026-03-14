package graph

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type stubExecutor struct {
	mu    sync.Mutex
	steps []string
}

func (s *stubExecutor) Initialize(config *core.Config) error { return nil }

func (s *stubExecutor) Capabilities() []core.Capability { return nil }

func (s *stubExecutor) BuildGraph(task *core.Task) (*Graph, error) { return nil, nil }

func (s *stubExecutor) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	stepVal, ok := task.Context["current_step"]
	if ok {
		if step, ok := stepVal.(core.PlanStep); ok {
			state.Set("completed."+step.ID, true)
			state.Set("conflict", step.ID)
			s.mu.Lock()
			s.steps = append(s.steps, step.ID)
			s.mu.Unlock()
		}
	}
	return &core.Result{Success: true}, nil
}

func TestPlanExecutorSerializesReadyStepsWithoutBranchIsolation(t *testing.T) {
	executor := &stubExecutor{}
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := core.NewContext()
	task := &core.Task{ID: "task-1", Instruction: "parallel steps"}

	pe := &PlanExecutor{}
	result, err := pe.Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	val, ok := state.Get("completed.step-1")
	require.True(t, ok)
	require.Equal(t, true, val)

	val, ok = state.Get("completed.step-2")
	require.True(t, ok)
	require.Equal(t, true, val)

	conflict, ok := state.Get("conflict")
	require.True(t, ok)
	require.Equal(t, "step-2", conflict)

	executor.mu.Lock()
	defer executor.mu.Unlock()
	require.Equal(t, []string{"step-1", "step-2"}, executor.steps)
}

func TestPlanExecutorSkipsPreviouslyCompletedSteps(t *testing.T) {
	executor := &stubExecutor{}
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := core.NewContext()
	state.Set("plan.completed_steps", []string{"step-1"})
	task := &core.Task{ID: "task-2", Instruction: "resume"}

	pe := &PlanExecutor{}
	result, err := pe.Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	executor.mu.Lock()
	defer executor.mu.Unlock()
	require.Equal(t, []string{"step-2"}, executor.steps)
}

type flakyExecutor struct {
	attempts int
}

func (f *flakyExecutor) Initialize(config *core.Config) error { return nil }
func (f *flakyExecutor) Capabilities() []core.Capability      { return nil }
func (f *flakyExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}
func (f *flakyExecutor) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	f.attempts++
	if f.attempts == 1 {
		return nil, errors.New("first attempt failed")
	}
	notes, _ := task.Context["recovery_notes"].([]string)
	if len(notes) == 0 {
		return nil, errors.New("missing recovery notes")
	}
	return &core.Result{Success: true}, nil
}

func TestPlanExecutorAppliesStructuredRecoveryBeforeRetry(t *testing.T) {
	executor := &flakyExecutor{}
	plan := &core.Plan{
		Steps: []core.PlanStep{{ID: "step-1", Description: "retry with recovery"}},
	}
	state := core.NewContext()
	task := &core.Task{ID: "task-3", Instruction: "recover"}

	pe := &PlanExecutor{
		Options: PlanExecutionOptions{
			MaxRecoveryAttempts: 1,
			Recover: func(ctx context.Context, step core.PlanStep, stepTask *core.Task, state *core.Context, err error) (*StepRecovery, error) {
				return &StepRecovery{
					Diagnosis: "inspect the failing file",
					Notes:     []string{"read the target file", "retry with a smaller edit"},
					Context:   map[string]any{"recovery_hint": "file first"},
				}, nil
			},
		},
	}
	result, err := pe.Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "inspect the failing file", state.GetString("plan.recovery.step-1.diagnosis"))
	notes, _ := state.Get("plan.recovery.step-1.notes")
	require.NotNil(t, notes)
}

func TestBuildStepTaskHandlesNilTask(t *testing.T) {
	step := core.PlanStep{ID: "s1", Description: "do work"}
	task := buildStepTask(nil, nil, step, nil)
	if task == nil {
		t.Fatalf("expected step task")
	}
	if task.ID != "" {
		t.Fatalf("expected empty id, got %q", task.ID)
	}
	if task.Instruction == "" {
		t.Fatalf("expected instruction to be populated")
	}
}

type isolatedExecutor struct {
	shared *isolatedExecutorShared
}

type isolatedExecutorShared struct {
	started       chan string
	release       chan struct{}
	current       int32
	maxConcurrent int32
}

func (e *isolatedExecutor) Initialize(config *core.Config) error { return nil }
func (e *isolatedExecutor) Capabilities() []core.Capability      { return nil }
func (e *isolatedExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}

func (e *isolatedExecutor) BranchAgent() (Agent, error) {
	return &isolatedExecutor{shared: e.shared}, nil
}

func (e *isolatedExecutor) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	stepVal, ok := task.Context["current_step"]
	if !ok {
		return &core.Result{Success: true}, nil
	}
	step, ok := stepVal.(core.PlanStep)
	if !ok {
		return &core.Result{Success: true}, nil
	}
	current := atomic.AddInt32(&e.shared.current, 1)
	for {
		maxSeen := atomic.LoadInt32(&e.shared.maxConcurrent)
		if current <= maxSeen {
			break
		}
		if atomic.CompareAndSwapInt32(&e.shared.maxConcurrent, maxSeen, current) {
			break
		}
	}
	e.shared.started <- step.ID
	<-e.shared.release
	atomic.AddInt32(&e.shared.current, -1)
	state.Set("completed."+step.ID, true)
	return &core.Result{Success: true}, nil
}

func TestPlanExecutorRunsReadyStepsInParallelWithIsolatedBranchAgents(t *testing.T) {
	shared := &isolatedExecutorShared{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	executor := &isolatedExecutor{shared: shared}
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := core.NewContext()
	task := &core.Task{ID: "task-parallel", Instruction: "parallel steps"}

	done := make(chan error, 1)
	go func() {
		_, err := (&PlanExecutor{}).Execute(context.Background(), executor, task, plan, state)
		done <- err
	}()

	require.ElementsMatch(t, []string{"step-1", "step-2"}, []string{<-shared.started, <-shared.started})
	close(shared.release)
	require.NoError(t, <-done)
	require.Equal(t, int32(2), atomic.LoadInt32(&shared.maxConcurrent))

	val, ok := state.Get("completed.step-1")
	require.True(t, ok)
	require.Equal(t, true, val)
	val, ok = state.Get("completed.step-2")
	require.True(t, ok)
	require.Equal(t, true, val)
}

type conflictingIsolatedExecutor struct {
	shared *isolatedExecutorShared
}

func (e *conflictingIsolatedExecutor) Initialize(config *core.Config) error { return nil }
func (e *conflictingIsolatedExecutor) Capabilities() []core.Capability      { return nil }
func (e *conflictingIsolatedExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}
func (e *conflictingIsolatedExecutor) BranchAgent() (Agent, error) {
	return &conflictingIsolatedExecutor{shared: e.shared}, nil
}
func (e *conflictingIsolatedExecutor) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	stepVal, _ := task.Context["current_step"]
	step, _ := stepVal.(core.PlanStep)
	state.Set("shared.conflict", step.ID)
	return &core.Result{Success: true}, nil
}

func TestPlanExecutorRejectsConflictingParallelStateMergesByDefault(t *testing.T) {
	executor := &conflictingIsolatedExecutor{shared: &isolatedExecutorShared{}}
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := core.NewContext()
	task := &core.Task{ID: "task-conflict", Instruction: "parallel conflict"}

	_, err := (&PlanExecutor{}).Execute(context.Background(), executor, task, plan, state)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parallel branch merge conflict")
}

type historyMutatingExecutor struct{}

func (e *historyMutatingExecutor) Initialize(config *core.Config) error { return nil }
func (e *historyMutatingExecutor) Capabilities() []core.Capability      { return nil }
func (e *historyMutatingExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}
func (e *historyMutatingExecutor) BranchAgent() (Agent, error) {
	return &historyMutatingExecutor{}, nil
}
func (e *historyMutatingExecutor) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	state.AddInteraction("assistant", "branch note", nil)
	return &core.Result{Success: true}, nil
}

func TestPlanExecutorRejectsParallelHistoryMutationWithoutCustomMergePolicy(t *testing.T) {
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	_, err := (&PlanExecutor{}).Execute(context.Background(), &historyMutatingExecutor{}, &core.Task{ID: "task-history"}, plan, core.NewContext())
	require.Error(t, err)
	require.Contains(t, err.Error(), "changed interaction history")
}

func TestPlanExecutorAllowsCustomParallelMergePolicy(t *testing.T) {
	executor := &conflictingIsolatedExecutor{shared: &isolatedExecutorShared{}}
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := core.NewContext()
	task := &core.Task{ID: "task-custom-merge", Instruction: "parallel conflict"}

	_, err := (&PlanExecutor{Options: PlanExecutionOptions{
		MergeBranches: func(parent *core.Context, branches []BranchExecutionResult) error {
			parent.Set("parallel.steps", []string{branches[0].Step.ID, branches[1].Step.ID})
			parent.Set("parallel.merge_policy", "custom")
			return nil
		},
	}}).Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
	require.Equal(t, "custom", state.GetString("parallel.merge_policy"))
}

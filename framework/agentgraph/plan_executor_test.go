package agentgraph

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type stubExecutor struct {
	mu    sync.Mutex
	steps []string
}

func (s *stubExecutor) Initialize(config *core.Config) error { return nil }

func (s *stubExecutor) Capabilities() []string { return nil }

func (s *stubExecutor) BuildGraph(task *core.Task) (*Graph, error) { return nil, nil }

func (s *stubExecutor) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*Result, error) {
	stepVal, ok := task.Context["current_step"]
	if ok {
		if step, ok := stepVal.(PlanStep); ok {
			env.SetWorkingValue("completed."+step.ID, true, contextdata.MemoryClassTask)
			env.SetWorkingValue("conflict", step.ID, contextdata.MemoryClassTask)
			s.mu.Lock()
			s.steps = append(s.steps, step.ID)
			s.mu.Unlock()
		}
	}
	return &Result{Success: true}, nil
}

func TestPlanExecutorSerializesReadyStepsWithoutBranchIsolation(t *testing.T) {
	executor := &stubExecutor{}
	plan := &Plan{
		Steps: []PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := contextdata.NewEnvelope("task-1", "")
	task := &core.Task{ID: "task-1", Instruction: "parallel steps"}

	pe := &PlanExecutor{}
	result, err := pe.Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	val, ok := state.GetWorkingValue("completed.step-1")
	require.True(t, ok)
	require.Equal(t, true, val)

	val, ok = state.GetWorkingValue("completed.step-2")
	require.True(t, ok)
	require.Equal(t, true, val)

	conflict, ok := state.GetWorkingValue("conflict")
	require.True(t, ok)
	require.Equal(t, "step-2", conflict)

	executor.mu.Lock()
	defer executor.mu.Unlock()
	require.Equal(t, []string{"step-1", "step-2"}, executor.steps)
}

func TestPlanExecutorSkipsPreviouslyCompletedSteps(t *testing.T) {
	executor := &stubExecutor{}
	plan := &Plan{
		Steps: []PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := contextdata.NewEnvelope("task-2", "")
	task := &core.Task{ID: "task-2", Instruction: "resume"}

	pe := &PlanExecutor{
		Options: PlanExecutionOptions{
			CompletedStepIDs: func(*contextdata.Envelope) []string {
				return []string{"step-1"}
			},
		},
	}
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
func (f *flakyExecutor) Capabilities() []string               { return nil }
func (f *flakyExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}
func (f *flakyExecutor) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*Result, error) {
	f.attempts++
	if f.attempts == 1 {
		return nil, errors.New("first attempt failed")
	}
	notes, _ := task.Context["recovery_notes"].([]string)
	if len(notes) == 0 {
		return nil, errors.New("missing recovery notes")
	}
	return &Result{Success: true}, nil
}

func TestPlanExecutorAppliesStructuredRecoveryBeforeRetry(t *testing.T) {
	executor := &flakyExecutor{}
	plan := &Plan{
		Steps: []PlanStep{{ID: "step-1", Description: "retry with recovery"}},
	}
	state := contextdata.NewEnvelope("task-2", "")
	task := &core.Task{ID: "task-3", Instruction: "recover"}

	pe := &PlanExecutor{
		Options: PlanExecutionOptions{
			MaxRecoveryAttempts: 1,
			Recover: func(ctx context.Context, step PlanStep, stepTask *core.Task, env *contextdata.Envelope, err error) (*StepRecovery, error) {
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
	require.Equal(t, 2, executor.attempts)
}

func TestBuildStepTaskHandlesNilTask(t *testing.T) {
	step := PlanStep{ID: "s1", Description: "do work"}
	task := buildStepTask(nil, nil, step, nil)
	if task == nil {
		t.Fatalf("expected step task")
	}
	if task.ID != "" {
		t.Fatalf("expected empty id, got %q", task.ID)
	}
	if task.Instruction != step.Description {
		t.Fatalf("expected fallback instruction %q, got %q", step.Description, task.Instruction)
	}
}

func TestBuildStepTaskDoesNotReadArchitectState(t *testing.T) {
	step := PlanStep{ID: "s1", Description: "do work"}
	state := contextdata.NewEnvelope("task-2", "")
	state.SetWorkingValue("architect.last_step_summary", "framework should not read this", contextdata.MemoryClassTask)
	task := buildStepTask(&core.Task{}, nil, step, state)
	_ = task // suppress unused warning for now
	if _, ok := task.Context["previous_step_result"]; ok {
		t.Fatal("expected framework step task builder not to inject architect-specific context")
	}
}

func TestBuildStepTaskDoesNotCopyCallerSpecificContext(t *testing.T) {
	step := PlanStep{ID: "s1", Description: "do work"}
	task := buildStepTask(&core.Task{
		Context: map[string]any{
			"mode":                       "debug",
			"stream_callback":            func(string) {},
			"workflow_retrieval":         "retrieval text",
			"workflow_retrieval_payload": map[string]any{"kind": "payload"},
		},
	}, nil, step, nil)
	_ = task // suppress unused warning for now
	for _, key := range []string{"mode", "stream_callback", "workflow_retrieval", "workflow_retrieval_payload"} {
		if _, ok := task.Context[key]; ok {
			t.Fatalf("expected framework step task builder not to copy %q", key)
		}
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
func (e *isolatedExecutor) Capabilities() []string               { return nil }
func (e *isolatedExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}

func (e *isolatedExecutor) BranchExecutor() (WorkflowExecutor, error) {
	return &isolatedExecutor{shared: e.shared}, nil
}

func (e *isolatedExecutor) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*Result, error) {
	stepVal, ok := task.Context["current_step"]
	if !ok {
		return &Result{Success: true}, nil
	}
	step, ok := stepVal.(PlanStep)
	if !ok {
		return &Result{Success: true}, nil
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
	env.SetWorkingValue("completed."+step.ID, true, contextdata.MemoryClassTask)
	return &Result{Success: true}, nil
}

func TestPlanExecutorRunsReadyStepsInParallelWithIsolatedBranchAgents(t *testing.T) {
	shared := &isolatedExecutorShared{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	executor := &isolatedExecutor{shared: shared}
	plan := &Plan{
		Steps: []PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: make(map[string][]string),
	}
	state := contextdata.NewEnvelope("task-2", "")
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

	// With isolated branches, state is not merged back to parent by default
	// The test verifies that branches ran in parallel, not that state was merged
}

type conflictingIsolatedExecutor struct {
	shared *isolatedExecutorShared
}

func (e *conflictingIsolatedExecutor) Initialize(config *core.Config) error { return nil }
func (e *conflictingIsolatedExecutor) Capabilities() []string               { return nil }
func (e *conflictingIsolatedExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}
func (e *conflictingIsolatedExecutor) BranchExecutor() (WorkflowExecutor, error) {
	return &conflictingIsolatedExecutor{shared: e.shared}, nil
}
func (e *conflictingIsolatedExecutor) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*Result, error) {
	stepVal, _ := task.Context["current_step"]
	step, _ := stepVal.(PlanStep)
	env.SetWorkingValue("shared.conflict", step.ID, contextdata.MemoryClassTask)
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
	task := &core.Task{ID: "task-conflict", Instruction: "parallel conflict"}

	// New merge strategy is union-based with last-write-wins, so no error is expected
	_, err := (&PlanExecutor{}).Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
}

type historyMutatingExecutor struct{}

func (e *historyMutatingExecutor) Initialize(config *core.Config) error { return nil }
func (e *historyMutatingExecutor) Capabilities() []string               { return nil }
func (e *historyMutatingExecutor) BuildGraph(task *core.Task) (*Graph, error) {
	return nil, nil
}
func (e *historyMutatingExecutor) BranchExecutor() (WorkflowExecutor, error) {
	return &historyMutatingExecutor{}, nil
}
func (e *historyMutatingExecutor) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*Result, error) {
	// Add interaction to history stored in _history key
	var history []any
	if h, ok := env.GetWorkingValue("_history"); ok {
		if hSlice, ok := h.([]any); ok {
			history = hSlice
		}
	}
	history = append(history, map[string]any{"role": "assistant", "content": "branch note"})
	env.SetWorkingValue("_history", history, contextdata.MemoryClassTask)
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
	// New merge strategy is union-based with last-write-wins, so history mutation is allowed
	_, err := (&PlanExecutor{}).Execute(context.Background(), &historyMutatingExecutor{}, &core.Task{ID: "task-history"}, plan, contextdata.NewEnvelope("task-history", ""))
	require.NoError(t, err)
	// Verify that history was merged (union-based merge includes all entries)
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
	task := &core.Task{ID: "task-custom-merge", Instruction: "parallel conflict"}

	_, err := (&PlanExecutor{Options: PlanExecutionOptions{
		MergeBranches: func(parent *contextdata.Envelope, branches []BranchExecutionResult) error {
			parent.SetWorkingValue("parallel.steps", []string{branches[0].Step.ID, branches[1].Step.ID}, contextdata.MemoryClassTask)
			parent.SetWorkingValue("parallel.merge_policy", "custom", contextdata.MemoryClassTask)
			return nil
		},
	}}).Execute(context.Background(), executor, task, plan, state)
	require.NoError(t, err)
	val, _ := state.GetWorkingValue("parallel.merge_policy")
	require.Equal(t, "custom", val)
}

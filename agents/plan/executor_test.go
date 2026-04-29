package plan

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
	_ = task
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
	_ = task
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

	// With isolated branches, state is not merged back to parent by default.
	// The test verifies that branches ran in parallel, not that state was merged.
}

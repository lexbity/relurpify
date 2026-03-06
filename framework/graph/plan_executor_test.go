package graph

import (
	"context"
	"errors"
	"sync"
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

func TestPlanExecutorMergesParallelContextsDeterministically(t *testing.T) {
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

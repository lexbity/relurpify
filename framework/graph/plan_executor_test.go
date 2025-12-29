package graph

import (
	"context"
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

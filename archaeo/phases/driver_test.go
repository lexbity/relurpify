package phases_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	"codeburg.org/lexbit/relurpify/archaeo/phases"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestDriverHandlePreparationOutcomeAndCompletion(t *testing.T) {
	ctx := context.Background()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-driver",
		TaskID:      "task-driver",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "phase driver",
		Status:      memory.WorkflowRunStatusRunning,
	}))

	driver := phases.Driver{
		Service: phases.Service{Store: store},
		Broker:  guidance.NewGuidanceBroker(time.Minute),
		Handoff: func(_ context.Context, _ *core.Task, state *core.Context, _ *frameworkplan.PlanStep) error {
			state.Set("euclo.execution_handoff_ref", "plan-driver:v1")
			return nil
		},
	}
	state := core.NewContext()
	state.Set("euclo.workflow_id", "wf-driver")
	plan := &frameworkplan.LivingPlan{ID: "plan-driver", Version: 1}
	state.Set("euclo.living_plan", plan)

	result, prepErr, handled := driver.HandlePreparationOutcome(ctx, &core.Task{}, state, &core.Result{Success: false}, context.DeadlineExceeded, nil)
	require.True(t, handled)
	require.Error(t, prepErr)
	require.NotNil(t, result)

	phaseStateRaw, ok := state.Get("euclo.archaeo_phase_state")
	require.True(t, ok)
	phaseState, ok := phaseStateRaw.(*archaeodomain.WorkflowPhaseState)
	require.True(t, ok)
	require.Equal(t, archaeodomain.PhaseBlocked, phaseState.CurrentPhase)

	step := &frameworkplan.PlanStep{ID: "step-1"}
	driver.EnterExecution(ctx, &core.Task{}, state, step)
	phaseStateRaw, _ = state.Get("euclo.archaeo_phase_state")
	phaseState = phaseStateRaw.(*archaeodomain.WorkflowPhaseState)
	require.Equal(t, archaeodomain.PhaseExecution, phaseState.CurrentPhase)
	require.Equal(t, "plan-driver:v1", state.GetString("euclo.execution_handoff_ref"))

	driver.EnterVerification(ctx, &core.Task{}, state, step, nil)
	driver.EnterSurfacing(ctx, &core.Task{}, state, step, nil)
	driver.Complete(ctx, &core.Task{}, state, step, nil)
	phaseStateRaw, _ = state.Get("euclo.archaeo_phase_state")
	phaseState = phaseStateRaw.(*archaeodomain.WorkflowPhaseState)
	require.Equal(t, archaeodomain.PhaseCompleted, phaseState.CurrentPhase)
}

package execution

import (
	"context"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpic"
	euclobehavior "github.com/lexcodex/relurpify/named/euclo/relurpic/behavior"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/require"
)

type stubWorkflowExecutor struct{}

func (stubWorkflowExecutor) Initialize(_ *core.Config) error { return nil }
func (stubWorkflowExecutor) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	return &core.Result{Success: true, Data: map[string]any{"summary": "ok"}}, nil
}
func (stubWorkflowExecutor) Capabilities() []core.Capability { return nil }
func (stubWorkflowExecutor) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	return nil, nil
}

func sessionBehaviorInput() (euclobehavior.Service, eucloruntime.UnitOfWork, graph.WorkflowExecutor) {
	return *euclobehavior.NewService(), eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk,
	}, stubWorkflowExecutor{}
}

func TestSessionExecuteFailsWithoutBehaviorService(t *testing.T) {
	svc := SessionService{}
	out := svc.Execute(context.Background(), SessionInput{
		State: core.NewContext(),
	})
	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
}

func TestSessionResolveEmitterDefaults(t *testing.T) {
	svc := SessionService{}
	emitter, withTransitions, maxTransitions := svc.resolveEmitter(nil)
	require.NotNil(t, emitter)
	require.IsType(t, &interaction.NoopEmitter{}, emitter)
	require.False(t, withTransitions)
	require.Zero(t, maxTransitions)
}

func TestSessionExecuteStopsBeforeVerificationWhenCheckpointFails(t *testing.T) {
	behaviorService, work, executor := sessionBehaviorInput()
	svc := SessionService{
		BehaviorService: &behaviorService,
		BeforeVerification: func(context.Context, *core.Task, *core.Context) error {
			return context.Canceled
		},
	}
	out := svc.Execute(context.Background(), SessionInput{
		Task:             &core.Task{},
		ExecutionTask:    &core.Task{},
		WorkflowExecutor: executor,
		State:            core.NewContext(),
		Work:             work,
	})
	require.Error(t, out.Err)
}

func TestSessionExecuteRunsMutationCheckpointsInOrder(t *testing.T) {
	var seen []archaeodomain.MutationCheckpoint
	behaviorService, work, executor := sessionBehaviorInput()
	svc := SessionService{
		BehaviorService: &behaviorService,
		Checkpoint: func(_ context.Context, checkpoint archaeodomain.MutationCheckpoint, _ *core.Task, state *core.Context) error {
			seen = append(seen, checkpoint)
			return nil
		},
	}
	out := svc.Execute(context.Background(), SessionInput{
		Task:             &core.Task{},
		ExecutionTask:    &core.Task{},
		WorkflowExecutor: executor,
		State:            core.NewContext(),
		Work:             work,
	})
	require.NoError(t, out.Err)
	require.Equal(t, []archaeodomain.MutationCheckpoint{
		archaeodomain.MutationCheckpointPreDispatch,
		archaeodomain.MutationCheckpointPostExecution,
		archaeodomain.MutationCheckpointPreVerification,
		archaeodomain.MutationCheckpointPreFinalization,
	}, seen)
}

func TestSessionExecuteStopsAtCheckpointFailure(t *testing.T) {
	var seen []archaeodomain.MutationCheckpoint
	behaviorService, work, executor := sessionBehaviorInput()
	svc := SessionService{
		BehaviorService: &behaviorService,
		Checkpoint: func(_ context.Context, checkpoint archaeodomain.MutationCheckpoint, _ *core.Task, _ *core.Context) error {
			seen = append(seen, checkpoint)
			if checkpoint == archaeodomain.MutationCheckpointPostExecution {
				return context.Canceled
			}
			return nil
		},
	}
	out := svc.Execute(context.Background(), SessionInput{
		Task:             &core.Task{},
		ExecutionTask:    &core.Task{},
		WorkflowExecutor: executor,
		State:            core.NewContext(),
		Work:             work,
	})
	require.Error(t, out.Err)
	require.Equal(t, []archaeodomain.MutationCheckpoint{
		archaeodomain.MutationCheckpointPreDispatch,
		archaeodomain.MutationCheckpointPostExecution,
	}, seen)
}

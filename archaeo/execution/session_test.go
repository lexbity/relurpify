package execution

import (
	"context"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	"github.com/stretchr/testify/require"
)

func TestSessionExecuteFailsWithoutProfileController(t *testing.T) {
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
	svc := SessionService{
		ProfileCtrl: &orchestrate.ProfileController{},
		BeforeVerification: func(context.Context, *core.Task, *core.Context) error {
			return context.Canceled
		},
	}
	out := svc.Execute(context.Background(), SessionInput{
		Task:          &core.Task{},
		ExecutionTask: &core.Task{},
		State:         core.NewContext(),
	})
	require.Error(t, out.Err)
}

func TestSessionExecuteRunsMutationCheckpointsInOrder(t *testing.T) {
	var seen []archaeodomain.MutationCheckpoint
	svc := SessionService{
		ProfileCtrl: &orchestrate.ProfileController{},
		Checkpoint: func(_ context.Context, checkpoint archaeodomain.MutationCheckpoint, _ *core.Task, state *core.Context) error {
			seen = append(seen, checkpoint)
			return nil
		},
	}
	out := svc.Execute(context.Background(), SessionInput{
		Task:          &core.Task{},
		ExecutionTask: &core.Task{},
		State:         core.NewContext(),
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
	svc := SessionService{
		ProfileCtrl: &orchestrate.ProfileController{},
		Checkpoint: func(_ context.Context, checkpoint archaeodomain.MutationCheckpoint, _ *core.Task, _ *core.Context) error {
			seen = append(seen, checkpoint)
			if checkpoint == archaeodomain.MutationCheckpointPostExecution {
				return context.Canceled
			}
			return nil
		},
	}
	out := svc.Execute(context.Background(), SessionInput{
		Task:          &core.Task{},
		ExecutionTask: &core.Task{},
		State:         core.NewContext(),
	})
	require.Error(t, out.Err)
	require.Equal(t, []archaeodomain.MutationCheckpoint{
		archaeodomain.MutationCheckpointPreDispatch,
		archaeodomain.MutationCheckpointPostExecution,
	}, seen)
}

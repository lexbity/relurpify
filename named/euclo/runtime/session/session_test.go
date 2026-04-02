package session

import (
	"context"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclobehavior "github.com/lexcodex/relurpify/named/euclo/runtime/orchestrate"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
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
		Environment:     testutil.Env(t),
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
		Environment:     testutil.Env(t),
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
		Environment:     testutil.Env(t),
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

func TestSessionExecuteReportsAssuranceForMissingVerificationOnMutation(t *testing.T) {
	behaviorService, work, executor := sessionBehaviorInput()
	work.PrimaryRelurpicCapabilityID = euclorelurpic.CapabilityChatImplement
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	svc := SessionService{
		Environment:     testutil.Env(t),
		BehaviorService: &behaviorService,
	}

	out := svc.Execute(context.Background(), SessionInput{
		Task:             &core.Task{},
		ExecutionTask:    &core.Task{},
		WorkflowExecutor: executor,
		State:            state,
		Mode:             euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "edit_verify_repair",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
		Work: work,
	})

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	require.Equal(t, eucloruntime.AssuranceClassUnverifiedSuccess, out.Result.Data["assurance_class"])
	require.Equal(t, eucloruntime.AssuranceClassUnverifiedSuccess, out.FinalReport["assurance_class"])
}

func TestSessionExecuteReportsRepairExhaustedAssurance(t *testing.T) {
	behaviorService, work, executor := sessionBehaviorInput()
	work.PrimaryRelurpicCapabilityID = euclorelurpic.CapabilityChatImplement
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	state.Set("pipeline.verify", map[string]any{
		"status":     "fail",
		"provenance": "executed",
		"checks": []map[string]any{{
			"name":       "go_test",
			"status":     "fail",
			"provenance": "executed",
		}},
	})
	state.Set("euclo.recovery_trace", map[string]any{
		"status":        "repair_exhausted",
		"attempt_count": 2,
	})
	svc := SessionService{
		Environment:     testutil.Env(t),
		BehaviorService: &behaviorService,
	}

	out := svc.Execute(context.Background(), SessionInput{
		Task:             &core.Task{},
		ExecutionTask:    &core.Task{},
		WorkflowExecutor: executor,
		State:            state,
		Mode:             euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "edit_verify_repair",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
		Work: work,
	})

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	require.Equal(t, eucloruntime.AssuranceClassRepairExhausted, out.Result.Data["assurance_class"])
	require.Equal(t, eucloruntime.AssuranceClassRepairExhausted, out.FinalReport["assurance_class"])
	require.Equal(t, "repair_exhausted", out.ProofSurface.RecoveryStatus)
	require.Equal(t, 2, out.ProofSurface.RecoveryAttempts)
}

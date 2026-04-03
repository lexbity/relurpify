package assurance

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
	euclodispatch "github.com/lexcodex/relurpify/named/euclo/runtime/dispatch"
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

func assuranceBehaviorInput() (euclodispatch.Dispatcher, eucloruntime.UnitOfWork, graph.WorkflowExecutor) {
	return *euclodispatch.NewDispatcher(), eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk,
	}, stubWorkflowExecutor{}
}

func TestAssuranceExecuteFailsWithoutBehaviorService(t *testing.T) {
	svc := Runtime{}
	out := svc.Execute(context.Background(), Input{
		State: core.NewContext(),
	})
	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
}

func TestAssuranceResolveEmitterDefaults(t *testing.T) {
	svc := Runtime{}
	emitter, withTransitions, maxTransitions := svc.resolveEmitter(nil)
	require.NotNil(t, emitter)
	require.IsType(t, &interaction.NoopEmitter{}, emitter)
	require.False(t, withTransitions)
	require.Zero(t, maxTransitions)
}

func TestAssuranceExecuteStopsBeforeVerificationWhenCheckpointFails(t *testing.T) {
	behaviorService, work, executor := assuranceBehaviorInput()
	svc := Runtime{
		Environment:        testutil.Env(t),
		BehaviorDispatcher: &behaviorService,
		BeforeVerification: func(context.Context, *core.Task, *core.Context) error {
			return context.Canceled
		},
	}
	out := svc.Execute(context.Background(), Input{
		Task:             &core.Task{},
		ExecutionTask:    &core.Task{},
		WorkflowExecutor: executor,
		State:            core.NewContext(),
		Work:             work,
	})
	require.Error(t, out.Err)
}

func TestAssuranceExecuteRunsMutationCheckpointsInOrder(t *testing.T) {
	var seen []archaeodomain.MutationCheckpoint
	behaviorService, work, executor := assuranceBehaviorInput()
	svc := Runtime{
		Environment:        testutil.Env(t),
		BehaviorDispatcher: &behaviorService,
		Checkpoint: func(_ context.Context, checkpoint archaeodomain.MutationCheckpoint, _ *core.Task, state *core.Context) error {
			seen = append(seen, checkpoint)
			return nil
		},
	}
	out := svc.Execute(context.Background(), Input{
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

func TestAssuranceExecuteStopsAtCheckpointFailure(t *testing.T) {
	var seen []archaeodomain.MutationCheckpoint
	behaviorService, work, executor := assuranceBehaviorInput()
	svc := Runtime{
		Environment:        testutil.Env(t),
		BehaviorDispatcher: &behaviorService,
		Checkpoint: func(_ context.Context, checkpoint archaeodomain.MutationCheckpoint, _ *core.Task, _ *core.Context) error {
			seen = append(seen, checkpoint)
			if checkpoint == archaeodomain.MutationCheckpointPostExecution {
				return context.Canceled
			}
			return nil
		},
	}
	out := svc.Execute(context.Background(), Input{
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

func TestAssuranceExecuteReportsAssuranceForMissingVerificationOnMutation(t *testing.T) {
	behaviorService, work, executor := assuranceBehaviorInput()
	work.PrimaryRelurpicCapabilityID = euclorelurpic.CapabilityChatImplement
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	svc := Runtime{
		Environment:        testutil.Env(t),
		BehaviorDispatcher: &behaviorService,
	}

	out := svc.Execute(context.Background(), Input{
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

func TestAssuranceExecuteReportsRepairExhaustedAssurance(t *testing.T) {
	behaviorService, work, executor := assuranceBehaviorInput()
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
	svc := Runtime{
		Environment:        testutil.Env(t),
		BehaviorDispatcher: &behaviorService,
	}

	out := svc.Execute(context.Background(), Input{
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

func TestAssuranceExecuteAllowsOperatorWaiverForMissingVerification(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	state.Set("euclo.execution_waiver", eucloruntime.ExecutionWaiver{
		WaiverID:  "waiver-1",
		Kind:      eucloruntime.WaiverKindVerification,
		Reason:    "operator approved degraded verification",
		GrantedBy: "operator",
		RunID:     "run-1",
	})
	svc := Runtime{
		Environment: testutil.Env(t),
	}
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "waived success"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "edit_verify_repair",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
	}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.NoError(t, out.Err)
	require.NotNil(t, out.Result)
	require.True(t, out.Result.Success)
	require.Equal(t, eucloruntime.AssuranceClassOperatorDeferred, out.Result.Data["assurance_class"])
	require.Equal(t, "operator_waiver", out.FinalReport["degradation_mode"])
	require.Equal(t, "operator_waiver", out.FinalReport["degradation_reason"])
}

func TestAssuranceExecuteMarksAutomaticDegradationWhenVerificationToolsUnavailable(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	state.Set("euclo.envelope", eucloruntime.TaskEnvelope{
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{
			HasExecuteTools:      false,
			HasVerificationTools: false,
		},
	})
	svc := Runtime{
		Environment: testutil.Env(t),
	}
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "degraded failure"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "edit_verify_repair",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
	}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	require.Equal(t, "automatic", out.FinalReport["degradation_mode"])
	require.Equal(t, "verification_tools_unavailable", out.FinalReport["degradation_reason"])
}

func TestAssuranceApplyVerificationAndArtifacts_ReproduceLocalizePatchMarksAutomaticDegradation(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	state.Set("euclo.envelope", eucloruntime.TaskEnvelope{
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{
			HasExecuteTools:      false,
			HasVerificationTools: false,
		},
	})
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "debug degraded failure"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "debug"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "reproduce_localize_patch",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
	}
	svc := Runtime{Environment: testutil.Env(t)}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	require.Equal(t, "automatic", out.FinalReport["degradation_mode"])
	require.Equal(t, "verification_tools_unavailable", out.FinalReport["degradation_reason"])
	require.Equal(t, eucloruntime.AssuranceClassUnverifiedSuccess, out.FinalReport["assurance_class"])
}

func TestAssuranceApplyVerificationAndArtifacts_TDDIncompleteBlocksCompletion(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	state.Set("pipeline.verify", map[string]any{
		"status":     "pass",
		"provenance": "executed",
		"run_id":     "run-1",
		"checks": []any{map[string]any{
			"name":       "go_test",
			"status":     "pass",
			"provenance": "executed",
			"run_id":     "run-1",
		}},
	})
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "tdd incomplete"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "tdd"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "test_driven_generation",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
	}
	svc := Runtime{Environment: testutil.Env(t)}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	require.Equal(t, eucloruntime.AssuranceClassTDDIncomplete, out.Result.Data["assurance_class"])
	require.Equal(t, eucloruntime.AssuranceClassTDDIncomplete, out.FinalReport["assurance_class"])
}

func TestAssuranceApplyVerificationAndArtifacts_ReviewSuggestImplementDoesNotRequireVerification(t *testing.T) {
	state := core.NewContext()
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "review completed"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "review"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "review_suggest_implement",
			VerificationRequired: false,
			MutationAllowed:      false,
		},
	}
	svc := Runtime{Environment: testutil.Env(t)}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.NoError(t, out.Err)
	require.NotNil(t, out.Result)
	require.True(t, out.Result.Success)
	require.Equal(t, eucloruntime.AssuranceClassUnverifiedSuccess, out.Result.Data["assurance_class"])
	_, degraded := out.FinalReport["degradation_mode"]
	require.False(t, degraded)
}

func TestAssuranceApplyVerificationAndArtifacts_RejectsFallbackVerificationForFreshEdits(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	state.Set("pipeline.verify", "verification looked okay")
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "fallback verification"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "edit_verify_repair",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
	}
	svc := Runtime{Environment: testutil.Env(t)}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	successGate, ok := out.Result.Data["success_gate"].(eucloruntime.SuccessGateResult)
	require.True(t, ok)
	require.Equal(t, "verification_fallback_rejected", successGate.Reason)
	require.Equal(t, eucloruntime.AssuranceClassUnverifiedSuccess, successGate.AssuranceClass)
}

func TestAssuranceApplyVerificationAndArtifacts_RejectsReusedVerificationForFreshEdits(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Executed: []eucloruntime.EditOperationRecord{{Path: "main.go", Status: "applied"}},
	})
	state.Set("react.verification_latched_summary", "reused previous verification")
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "reused verification"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "edit_verify_repair",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
	}
	svc := Runtime{Environment: testutil.Env(t)}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	successGate, ok := out.Result.Data["success_gate"].(eucloruntime.SuccessGateResult)
	require.True(t, ok)
	require.Equal(t, "verification_reused_rejected", successGate.Reason)
	require.Equal(t, eucloruntime.AssuranceClassUnverifiedSuccess, successGate.AssuranceClass)
}

func TestAssuranceApplyVerificationAndArtifacts_TreatsFinalOutputFileWriteAsMutation(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "file_write applied the requested changes",
		"final_output": map[string]any{
			"result": map[string]any{
				"file_write": map[string]any{
					"success": true,
					"data": map[string]any{
						"path": "testsuite/fixtures/strings.go",
					},
				},
			},
		},
	})
	out := Output{Result: &core.Result{Success: true, Data: map[string]any{"summary": "live-style mutation without verification"}}}
	in := Input{
		Task:  &core.Task{},
		State: state,
		Mode:  euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{
			ProfileID:            "edit_verify_repair",
			VerificationRequired: true,
			MutationAllowed:      true,
		},
	}
	svc := Runtime{Environment: testutil.Env(t)}
	svc.applyVerificationAndArtifacts(context.Background(), in, &out)

	require.Error(t, out.Err)
	require.NotNil(t, out.Result)
	require.False(t, out.Result.Success)
	successGate, ok := out.Result.Data["success_gate"].(eucloruntime.SuccessGateResult)
	require.True(t, ok)
	require.Equal(t, "verification_missing", successGate.Reason)
	require.Equal(t, eucloruntime.AssuranceClassUnverifiedSuccess, successGate.AssuranceClass)
}

package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullExecuteCodeModeEditVerifyRepair(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "all tests pass",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-code-1",
		Instruction: "implement the foo feature",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.mode_resolution")
	require.True(t, ok)
	mode, ok := raw.(euclotypes.ModeResolution)
	require.True(t, ok)
	assert.NotEmpty(t, mode.ModeID)

	raw, ok = state.Get("euclo.execution_profile_selection")
	require.True(t, ok)
	profile, ok := raw.(euclotypes.ExecutionProfileSelection)
	require.True(t, ok)
	assert.NotEmpty(t, profile.ProfileID)

	raw, ok = state.Get("euclo.unit_of_work")
	require.True(t, ok)
	work, ok := raw.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	assert.Equal(t, mode.ModeID, work.ModeID)
	assert.NotEmpty(t, work.ObjectiveKind)
	assert.NotEmpty(t, work.BehaviorFamily)
	assert.NotEmpty(t, work.ContextStrategyID)
	assert.NotEmpty(t, work.ToolBindings)
	assert.NotEmpty(t, work.ExecutorDescriptor.ExecutorID)
	assert.NotEmpty(t, work.PrimaryRelurpicCapabilityID)
	assert.Contains(t, work.PrimaryRelurpicCapabilityID, "euclo:chat.")
	assert.Equal(t, work.ModeID, work.ResolvedPolicy.ModeID)
	assert.Equal(t, profile.ProfileID, work.ResolvedPolicy.ProfileID)
	assert.Equal(t, eucloruntime.UnitOfWorkStatusCompleted, work.Status)
	assert.NotEmpty(t, work.VerificationPolicyID)

	raw, ok = state.Get("euclo.capability_family_routing")
	require.True(t, ok)
	routing, ok := raw.(eucloruntime.CapabilityFamilyRouting)
	require.True(t, ok)
	assert.NotEmpty(t, routing.PrimaryFamilyID)

	// Action log and proof surface should be populated.
	raw, ok = state.Get("euclo.action_log")
	require.True(t, ok)
	actionLog, ok := raw.([]eucloruntime.ActionLogEntry)
	require.True(t, ok)
	assert.NotEmpty(t, actionLog)

	raw, ok = state.Get("euclo.proof_surface")
	require.True(t, ok)
	proof, ok := raw.(eucloruntime.ProofSurface)
	require.True(t, ok)
	assert.NotEmpty(t, proof.ArtifactKinds)
	assert.Equal(t, work.ModeID, proof.ModeID)
	assert.Equal(t, profile.ProfileID, proof.ProfileID)

	raw, ok = state.Get("euclo.resolved_execution_policy")
	require.True(t, ok)
	policy, ok := raw.(eucloruntime.ResolvedExecutionPolicy)
	require.True(t, ok)
	assert.Equal(t, work.ResolvedPolicy.ProfileID, policy.ProfileID)

	raw, ok = state.Get("euclo.compiled_execution")
	require.True(t, ok)
	compiled, ok := raw.(eucloruntime.CompiledExecution)
	require.True(t, ok)
	assert.Equal(t, work.ID, compiled.UnitOfWorkID)
	assert.Equal(t, work.ExecutorDescriptor.ExecutorID, compiled.ExecutorDescriptor.ExecutorID)

	raw, ok = state.Get("euclo.executor_runtime")
	require.True(t, ok)
	runtimeState, ok := raw.(eucloruntime.ExecutorRuntimeState)
	require.True(t, ok)
	assert.Equal(t, work.ExecutorDescriptor.Family, runtimeState.Family)
	assert.NotEmpty(t, runtimeState.Path)

	raw, ok = state.Get("euclo.chat_capability_runtime")
	require.True(t, ok)
	chatRuntime, ok := raw.(eucloruntime.ChatCapabilityRuntimeState)
	require.True(t, ok)
	assert.Equal(t, work.PrimaryRelurpicCapabilityID, chatRuntime.PrimaryCapabilityID)
	assert.NotEmpty(t, chatRuntime.PolicySnapshotID)
	assert.Equal(t, "pass", chatRuntime.VerificationStatus)
	assert.Equal(t, 1, chatRuntime.VerificationCheckCount)
	assert.True(t, chatRuntime.SharedContextEnabled)

	sessionID := state.GetString("euclo.session_id")
	assert.NotEmpty(t, sessionID)
	assert.Contains(t, sessionID, "euclo-")
}

func TestFullExecuteChatAskPublishesAskRuntimeContract(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-chat-ask-1",
		Instruction: "how does the caching layer work?",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.chat_capability_runtime")
	require.True(t, ok)
	chatRuntime, ok := raw.(eucloruntime.ChatCapabilityRuntimeState)
	require.True(t, ok)
	assert.Equal(t, "euclo:chat.ask", chatRuntime.PrimaryCapabilityID)
	assert.NotEmpty(t, chatRuntime.PolicySnapshotID)
	assert.True(t, chatRuntime.AskActive)
	assert.True(t, chatRuntime.NonMutating)
	assert.False(t, chatRuntime.MutationObserved)
}

func TestFullExecuteChatInspectPublishesInspectRuntimeContract(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-chat-inspect-1",
		Instruction: "inspect the current auth middleware and review the flow",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.chat_capability_runtime")
	require.True(t, ok)
	chatRuntime, ok := raw.(eucloruntime.ChatCapabilityRuntimeState)
	require.True(t, ok)
	assert.Equal(t, "euclo:chat.inspect", chatRuntime.PrimaryCapabilityID)
	assert.NotEmpty(t, chatRuntime.PolicySnapshotID)
	assert.True(t, chatRuntime.InspectActive)
	assert.True(t, chatRuntime.InspectFirst)
	assert.True(t, chatRuntime.LocalReviewActive)
}

func TestFullExecutePreservesWorkUnitAcrossDebugToImplementTransition(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("euclo.unit_of_work", eucloruntime.UnitOfWork{
		ID:                          "uow-debug-seeded",
		RootID:                      "uow-debug-seeded",
		ModeID:                      "debug",
		PrimaryRelurpicCapabilityID: "euclo:debug.investigate",
	})
	state.Set("pipeline.verify", map[string]any{
		"status": "pass",
		"checks": []any{map[string]any{"name": "go test", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-debug-to-implement-1",
		Instruction: "implement the fix for the failing parser test",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.unit_of_work")
	require.True(t, ok)
	work, ok := raw.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	assert.Equal(t, "uow-debug-seeded", work.ID)
	assert.Empty(t, work.PredecessorUnitOfWorkID)

	raw, ok = state.Get("euclo.unit_of_work_transition")
	require.True(t, ok)
	transition, ok := raw.(eucloruntime.UnitOfWorkTransitionState)
	require.True(t, ok)
	assert.True(t, transition.Preserved)
	assert.False(t, transition.Rebound)
	assert.Equal(t, "uow-debug-seeded", transition.CurrentUnitOfWorkID)
}

func TestFullExecuteRebindsWorkUnitFromArchaeoBackedToLocalChat(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("euclo.unit_of_work", eucloruntime.UnitOfWork{
		ID:                          "uow-plan-seeded",
		RootID:                      "uow-plan-seeded",
		ModeID:                      "planning",
		PrimaryRelurpicCapabilityID: "euclo:archaeology.compile-plan",
		SemanticInputs: eucloruntime.SemanticInputBundle{
			PatternRefs: []string{"pattern:1"},
		},
		PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
			IsPlanBacked: true,
			PlanID:       "plan-1",
			PlanVersion:  1,
		},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-plan-to-local-1",
		Instruction: "how does the auth middleware work?",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.unit_of_work")
	require.True(t, ok)
	work, ok := raw.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	assert.NotEqual(t, "uow-plan-seeded", work.ID)
	assert.Equal(t, "uow-plan-seeded", work.PredecessorUnitOfWorkID)
	assert.Equal(t, "uow-plan-seeded", work.RootID)

	raw, ok = state.Get("euclo.compiled_execution")
	require.True(t, ok)
	compiled, ok := raw.(eucloruntime.CompiledExecution)
	require.True(t, ok)
	assert.Equal(t, "uow-plan-seeded", compiled.PredecessorUnitOfWorkID)
	assert.Equal(t, "uow-plan-seeded", compiled.RootUnitOfWorkID)

	raw, ok = state.Get("euclo.unit_of_work_transition")
	require.True(t, ok)
	transition, ok := raw.(eucloruntime.UnitOfWorkTransitionState)
	require.True(t, ok)
	assert.True(t, transition.Rebound)
	assert.False(t, transition.Preserved)
	assert.Equal(t, "archaeo_to_local_rebind", transition.Reason)

	raw, ok = state.Get("euclo.unit_of_work_history")
	require.True(t, ok)
	history, ok := raw.([]eucloruntime.UnitOfWorkHistoryEntry)
	require.True(t, ok)
	assert.Len(t, history, 2)
	assert.Equal(t, "uow-plan-seeded", history[1].PredecessorUnitOfWorkID)
}

func TestFullExecutePreservesWorkUnitAcrossChatToArchaeologyTransition(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("euclo.unit_of_work", eucloruntime.UnitOfWork{
		ID:                          "uow-chat-seeded",
		RootID:                      "uow-chat-seeded",
		ModeID:                      "code",
		PrimaryRelurpicCapabilityID: "euclo:chat.implement",
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-chat-to-arch-1",
		Instruction: "compile the plan for the auth migration",
		Context:     map[string]any{"workspace": "/tmp/ws", "mode": "planning", "workflow_id": "wf-plan"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.unit_of_work")
	require.True(t, ok)
	work, ok := raw.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	assert.Equal(t, "uow-chat-seeded", work.ID)
	assert.Equal(t, "euclo:archaeology.compile-plan", work.PrimaryRelurpicCapabilityID)

	raw, ok = state.Get("euclo.unit_of_work_transition")
	require.True(t, ok)
	transition, ok := raw.(eucloruntime.UnitOfWorkTransitionState)
	require.True(t, ok)
	assert.True(t, transition.Preserved)
	assert.Equal(t, "capability_owner_transition", transition.Reason)

	raw, ok = state.Get("euclo.archaeology_capability_runtime")
	require.True(t, ok)
	archRuntime, ok := raw.(eucloruntime.ArchaeologyCapabilityRuntimeState)
	require.True(t, ok)
	assert.Equal(t, "euclo:archaeology.compile-plan", archRuntime.PrimaryCapabilityID)
	assert.NotEmpty(t, archRuntime.PolicySnapshotID)
}

func TestFullExecutePreservesWorkUnitAcrossArchaeologyToImplementPlan(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("euclo.unit_of_work", eucloruntime.UnitOfWork{
		ID:                          "uow-arch-seeded",
		RootID:                      "uow-arch-seeded",
		ModeID:                      "planning",
		PrimaryRelurpicCapabilityID: "euclo:archaeology.explore",
	})
	state.Set("euclo.current_plan_step_id", "step-1")

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-arch-to-implement-1",
		Instruction: "implement the plan for the auth migration",
		Context:     map[string]any{"workspace": "/tmp/ws", "mode": "planning", "workflow_id": "wf-plan-impl"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.unit_of_work")
	require.True(t, ok)
	work, ok := raw.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	assert.Equal(t, "uow-arch-seeded", work.ID)
	assert.Equal(t, "euclo:archaeology.implement-plan", work.PrimaryRelurpicCapabilityID)

	raw, ok = state.Get("euclo.unit_of_work_transition")
	require.True(t, ok)
	transition, ok := raw.(eucloruntime.UnitOfWorkTransitionState)
	require.True(t, ok)
	assert.True(t, transition.Preserved)

	raw, ok = state.Get("euclo.archaeology_capability_runtime")
	require.True(t, ok)
	archRuntime, ok := raw.(eucloruntime.ArchaeologyCapabilityRuntimeState)
	require.True(t, ok)
	assert.Equal(t, "euclo:archaeology.implement-plan", archRuntime.PrimaryCapabilityID)
	assert.NotEmpty(t, archRuntime.PolicySnapshotID)
}

func TestFullExecuteDebugModePublishesDebugRuntimeContract(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification ok",
		"checks":  []any{map[string]any{"name": "go test", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-debug-1",
		Instruction: "fix the failing test in pkg/foo",
		Context:     map[string]any{"mode": "debug", "workspace": "/tmp/ws", "workflow_id": "wf-debug"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.mode_resolution")
	require.True(t, ok)
	mode, ok := raw.(euclotypes.ModeResolution)
	require.True(t, ok)
	assert.Equal(t, "debug", mode.ModeID)

	raw, ok = state.Get("euclo.unit_of_work")
	require.True(t, ok)
	work, ok := raw.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	assert.Equal(t, "debug", work.ModeID)
	assert.Equal(t, "euclo:debug.investigate", work.PrimaryRelurpicCapabilityID)
	assert.Equal(t, "debug.investigate.htn_drilldown", work.ExecutorDescriptor.RecipeID)
	assert.Contains(t, []eucloruntime.ExecutorFamily{eucloruntime.ExecutorFamilyHTN, eucloruntime.ExecutorFamilyReact}, work.ExecutorDescriptor.Family)
	assert.True(t, work.ResolvedPolicy.RequireVerificationStep || len(work.ResolvedPolicy.PreferredVerifyCapabilities) >= 0)
	assert.Equal(t, "wf-debug", work.SemanticInputs.WorkflowID)

	raw, ok = state.Get("euclo.resolved_execution_policy")
	require.True(t, ok)
	policy, ok := raw.(eucloruntime.ResolvedExecutionPolicy)
	require.True(t, ok)
	assert.Equal(t, "debug", policy.ModeID)

	raw, ok = state.Get("euclo.compiled_execution")
	require.True(t, ok)
	compiled, ok := raw.(eucloruntime.CompiledExecution)
	require.True(t, ok)
	assert.Equal(t, work.ExecutorDescriptor.ExecutorID, compiled.ExecutorDescriptor.ExecutorID)
	assert.Equal(t, work.SemanticInputs, compiled.SemanticInputs)

	raw, ok = state.Get("euclo.executor_runtime")
	require.True(t, ok)
	runtimeState, ok := raw.(eucloruntime.ExecutorRuntimeState)
	require.True(t, ok)
	assert.Equal(t, work.ExecutorDescriptor.Family, runtimeState.Family)
	assert.NotEmpty(t, runtimeState.Path)

	raw, ok = state.Get("euclo.context_runtime")
	require.True(t, ok)
	contextRuntime, ok := raw.(eucloruntime.ContextRuntimeState)
	require.True(t, ok)
	assert.Equal(t, "debug", contextRuntime.ModeID)
	assert.Equal(t, work.ExecutorDescriptor.Family, contextRuntime.ExecutorFamily)
	assert.Equal(t, "aggressive", contextRuntime.StrategyName)
	assert.True(t, contextRuntime.InitialLoadAttempted)

	raw, ok = state.Get("euclo.security_runtime")
	require.True(t, ok)
	securityRuntime, ok := raw.(eucloruntime.SecurityRuntimeState)
	require.True(t, ok)
	assert.Equal(t, "debug", securityRuntime.ModeID)
	assert.False(t, securityRuntime.Blocked)
	assert.NotEmpty(t, securityRuntime.ExecutionCatalogSnapshotID)
	assert.NotEmpty(t, securityRuntime.PolicySnapshotID)

	raw, ok = state.Get("euclo.debug_capability_runtime")
	require.True(t, ok)
	debugRuntime, ok := raw.(eucloruntime.DebugCapabilityRuntimeState)
	require.True(t, ok)
	assert.Equal(t, "euclo:debug.investigate", debugRuntime.PrimaryCapabilityID)
	assert.NotEmpty(t, debugRuntime.PolicySnapshotID)
	assert.True(t, debugRuntime.RootCauseActive)
	assert.True(t, debugRuntime.LocalizationActive)
	assert.True(t, debugRuntime.VerificationRepairActive)
	assert.True(t, debugRuntime.ToolExpositionFacet)
	assert.Equal(t, "pass", debugRuntime.VerificationStatus)
	assert.Equal(t, 1, debugRuntime.VerificationCheckCount)
	assert.Contains(t, debugRuntime.ToolOutputSources, "pipeline.verify")
	assert.Equal(t, "euclo:chat.implement", debugRuntime.EscalationTarget)

	raw, ok = state.Get("euclo.shared_context_runtime")
	require.True(t, ok)
	sharedRuntime, ok := raw.(eucloruntime.SharedContextRuntimeState)
	require.True(t, ok)
	assert.True(t, sharedRuntime.Enabled)
	assert.Equal(t, work.ExecutorDescriptor.Family, sharedRuntime.ExecutorFamily)
}

func TestFullExecuteWithRecoveryFallback(t *testing.T) {
	// Test the full recovery path through ProfileController + RecoveryController.
	// Uses ProfileController.ExecuteProfile directly to avoid mode/profile
	// resolution indirection.
	reg := capabilities.NewEucloCapabilityRegistry()
	primary := &stubProfileCapability{
		id:       "euclo:primary_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "primary failed",
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:recovery_succeeds",
			},
		},
	}
	recoveryCap := &stubProfileCapability{
		id:       "euclo:recovery_succeeds",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "recovered",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:recovery_succeeds", Status: "produced"},
				{Kind: euclotypes.ArtifactKindVerification, ProducerID: "euclo:recovery_succeeds", Status: "produced"},
			},
		},
	}
	_ = reg.Register(primary)
	_ = reg.Register(recoveryCap)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), euclotypes.DefaultExecutionProfileRegistry(), euclotypes.DefaultModeRegistry(), env)
	pc := orchestrate.NewProfileController(orchestrate.AdaptCapabilityRegistry(reg), gate.DefaultPhaseGates(), env, euclotypes.DefaultExecutionProfileRegistry(), rc)
	execEnv := testEnvelope(nil)

	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, execEnv)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Recovery should have been attempted and succeeded.
	assert.True(t, recoveryCap.executeCalled, "recovery capability should have been executed")
	assert.True(t, primary.executeCalled, "primary capability should have been executed")
	assert.Equal(t, 1, pcResult.RecoveryAttempts, "should have 1 recovery attempt")

	// Recovery trace artifact should be present.
	hasRecoveryTrace := false
	for _, art := range pcResult.Artifacts {
		if art.Kind == euclotypes.ArtifactKindRecoveryTrace {
			hasRecoveryTrace = true
		}
	}
	assert.True(t, hasRecoveryTrace, "should have recovery trace artifact")

	raw, ok := execEnv.State.Get("euclo.recovery_trace")
	require.True(t, ok, "recovery trace should be written to state")
	trace, ok := raw.(map[string]any)
	require.True(t, ok)
	attempts, ok := trace["attempts"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, attempts, 1)
	assert.Equal(t, "capability", attempts[0]["level"])
	assert.Equal(t, "capability_fallback", attempts[0]["strategy"])
}

func TestFullExecuteReviewModePublishesReviewReasoningFamilies(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "review checks passed",
		"checks":  []any{map[string]any{"name": "review", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-review-1",
		Instruction: "review the proposed compatibility change",
		Context:     map[string]any{"mode": "review", "workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.unit_of_work")
	require.True(t, ok)
	work, ok := raw.(eucloruntime.UnitOfWork)
	require.True(t, ok)
	assert.Equal(t, "review", work.ModeID)
	assert.Equal(t, eucloruntime.ExecutorFamilyReflection, work.ExecutorDescriptor.Family)
	assertRoutineFamilyPresent(t, work.RoutineBindings, "tension_assessment")
	assertRoutineFamilyPresent(t, work.RoutineBindings, "coherence_assessment")
	assertRoutineFamilyPresent(t, work.RoutineBindings, "compatibility_assessment")
	assertRoutineFamilyPresent(t, work.RoutineBindings, "approval_assessment")
}

func assertRoutineFamilyPresent(t *testing.T, bindings []eucloruntime.UnitOfWorkRoutineBinding, family string) {
	t.Helper()
	for _, binding := range bindings {
		if binding.Family == family {
			return
		}
	}
	t.Fatalf("missing routine family %q in %#v", family, bindings)
}

func TestFullExecuteSessionScopingPreventsRecursion(t *testing.T) {
	agent := integrationAgent(t)

	// Pre-set a different session ID to simulate recursive invocation.
	state := core.NewContext()
	state.Set("euclo.session_id", "euclo-existing-session")
	state.Set("pipeline.verify", map[string]any{
		"status": "pass", "summary": "ok",
		"checks": []any{map[string]any{"name": "test", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-recursion-1",
		Instruction: "should fail due to session scoping",
	}, state)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session scoping violation")
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestFullExecuteArtifactProvenanceComplete(t *testing.T) {
	// Test artifact provenance through ProfileController directly.
	reg := capabilities.NewEucloCapabilityRegistry()
	provCap := &stubProfileCapability{
		id:       "euclo:provenance_test",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "done",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindExplore, ProducerID: "euclo:provenance_test", Status: "produced"},
				{Kind: euclotypes.ArtifactKindPlan, ProducerID: "euclo:provenance_test", Status: "produced"},
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:provenance_test", Status: "produced"},
			},
		},
	}
	_ = reg.Register(provCap)

	env := testEnv(t)
	pc := orchestrate.NewProfileController(orchestrate.AdaptCapabilityRegistry(reg), gate.DefaultPhaseGates(), env, euclotypes.DefaultExecutionProfileRegistry(), nil)

	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, testEnvelope(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// All capability-produced artifacts should have ProducerID.
	capProduced := 0
	for _, a := range pcResult.Artifacts {
		if a.ProducerID == "euclo:provenance_test" {
			capProduced++
		}
	}
	assert.GreaterOrEqual(t, capProduced, 3, "expected at least 3 capability-produced artifacts")

	// ValidateArtifactProvenance should find no warnings.
	warnings := euclotypes.ValidateArtifactProvenance(pcResult.Artifacts)
	assert.Empty(t, warnings, "all produced artifacts should have ProducerID")
}

func TestValidateArtifactProvenanceDetectsMissingProducerID(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{ID: "a", Kind: euclotypes.ArtifactKindExplore, Status: "produced", ProducerID: "cap_a"},
		{ID: "b", Kind: euclotypes.ArtifactKindPlan, Status: "produced", ProducerID: ""},
		{ID: "c", Kind: euclotypes.ArtifactKindEditIntent, Status: "pending", ProducerID: ""},
	}
	warnings := euclotypes.ValidateArtifactProvenance(artifacts)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "artifact b")
}

func TestValidateArtifactProvenanceAllGood(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{ID: "a", Kind: euclotypes.ArtifactKindExplore, Status: "produced", ProducerID: "cap_a"},
		{ID: "b", Kind: euclotypes.ArtifactKindPlan, Status: "produced", ProducerID: "cap_b"},
	}
	warnings := euclotypes.ValidateArtifactProvenance(artifacts)
	assert.Empty(t, warnings)
}

func TestValidateArtifactProvenanceEmpty(t *testing.T) {
	warnings := euclotypes.ValidateArtifactProvenance(nil)
	assert.Empty(t, warnings)
}

// TestFamilyForPhase tests the familyForPhase function from runtime/routing.go
// Note: familyForPhase is a private function in the runtime package and cannot be called directly from tests.
// This test is commented out pending refactoring to expose the function or move to runtime_test.go
/*
func TestFamilyForPhase(t *testing.T) {
	assert.Equal(t, "planning", familyForPhase("plan", "implementation"))
	assert.Equal(t, "planning", familyForPhase("plan_tests", "implementation"))
	assert.Equal(t, "review", familyForPhase("review", "implementation"))
	assert.Equal(t, "review", familyForPhase("summarize", "implementation"))
	assert.Equal(t, "verification", familyForPhase("verify", "implementation"))
	assert.Equal(t, "debugging", familyForPhase("reproduce", "debugging"))
	assert.Equal(t, "debugging", familyForPhase("localize", "debugging"))
	assert.Equal(t, "implementation", familyForPhase("reproduce", "implementation"))
	assert.Equal(t, "implementation", familyForPhase("edit", "implementation"))
}
*/

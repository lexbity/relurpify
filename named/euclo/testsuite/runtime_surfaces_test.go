package testsuite

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	eucloarchaeomem "codeburg.org/lexbit/relurpify/named/euclo/runtime/archaeomem"
	eucloreporting "codeburg.org/lexbit/relurpify/named/euclo/runtime/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestAgentExecuteDispatchesThroughBehaviorDispatcher(t *testing.T) {
	agent := euclo.New(testutil.WorkspaceEnv(t))
	task := &core.Task{
		ID:          "task-chat-ask",
		Type:        core.TaskTypeAnalysis,
		Instruction: "What does this code do?",
		Context: map[string]any{
			"workspace": t.TempDir(),
		},
	}
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), task, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	// Use typed accessor to retrieve behavior trace
	trace, ok := euclostate.GetBehaviorTrace(state)
	require.True(t, ok)
	require.Equal(t, euclorelurpic.CapabilityChatAsk, trace.PrimaryCapabilityID)
	require.Contains(t, trace.RecipeIDs, string(execution.RecipeChatAskInquiry))
	require.Equal(t, "unit_of_work_behavior", trace.Path)

	rawAnalyze, ok := state.Get("pipeline.analyze")
	require.True(t, ok)
	analyze, ok := rawAnalyze.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "chat.ask", analyze["mode"])
}

func TestChatRuntimeCarriesBehaviorTrace(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatImplement,
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityChatDirectEditExecution,
			euclorelurpic.CapabilityChatLocalReview,
			euclorelurpic.CapabilityArchaeologyExplore,
		}},
	}
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", execution.Trace{
		Path:                     "unit_of_work_behavior",
		RecipeIDs:                []string{"chat.implement.architect", "chat.implement.edit"},
		SpecializedCapabilityIDs: []string{"euclo.execution.architect", "euclo:refactor.api_compatible"},
	})
	state.Set("pipeline.verify", map[string]any{
		"status": "pass",
		"checks": []any{map[string]any{"name": "architect_review", "status": "pass"}},
	})
	state.Set("euclo.capability_contract_runtime", eucloruntime.CapabilityContractRuntimeState{
		LazySemanticAcquisitionEligible:  true,
		LazySemanticAcquisitionTriggered: true,
	})
	rt := eucloreporting.BuildChatCapabilityRuntimeState(work, state, time.Now().UTC())
	if !rt.ImplementActive {
		t.Fatalf("expected implement runtime active")
	}
	if rt.BehaviorPath != "unit_of_work_behavior" {
		t.Fatalf("expected behavior path to carry through, got %q", rt.BehaviorPath)
	}
	if len(rt.ExecutedRecipeIDs) == 0 || rt.ExecutedRecipeIDs[0] == "" {
		t.Fatalf("expected executed recipes to be recorded")
	}
	if len(rt.SpecializedCapabilityIDs) == 0 {
		t.Fatalf("expected specialized capabilities to be recorded")
	}
	if !rt.LazySemanticAcquisitionTriggered {
		t.Fatalf("expected lazy semantic acquisition to be surfaced")
	}
}

func TestProofSurfaceCarriesRecoveryStateForRepairedRun(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode_resolution", eucloruntime.ModeResolution{ModeID: "code"})
	state.Set("euclo.execution_profile_selection", eucloruntime.ExecutionProfileSelection{ProfileID: "edit_verify_repair"})
	state.Set("euclo.verification", eucloruntime.VerificationEvidence{
		Status:     "pass",
		Provenance: eucloruntime.VerificationProvenanceExecuted,
	})
	state.Set("euclo.success_gate", eucloruntime.SuccessGateResult{
		Allowed:        true,
		Reason:         "verification_accepted",
		AssuranceClass: eucloruntime.AssuranceClassVerifiedSuccess,
	})
	state.Set("euclo.recovery_trace", map[string]any{
		"status":        "repaired",
		"attempt_count": 1,
	})

	proof := eucloreporting.BuildProofSurface(state, euclotypes.CollectArtifactsFromState(state))
	require.Equal(t, "repaired", proof.RecoveryStatus)
	require.Equal(t, 1, proof.RecoveryAttempts)
	require.Equal(t, "verified_success", proof.AssuranceClass)
}

func TestProofSurfaceCarriesAutomaticDegradationState(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode_resolution", eucloruntime.ModeResolution{ModeID: "debug"})
	state.Set("euclo.execution_profile_selection", eucloruntime.ExecutionProfileSelection{ProfileID: "reproduce_localize_patch"})
	state.Set("euclo.success_gate", eucloruntime.SuccessGateResult{
		Allowed:              false,
		Reason:               "verification_missing",
		AssuranceClass:       eucloruntime.AssuranceClassUnverifiedSuccess,
		DegradationMode:      "automatic",
		DegradationReason:    "verification_tools_unavailable",
		AutomaticDegradation: true,
	})

	proof := eucloreporting.BuildProofSurface(state, euclotypes.CollectArtifactsFromState(state))
	require.Equal(t, "automatic", proof.DegradationMode)
	require.Equal(t, "verification_tools_unavailable", proof.DegradationReason)
	require.Equal(t, "unverified_success", proof.AssuranceClass)
}

func TestArchaeologyRuntimeCarriesPolicyAndTrace(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyImplement,
		WorkflowID: "wf-1",
		SemanticInputs: eucloruntime.SemanticInputBundle{
			ExplorationID: "explore-1",
			PatternRefs:   []string{"pattern:a", "pattern:b"},
			TensionRefs:   []string{"tension:a"},
		},
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityArchaeologyCompilePlan,
			euclorelurpic.CapabilityArchaeologyConvergenceGuard,
		},
		PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
			PlanID:        "plan-1",
			PlanVersion:   3,
			IsPlanBacked:  true,
			IsLongRunning: true,
		}},
	}
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", execution.Trace{
		Path:                     "unit_of_work_behavior",
		RecipeIDs:                []string{"archaeology.implement-plan.rewoo"},
		SpecializedCapabilityIDs: []string{"euclo.execution.rewoo"},
	})
	state.Set("euclo.security_runtime", eucloruntime.SecurityRuntimeState{
		PolicySnapshotID:     "policy-1",
		AdmittedCallableCaps: []string{"file_read", "file_write"},
		AdmittedModelTools:   []string{"file_read", "file_write"},
	})
	rt := eucloarchaeomem.BuildArchaeologyCapabilityRuntimeState(work, state, time.Now().UTC())
	if !rt.PlanBound || !rt.HasCompiledPlan {
		t.Fatalf("expected archaeology runtime to be plan-bound and compiled")
	}
	if rt.PolicySnapshotID != "policy-1" {
		t.Fatalf("expected policy snapshot to propagate, got %q", rt.PolicySnapshotID)
	}
	if len(rt.ExecutedRecipeIDs) == 0 {
		t.Fatalf("expected executed recipes to propagate")
	}
	if len(rt.SpecializedCapabilityIDs) == 0 {
		t.Fatalf("expected specialized capabilities to propagate")
	}
}

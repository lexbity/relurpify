package testsuite

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	eucloarchaeomem "github.com/lexcodex/relurpify/named/euclo/runtime/archaeomem"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestAgentExecuteDispatchesThroughBehaviorService(t *testing.T) {
	agent := euclo.New(testutil.Env(t))
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

	rawTrace, ok := state.Get("euclo.relurpic_behavior_trace")
	require.True(t, ok)
	trace, ok := rawTrace.(execution.Trace)
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
	work := eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatImplement,
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityChatDirectEditExecution,
			euclorelurpic.CapabilityChatLocalReview,
			euclorelurpic.CapabilityArchaeologyExplore,
		},
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

func TestArchaeologyRuntimeCarriesPolicyAndTrace(t *testing.T) {
	work := eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyImplement,
		WorkflowID:                  "wf-1",
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
		},
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

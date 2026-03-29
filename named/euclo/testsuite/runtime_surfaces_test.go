package testsuite

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	eucloarchaeomem "github.com/lexcodex/relurpify/named/euclo/runtime/archaeomem"
	eucloreporting "github.com/lexcodex/relurpify/named/euclo/runtime/reporting"
)

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
			PlanID:       "plan-1",
			PlanVersion:  3,
			IsPlanBacked: true,
			IsLongRunning: true,
		},
	}
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", execution.Trace{
		Path:      "unit_of_work_behavior",
		RecipeIDs: []string{"archaeology.implement-plan.rewoo"},
		SpecializedCapabilityIDs: []string{"euclo.execution.rewoo"},
	})
	state.Set("euclo.security_runtime", eucloruntime.SecurityRuntimeState{
		PolicySnapshotID:    "policy-1",
		AdmittedCallableCaps: []string{"file_read", "file_write"},
		AdmittedModelTools:  []string{"file_read", "file_write"},
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

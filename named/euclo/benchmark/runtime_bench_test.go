package benchmark

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

func BenchmarkBuildChatCapabilityRuntimeState(b *testing.B) {
	work := eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatImplement,
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityChatDirectEditExecution,
			euclorelurpic.CapabilityChatLocalReview,
			euclorelurpic.CapabilityChatTargetedVerification,
			euclorelurpic.CapabilityArchaeologyExplore,
		},
	}
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", execution.Trace{
		Path:                     "unit_of_work_behavior",
		RecipeIDs:                []string{"chat.implement.architect", "chat.implement.edit", "chat.implement.verify"},
		SpecializedCapabilityIDs: []string{"euclo.execution.architect", "euclo:review.implement_if_safe"},
	})
	state.Set("pipeline.verify", map[string]any{
		"status": "pass",
		"checks": []any{
			map[string]any{"name": "architect_review", "status": "pass"},
			map[string]any{"name": "verification", "status": "pass"},
		},
	})
	now := time.Now().UTC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eucloreporting.BuildChatCapabilityRuntimeState(work, state, now)
	}
}

func BenchmarkBuildArchaeologyCapabilityRuntimeState(b *testing.B) {
	work := eucloruntime.UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyImplement,
		WorkflowID:                  "wf-bench",
		SemanticInputs: eucloruntime.SemanticInputBundle{
			ExplorationID:         "explore-bench",
			PatternRefs:           []string{"pattern:a", "pattern:b", "pattern:c"},
			TensionRefs:           []string{"tension:a", "tension:b"},
			ProspectiveRefs:       []string{"prospective:a"},
			ConvergenceRefs:       []string{"convergence:a"},
			RequestProvenanceRefs: []string{"request:a", "request:b"},
		},
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityArchaeologyCompilePlan,
			euclorelurpic.CapabilityArchaeologyExplore,
			euclorelurpic.CapabilityArchaeologyConvergenceGuard,
		},
		PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
			PlanID:        "plan-bench",
			PlanVersion:   7,
			IsPlanBacked:  true,
			IsLongRunning: true,
		},
	}
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", execution.Trace{
		Path:                     "unit_of_work_behavior",
		RecipeIDs:                []string{"archaeology.compile.shape", "archaeology.implement.rewoo"},
		SpecializedCapabilityIDs: []string{"euclo.execution.rewoo"},
	})
	state.Set("euclo.security_runtime", eucloruntime.SecurityRuntimeState{
		PolicySnapshotID:    "policy-bench",
		AdmittedCallableCaps: []string{"file_read", "file_write", "cli_go"},
		AdmittedModelTools:  []string{"file_read", "file_write", "cli_go"},
	})
	now := time.Now().UTC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eucloarchaeomem.BuildArchaeologyCapabilityRuntimeState(work, state, now)
	}
}

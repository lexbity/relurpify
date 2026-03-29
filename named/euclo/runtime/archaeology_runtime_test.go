package runtime

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestBuildArchaeologyCapabilityRuntimeStateForCompilePlan(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.security_runtime", SecurityRuntimeState{
		PolicySnapshotID:     "policy-arch",
		AdmittedCallableCaps: []string{"relurpic:pattern_surfacing", "relurpic:prospective_analysis"},
		AdmittedModelTools:   []string{"file_read"},
	})
	rt := BuildArchaeologyCapabilityRuntimeState(UnitOfWork{
		PrimaryRelurpicCapabilityID:     "euclo:archaeology.compile-plan",
		SupportingRelurpicCapabilityIDs: []string{"euclo:archaeology.pattern-surface", "euclo:archaeology.convergence-guard"},
		WorkflowID:                      "wf-1",
		SemanticInputs: SemanticInputBundle{
			ExplorationID:           "explore-1",
			PatternRefs:             []string{"pattern-1", "pattern-2"},
			TensionRefs:             []string{"tension-1"},
			ProspectiveRefs:         []string{"prospect-1"},
			ConvergenceRefs:         []string{"conv-1"},
			LearningInteractionRefs: []string{"learn-1"},
			PendingRequests:         []SemanticRequestRef{{RequestID: "req-1"}},
			CompletedRequests:       []SemanticRequestRef{{RequestID: "req-2"}},
		},
	}, state, time.Unix(123, 0).UTC())
	if rt.PrimaryOperation != "compile_plan" {
		t.Fatalf("unexpected primary operation: %#v", rt)
	}
	if !rt.PrimaryArchaeoAssociated || !rt.PrimaryLLMDependent {
		t.Fatalf("expected archaeology/llm flags: %#v", rt)
	}
	if rt.PatternRefCount != 2 || rt.PendingRequestCount != 1 {
		t.Fatalf("unexpected semantic counts: %#v", rt)
	}
	if len(rt.SupportingOperations) != 2 {
		t.Fatalf("unexpected supporting operations: %#v", rt)
	}
	if rt.PolicySnapshotID != "policy-arch" || len(rt.AdmittedCapabilityIDs) != 2 || len(rt.AdmittedModelTools) != 1 {
		t.Fatalf("expected framework policy context, got %#v", rt)
	}
}

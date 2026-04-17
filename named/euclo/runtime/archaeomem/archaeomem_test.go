package archaeomem

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestBuildArchaeologyCapabilityRuntimeState_ExplorePrimaryPopulatesCounts(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{WorkflowID: "wf-arch",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyExplore,
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityArchaeologyPatternSurface,
		},
		SemanticInputs: eucloruntime.SemanticInputBundle{
			ExplorationID:           "ex-1",
			PatternRefs:             []string{"p1", "p2"},
			TensionRefs:             []string{"t1"},
			ProspectiveRefs:         []string{"pr1"},
			ConvergenceRefs:         []string{"c1"},
			PendingRequests:         []eucloruntime.SemanticRequestRef{{RequestID: "rq1"}},
			CompletedRequests:       []eucloruntime.SemanticRequestRef{{RequestID: "done1"}},
			LearningInteractionRefs: []string{"lr1"},
		},
		PlanBinding: &eucloruntime.UnitOfWorkPlanBinding{
			PlanID:        "plan-z",
			PlanVersion:   3,
			IsPlanBacked:  true,
			IsLongRunning: true,
		}},
	}
	state := core.NewContext()
	state.Set("euclo.relurpic_behavior_trace", execution.Trace{
		RecipeIDs: []string{"archaeology.explore"},
		Path:      "explore",
	})
	state.Set("euclo.security_runtime", eucloruntime.SecurityRuntimeState{
		PolicySnapshotID:     "pol-1",
		AdmittedCallableCaps: []string{"euclo:archaeology.explore"},
		AdmittedModelTools:   []string{"file_read"},
	})

	rt := BuildArchaeologyCapabilityRuntimeState(work, state, time.Unix(1700000000, 0).UTC())
	if !strings.Contains(rt.Summary, "primary=") {
		t.Fatalf("expected operation summary segment, got %q", rt.Summary)
	}
	if rt.PatternRefCount != 2 || rt.TensionRefCount != 1 {
		t.Fatalf("unexpected ref counts: pattern=%d tension=%d", rt.PatternRefCount, rt.TensionRefCount)
	}
	if !rt.HasCompiledPlan || rt.PlanID != "plan-z" || rt.PlanVersion != 3 {
		t.Fatalf("unexpected plan fields: %#v", rt)
	}
	if len(rt.SupportingCapabilityIDs) == 0 {
		t.Fatal("expected archaeology-associated supporting capabilities")
	}
	if len(rt.ExecutedRecipeIDs) != 1 || rt.BehaviorPath != "explore" {
		t.Fatalf("unexpected trace: recipes=%v path=%q", rt.ExecutedRecipeIDs, rt.BehaviorPath)
	}
	if rt.PolicySnapshotID != "pol-1" {
		t.Fatalf("expected policy snapshot id, got %q", rt.PolicySnapshotID)
	}
}

func TestBuildArchaeologyCapabilityRuntimeState_NonArchaeologyPrimaryReturnsEarlyShape(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatAsk}}
	rt := BuildArchaeologyCapabilityRuntimeState(work, core.NewContext(), time.Now().UTC())
	if len(rt.SupportingCapabilityIDs) != 0 {
		t.Fatalf("chat ask should not list archaeology support, got %#v", rt.SupportingCapabilityIDs)
	}
	if rt.Summary != "" {
		t.Fatalf("expected empty summary for non-archaeology primary, got %q", rt.Summary)
	}
}

func TestBuildArchaeologyCapabilityRuntimeState_ConcurrentReadsDoNotRace(t *testing.T) {
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{WorkflowID: "wf-race",
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityArchaeologyCompilePlan,
		SemanticInputs: eucloruntime.SemanticInputBundle{
			PatternRefs: []string{"a"},
			TensionRefs: []string{"b"},
		}},
	}
	state := core.NewContext()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = BuildArchaeologyCapabilityRuntimeState(work, state, time.Now().UTC())
		}()
	}
	wg.Wait()
}

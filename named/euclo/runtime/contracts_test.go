package runtime

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestEnforcePreExecutionCapabilityContractsRequiresCompiledPlan(t *testing.T) {
	err := EnforcePreExecutionCapabilityContracts(UnitOfWork{
		PrimaryRelurpicCapabilityID: "euclo:archaeology.implement-plan",
	})
	if err == nil {
		t.Fatal("expected compiled plan requirement error")
	}
}

func TestEvaluatePostExecutionCapabilityContractsRejectsMutationForChatAsk(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{"summary": "mutation observed"})
	rt, err := EvaluatePostExecutionCapabilityContracts(UnitOfWork{
		PrimaryRelurpicCapabilityID: "euclo:chat.ask",
	}, state, time.Now().UTC())
	if err == nil {
		t.Fatal("expected non-mutating contract error")
	}
	if !rt.Blocked {
		t.Fatalf("expected blocked runtime state: %#v", rt)
	}
}

func TestBuildCapabilityContractDeferredIssuesForCompilePlanWithoutPlan(t *testing.T) {
	issues := BuildCapabilityContractDeferredIssues(UnitOfWork{
		PrimaryRelurpicCapabilityID: "euclo:archaeology.compile-plan",
		WorkflowID:                  "wf-1",
		RunID:                       "run-1",
		ExecutionID:                 "exec-1",
		SemanticInputs: SemanticInputBundle{
			PatternRefs: []string{"pattern-1"},
		},
	}, core.NewContext(), time.Unix(123, 0).UTC())
	if len(issues) != 1 {
		t.Fatalf("expected deferred issue, got %#v", issues)
	}
	if issues[0].RecommendedReentry != "archaeology" {
		t.Fatalf("unexpected deferred issue: %#v", issues[0])
	}
}

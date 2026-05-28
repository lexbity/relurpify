package rex

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/classify"
	"codeburg.org/lexbit/relurpify/named/rex/route"
	"codeburg.org/lexbit/relurpify/named/rex/state"
)

func TestPersistOutcomeWithNilWorkflowDoesNotPanic(t *testing.T) {
	agent := New(testEnv(t))
	env := contextdata.NewEnvelope("task-1", "")
	plan := &executionPlan{
		Identity: state.Identity{
			WorkflowID: "wf-1",
			RunID:      "run-1",
		},
		Classification: classify.Classification{ReadOnly: true, Intent: "review", RiskLevel: "low"},
		Decision: route.RouteDecision{
			Family:       route.FamilyArchitect,
			Mode:         "mutation",
			Profile:      "managed",
			RequireProof: false,
		},
		EventSuffix: "runtime",
	}
	result := &core.Result{Success: true}

	agent.persistOutcome(context.Background(), &core.Task{ID: "task-1"}, env, plan, result, executionSurfaces{})

	if got, ok := core.ResultField(result.Data, "rex.proof_surface"); !ok || got == nil {
		t.Fatal("expected proof surface to be populated")
	}
	if got, ok := core.ResultField(result.Data, "rex.action_log"); !ok || got == nil {
		t.Fatal("expected action log to be populated")
	}
	if agent.lastProof.RouteFamily != route.FamilyArchitect {
		t.Fatalf("lastProof.RouteFamily = %q, want %q", agent.lastProof.RouteFamily, route.FamilyArchitect)
	}
}

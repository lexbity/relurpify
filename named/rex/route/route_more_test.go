package route

import (
	"testing"

	"github.com/lexcodex/relurpify/named/rex/classify"
	"github.com/lexcodex/relurpify/named/rex/envelope"
)

func TestBuildExecutionPlanCopiesDecisionFields(t *testing.T) {
	decision := RouteDecision{
		Family:             FamilyPlanner,
		RequirePersistence: true,
		RequireProof:       true,
		RequireRetrieval:   true,
		Fallbacks:          []string{FamilyReAct, FamilyArchitect},
	}

	plan := BuildExecutionPlan(decision)
	if plan.PrimaryFamily != FamilyPlanner || !plan.RequirePersistence || !plan.RequireRetrieval || !plan.RequireVerification {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	decision.Fallbacks[0] = "changed"
	if plan.Fallbacks[0] != FamilyReAct {
		t.Fatalf("expected plan fallbacks to be copied: %+v", plan.Fallbacks)
	}
}

func TestDecideUsesResumedRouteAndFallbackFamilies(t *testing.T) {
	resumed := Decide(envelope.Envelope{ResumedRoute: FamilyArchitect}, classify.Classification{})
	if resumed.Family != FamilyArchitect || resumed.Mode != "mutation" {
		t.Fatalf("unexpected resumed decision: %+v", resumed)
	}

	planning := Decide(envelope.Envelope{}, classify.Classification{Intent: "planning"})
	if planning.Family != FamilyPlanner || planning.Profile != "read-only" {
		t.Fatalf("unexpected planner decision: %+v", planning)
	}

	react := Decide(envelope.Envelope{}, classify.Classification{})
	if react.Family != FamilyReAct || react.Mode != "open" {
		t.Fatalf("unexpected react decision: %+v", react)
	}
}

func TestDecisionForFamilyDefaultsToReActForUnknownFamily(t *testing.T) {
	decision := decisionForFamily("unknown", classify.Classification{LongRunningManaged: true})
	if decision.Family != FamilyReAct || !decision.RequirePersistence || !decision.RequireRetrieval {
		t.Fatalf("unexpected default decision: %+v", decision)
	}
}

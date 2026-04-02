package gate

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestTestDrivenGenerationGates_RequireTDDLifecycleCompletion(t *testing.T) {
	gates := testDrivenGenerationGates()
	if len(gates) != 2 {
		t.Fatalf("expected two gates, got %d", len(gates))
	}
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "plan", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"steps": []any{"write tests"}}},
		{ID: "edit", Kind: euclotypes.ArtifactKindEditIntent, Payload: map[string]any{"summary": "implemented fix"}},
		{ID: "verify", Kind: euclotypes.ArtifactKindVerification, Payload: map[string]any{"status": "pass"}},
		{ID: "tdd", Kind: euclotypes.ArtifactKindTDDLifecycle, Payload: map[string]any{
			"current_phase": "green",
			"status":        "in_progress",
			"phase_history": []any{
				map[string]any{"phase": "plan_tests", "status": "completed"},
				map[string]any{"phase": "red", "status": "completed"},
			},
		}},
	})

	eval := EvaluateGate(gates[1], "tdd", artifacts)
	if eval.Passed {
		t.Fatal("expected incomplete TDD lifecycle to fail verify gate")
	}
}

func TestTestDrivenGenerationGates_AcceptCompletedTDDLifecycle(t *testing.T) {
	gates := testDrivenGenerationGates()
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{ID: "plan", Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"steps": []any{"write tests"}}},
		{ID: "edit", Kind: euclotypes.ArtifactKindEditIntent, Payload: map[string]any{"summary": "implemented fix"}},
		{ID: "verify", Kind: euclotypes.ArtifactKindVerification, Payload: map[string]any{"status": "pass"}},
		{ID: "tdd", Kind: euclotypes.ArtifactKindTDDLifecycle, Payload: map[string]any{
			"current_phase": "complete",
			"status":        "completed",
			"phase_history": []any{
				map[string]any{"phase": "plan_tests", "status": "completed"},
				map[string]any{"phase": "red", "status": "completed"},
				map[string]any{"phase": "implement", "status": "completed"},
				map[string]any{"phase": "green", "status": "completed"},
				map[string]any{"phase": "complete", "status": "completed"},
			},
		}},
	})

	for _, gate := range gates {
		eval := EvaluateGate(gate, "tdd", artifacts)
		if !eval.Passed {
			t.Fatalf("expected gate %+v to pass, got %#v", gate, eval)
		}
	}
}

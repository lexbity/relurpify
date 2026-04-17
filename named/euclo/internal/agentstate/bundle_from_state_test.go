package agentstate

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestSemanticInputBundleFromState(t *testing.T) {
	state := core.NewContext()
	state.Set("pattern_refs", []string{"bare-pattern"})
	state.Set("archaeology.pattern_refs", []string{"arch-pattern-1"})
	state.Set("debug.pattern_refs", "debug-pattern-1")
	state.Set("tension_refs", []string{"bare-tension"})
	state.Set("archaeology.convergence_refs", []string{"arch-convergence"})
	state.Set("debug.learning_interaction_refs", []any{"debug-learning-1", 42})
	state.Set("workflow_id", "workflow-123")
	state.Set("exploration_id", "exploration-456")

	bundle := SemanticInputBundleFromState(state)

	if got := bundle.PatternRefs; len(got) != 3 || got[0] != "bare-pattern" || got[1] != "arch-pattern-1" || got[2] != "debug-pattern-1" {
		t.Fatalf("unexpected pattern refs: %#v", got)
	}
	if got := bundle.TensionRefs; len(got) != 1 || got[0] != "bare-tension" {
		t.Fatalf("unexpected tension refs: %#v", got)
	}
	if got := bundle.ConvergenceRefs; len(got) != 1 || got[0] != "arch-convergence" {
		t.Fatalf("expected archaeology.convergence_refs to be included, got %#v", got)
	}
	if got := bundle.LearningInteractionRefs; len(got) != 1 || got[0] != "debug-learning-1" {
		t.Fatalf("expected debug.learning_interaction_refs to be included, got %#v", got)
	}
	if got := bundle.WorkflowID; got != "workflow-123" {
		t.Fatalf("unexpected workflow id: %q", got)
	}
	if got := bundle.ExplorationID; got != "exploration-456" {
		t.Fatalf("unexpected exploration id: %q", got)
	}
}


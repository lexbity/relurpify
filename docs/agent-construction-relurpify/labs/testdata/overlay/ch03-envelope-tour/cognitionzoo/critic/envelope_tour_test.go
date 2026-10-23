package critic_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// TestEnvelopeTour walks one envelope through the lifecycle Chapter 3
// teaches: values enter working memory with a class and an origin, a
// parallel branch works on a clone, and only the branch's delta merges
// back into the parent.
func TestEnvelopeTour(t *testing.T) {
	env := &contextdata.Envelope{}
	env.SetTaskID("tour-1")
	env.SetNodeID("draft")

	// Working memory is the only mutable tier for graph nodes; every
	// value carries its memory class and its origin.
	env.SetWorkingValueWithOrigin("critic.draft", "first draft",
		contextdata.MemoryClassTask, contextdata.OriginLLM)

	draft, ok := contextdata.GetTyped[string](env, "critic.draft")
	if !ok || draft != "first draft" {
		t.Fatalf("GetTyped(critic.draft) = %q, %v", draft, ok)
	}
	if got := env.OriginOf("critic.draft"); got != contextdata.OriginLLM {
		t.Fatalf("OriginOf(critic.draft) = %q, want llm", got)
	}

	// A parallel branch runs on a clone, never on the parent envelope.
	branch := contextdata.CloneEnvelope(env)
	branch.SetNodeID("critique")
	branch.SetWorkingValueWithOrigin("critic.verdict", "issues found",
		contextdata.MemoryClassTask, contextdata.OriginLLM)

	// The parent is untouched by the branch's write.
	if _, ok := contextdata.GetTyped[string](env, "critic.verdict"); ok {
		t.Fatal("parent envelope saw a branch-local key before merge")
	}

	// Merge applies the branch's delta, not its whole state.
	delta := contextdata.ComputeBranchDelta(env, branch)
	if len(delta.WorkingMemoryAdded) != 1 || delta.WorkingMemoryAdded[0] != "critic.verdict" {
		t.Fatalf("delta additions = %v, want [critic.verdict]", delta.WorkingMemoryAdded)
	}
	stats, err := env.ApplyBranchMerges([]contextdata.BranchMergeUnit{
		{Index: 0, ID: "critique", Delta: delta, Env: branch},
	})
	if err != nil {
		t.Fatalf("ApplyBranchMerges: %v", err)
	}
	if stats.UnitsApplied != 1 || stats.KeysWritten != 1 {
		t.Fatalf("stats = %+v, want 1 unit, 1 key", stats)
	}
	verdict, ok := contextdata.GetTyped[string](env, "critic.verdict")
	if !ok || verdict != "issues found" {
		t.Fatalf("post-merge verdict = %q, %v", verdict, ok)
	}
}

package conformance

import (
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/model"
)

// critic_conformance_test.go registers the critic paradigm's declared
// conformance runners (book 2, "Agent Construction with Relurpify",
// Chapter 11). The runners live in their own file so the book's
// contribution stays self-contained; the eventual PR folds the entries
// into the conformanceRunners literal proper, next to reflection's.

func init() {
	conformanceRunners["critic/critique_writes_scratch"] = runCriticCritiqueWritesScratch
	conformanceRunners["critic/revise_fires_on_predicate"] = runCriticReviseFiresOnPredicate
	conformanceRunners["critic/revision_cap_bounded"] = runCriticRevisionCapBounded
	conformanceRunners["critic/critique_pass_ends_loop"] = runCriticCritiquePassEndsLoop
}

// runCriticCritiqueWritesScratch proves the `critique` directive writes
// the verdict to the ephemeral scratch namespace.
func runCriticCritiqueWritesScratch(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{
		text:      "acknowledged",
		chatQueue: []string{`{"verdict":"issues","issues":["missing error handling"]}`},
	}
	env := runFixture(t, "critic_critique.erpe", paradigmDeps(mdl, reg))

	if verdict, _ := contextdata.GetTyped[string](env, "scratch.critic.verdict"); verdict != "issues" {
		t.Fatalf("scratch.critic.verdict = %q, want issues", verdict)
	}
	issues, ok := contextdata.GetTyped[any](env, "scratch.critic.issues")
	if !ok || fmt.Sprint(issues) != "[missing error handling]" {
		t.Fatalf("scratch.critic.issues = %v", issues)
	}
}

// runCriticReviseFiresOnPredicate proves the revise body runs exactly once
// when scratch.critic.verdict contains issues and the loop re-critiques to
// a pass.
func runCriticReviseFiresOnPredicate(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{
		text:      "acknowledged",
		chatQueue: []string{`{"verdict":"issues","issues":["a"]}`, `{"verdict":"pass","issues":[]}`},
	}
	env := runFixture(t, "critic_revise.erpe", paradigmDeps(mdl, reg))

	if verdict, _ := contextdata.GetTyped[string](env, "scratch.critic.verdict"); verdict != "pass" {
		t.Fatalf("scratch.critic.verdict after re-critique = %q, want pass", verdict)
	}
	// work react + body react (ChatWithTools) and two critiques (Chat) = 4 calls.
	if got := mdl.callCount(); got != 4 {
		t.Fatalf("model calls = %d, want 4 (work + critique + body + re-critique)", got)
	}
	if revisions, _ := contextdata.GetTyped[int](env, "state.critic_revisions"); revisions != 1 {
		t.Fatalf("captured state.critic_revisions = %d, want 1", revisions)
	}
}

// runCriticRevisionCapBounded proves persistent issues run at most
// MaxRevisionCycles bodies and the last verdict stands.
func runCriticRevisionCapBounded(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{
		text: "acknowledged",
		chatQueue: []string{
			`{"verdict":"issues","issues":["a"]}`,
			`{"verdict":"issues","issues":["a"]}`,
			`{"verdict":"issues","issues":["a"]}`,
		},
	}
	env := runFixture(t, "critic_revise.erpe", paradigmDeps(mdl, reg))

	if verdict, _ := contextdata.GetTyped[string](env, "scratch.critic.verdict"); verdict != "issues" {
		t.Fatalf("scratch.critic.verdict at cap = %q, want issues (last verdict stands)", verdict)
	}
	if revisions, _ := contextdata.GetTyped[int](env, "state.critic_revisions"); revisions != 2 {
		t.Fatalf("captured state.critic_revisions = %d, want 2 (MaxRevisionCycles)", revisions)
	}
}

// runCriticCritiquePassEndsLoop proves a pass verdict ends the loop without
// executing the revise body.
func runCriticCritiquePassEndsLoop(t *testing.T) {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{
		text:      "acknowledged",
		chatQueue: []string{`{"verdict":"pass","issues":[]}`},
	}
	env := runFixture(t, "critic_pass.erpe", paradigmDeps(mdl, reg))

	if verdict, _ := contextdata.GetTyped[string](env, "scratch.critic.verdict"); verdict != "pass" {
		t.Fatalf("scratch.critic.verdict = %q, want pass", verdict)
	}
	// work react (ChatWithTools) + one critique (Chat) = 2 calls; the revise
	// body never ran.
	if got := mdl.callCount(); got != 2 {
		t.Fatalf("model calls = %d, want 2 (work + critique; no revise body)", got)
	}
}

// streamMatrixCritic proves the critic renders the compiled slice into the
// critique's system slot (D-2): the seeded sentinel must reach a model call.
func streamMatrixCritic(t *testing.T) [][]model.Message {
	t.Helper()
	reg := registry.NewRegistry()
	mdl := &conformanceRecordingModel{
		text:      "acknowledged",
		chatQueue: []string{`{"verdict":"pass","issues":[]}`},
	}
	env := contextdata.NewEnvelope("task-stream-critic", "session-stream")
	seedStreamSlice(t, env, "critic")
	runFixtureInto(t, "critic_critique.erpe", paradigmDeps(mdl, reg), env)
	return mdl.allMessages()
}

func init() {
	// Matrix annotations: the critic's cases were implemented with the
	// paradigm (book 2), not honored from an earlier phase.
	for _, cas := range []string{
		"critic/critique_writes_scratch",
		"critic/revise_fires_on_predicate",
		"critic/revision_cap_bounded",
		"critic/critique_pass_ends_loop",
	} {
		caseStatus[cas] = "implemented"
	}
}

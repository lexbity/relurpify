package state_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestInteractionFrameSeq_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")

	_, ok := state.GetInteractionFrameSeq(env)
	if ok {
		t.Fatal("expected absent before Set")
	}

	state.SetInteractionFrameSeq(env, 5)
	n, ok := state.GetInteractionFrameSeq(env)
	if !ok {
		t.Fatal("expected present after Set")
	}
	if n != 5 {
		t.Fatalf("got %d, want 5", n)
	}
}

func TestInteractionFrameRequested_DefaultFalse(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if state.GetInteractionFrameRequested(env) {
		t.Fatal("expected false for absent key")
	}
	state.SetInteractionFrameRequested(env, true)
	if !state.GetInteractionFrameRequested(env) {
		t.Fatal("expected true after Set")
	}
}

func TestInteractionResumeNodeID_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetInteractionResumeNodeID(env, "clarification-gate")
	if v := state.GetInteractionResumeNodeID(env); v != "clarification-gate" {
		t.Fatalf("got %q, want %q", v, "clarification-gate")
	}
}

func TestInteractionPause_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetInteractionPause(env, true)
	if !state.GetInteractionPause(env) {
		t.Fatal("expected true after Set")
	}
}

func TestAskState_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetAskQuestion(env, "Which package should I refactor?")
	state.SetAskChoices(env, []string{"framework/core", "named/rex", "platform/fs"})
	state.SetAskChoiceSource(env, "euclo.orchestrate.clarification")

	if q := state.GetAskQuestion(env); q != "Which package should I refactor?" {
		t.Fatalf("question: got %q", q)
	}
	choices, ok := state.GetAskChoices(env)
	if !ok || len(choices) != 3 {
		t.Fatalf("choices: got ok=%v len=%d", ok, len(choices))
	}
	if src := state.GetAskChoiceSource(env); src != "euclo.orchestrate.clarification" {
		t.Fatalf("source: got %q", src)
	}
}

package state_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestRouteContinuation_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	want := &euclotypes.RouteContinuation{
		SharedContext:         true,
		SourceRouteKind:       "thoughtrecipe",
		SourceRouteID:         "euclo.thoughtrecipe.intent.review",
		TargetRouteKind:       "thoughtrecipe",
		TargetRouteID:         "euclo.thoughtrecipe.intent.review",
		ActiveThoughtRecipeID: "euclo.thoughtrecipe.intent.review",
	}

	state.SetRouteContinuation(env, want)

	got, ok := state.GetRouteContinuation(env)
	if !ok {
		t.Fatal("expected present after Set")
	}
	if got != want {
		t.Fatalf("pointer identity lost: got %p, want %p", got, want)
	}
}

func TestRouteCandidateCount_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")

	_, ok := state.GetRouteCandidateCount(env)
	if ok {
		t.Fatal("expected absent before Set")
	}

	state.SetRouteCandidateCount(env, 3)
	n, ok := state.GetRouteCandidateCount(env)
	if !ok {
		t.Fatal("expected present after Set")
	}
	if n != 3 {
		t.Fatalf("got %d, want 3", n)
	}
}

func TestRouteFallback_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")

	state.SetRouteFallbackTaken(env, true)
	state.SetRouteFallbackID(env, "fallback-cap-1")

	if !state.GetRouteFallbackTaken(env) {
		t.Fatal("FallbackTaken: expected true")
	}
	if id := state.GetRouteFallbackID(env); id != "fallback-cap-1" {
		t.Fatalf("FallbackID: got %q, want %q", id, "fallback-cap-1")
	}
}

func TestRouteSkillFilter_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetRouteSkillFilter(env, "go-analysis")
	if v := state.GetRouteSkillFilter(env); v != "go-analysis" {
		t.Fatalf("got %q, want %q", v, "go-analysis")
	}
}

func TestRouteTelemetryOff_DefaultFalse(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if state.GetRouteTelemetryOff(env) {
		t.Fatal("expected false for absent key")
	}
}

func TestSkillFilter_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetSkillFilter(env, "rust")
	if v := state.GetSkillFilter(env); v != "rust" {
		t.Fatalf("got %q, want %q", v, "rust")
	}
}

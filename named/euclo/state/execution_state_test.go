package state_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestExecutionKind_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")

	state.SetExecutionKind(env, state.ExecutionKindCapability)
	if v := state.GetExecutionKind(env); v != state.ExecutionKindCapability {
		t.Fatalf("got %q, want %q", v, state.ExecutionKindCapability)
	}

	state.SetExecutionKind(env, state.ExecutionKindThoughtRecipe)
	if v := state.GetExecutionKind(env); v != state.ExecutionKindThoughtRecipe {
		t.Fatalf("got %q, want %q", v, state.ExecutionKindThoughtRecipe)
	}
}

func TestExecutionCapabilityID_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetExecutionCapabilityID(env, "euclo:cap.ast_query")
	if v := state.GetExecutionCapabilityID(env); v != "euclo:cap.ast_query" {
		t.Fatalf("got %q", v)
	}
}

func TestExecutionCompleted_DefaultFalse(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if state.GetExecutionCompleted(env) {
		t.Fatal("expected false for absent key")
	}
	state.SetExecutionCompleted(env, true)
	if !state.GetExecutionCompleted(env) {
		t.Fatal("expected true after Set")
	}
}

func TestDone_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if state.GetDone(env) {
		t.Fatal("expected false before Set")
	}
	state.SetDone(env, true)
	if !state.GetDone(env) {
		t.Fatal("expected true after Set")
	}
}

func TestForkBranch_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetForkBranch(env, "branch-a")
	if v := state.GetForkBranch(env); v != "branch-a" {
		t.Fatalf("got %q, want %q", v, "branch-a")
	}
}

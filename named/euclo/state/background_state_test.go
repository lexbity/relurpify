package state_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestBackgroundJobID_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetBackgroundJobID(env, "job-abc-123")
	if v := state.GetBackgroundJobID(env); v != "job-abc-123" {
		t.Fatalf("got %q, want %q", v, "job-abc-123")
	}
}

func TestBackgroundJobQueue_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetBackgroundJobQueue(env, "context_stream")
	if v := state.GetBackgroundJobQueue(env); v != "context_stream" {
		t.Fatalf("got %q, want %q", v, "context_stream")
	}
}

func TestBackgroundJobKind_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetBackgroundJobKind(env, "ingestion")
	if v := state.GetBackgroundJobKind(env); v != "ingestion" {
		t.Fatalf("got %q, want %q", v, "ingestion")
	}
}

func TestBackgroundJobSubmitted_DefaultFalse(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if state.GetBackgroundJobSubmitted(env) {
		t.Fatal("expected false for absent key")
	}
	state.SetBackgroundJobSubmitted(env, true)
	if !state.GetBackgroundJobSubmitted(env) {
		t.Fatal("expected true after Set")
	}
}

func TestBackgroundJobState_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetBackgroundJobState(env, "running")
	if v := state.GetBackgroundJobState(env); v != "running" {
		t.Fatalf("got %q, want %q", v, "running")
	}
}

func TestBackgroundJobCompleted_DefaultFalse(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if state.GetBackgroundJobCompleted(env) {
		t.Fatal("expected false for absent key")
	}
	state.SetBackgroundJobCompleted(env, true)
	if !state.GetBackgroundJobCompleted(env) {
		t.Fatal("expected true after Set")
	}
}

func TestBackgroundJobCompletion_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")

	_, ok := state.GetBackgroundJobCompletion(env)
	if ok {
		t.Fatal("expected absent before Set")
	}

	payload := map[string]any{"status": "ok", "count": 42}
	state.SetBackgroundJobCompletion(env, payload)

	got, ok := state.GetBackgroundJobCompletion(env)
	if !ok {
		t.Fatal("expected present after Set")
	}
	if got["status"] != "ok" {
		t.Fatalf("status: got %v, want %q", got["status"], "ok")
	}
}

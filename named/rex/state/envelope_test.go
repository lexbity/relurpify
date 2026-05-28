package state_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/rex/state"
)

func TestEnvelopeWorkflowID_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetEnvelopeWorkflowID(env, "  wf-999  ")
	if v := state.EnvelopeWorkflowID(env); v != "wf-999" {
		t.Fatalf("got %q, want %q", v, "wf-999")
	}
}

func TestEnvelopeRunID_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	state.SetEnvelopeRunID(env, "run-42")
	if v := state.EnvelopeRunID(env); v != "run-42" {
		t.Fatalf("got %q, want %q", v, "run-42")
	}
}

func TestResumedRoute_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if v := state.ResumedRoute(env); v != "" {
		t.Fatalf("expected empty before Set, got %q", v)
	}
	state.SetResumedRoute(env, "euclo.thoughtrecipe.intent.review")
	if v := state.ResumedRoute(env); v != "euclo.thoughtrecipe.intent.review" {
		t.Fatalf("got %q, want %q", v, "euclo.thoughtrecipe.intent.review")
	}
}

func TestArtifactKinds_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if v := state.ArtifactKinds(env); len(v) != 0 {
		t.Fatalf("expected nil/empty before Set, got %v", v)
	}
	state.SetArtifactKinds(env, []string{"diff", "summary"})
	got := state.ArtifactKinds(env)
	if len(got) != 2 || got[0] != "diff" || got[1] != "summary" {
		t.Fatalf("got %v, want [diff summary]", got)
	}
}

func TestEventType_ReturnsEmpty_WhenAbsent(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if v := state.EventType(env); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestEventID_ReturnsEmpty_WhenAbsent(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if v := state.EventID(env); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestAdmissionTenantID_ReturnsEmpty_WhenAbsent(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	if v := state.AdmissionTenantID(env); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestGatewaySessionID_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	contextdata.SetTyped(env, "gateway.session_id", "sess-77")
	if v := state.GatewaySessionID(env); v != "sess-77" {
		t.Fatalf("got %q, want %q", v, "sess-77")
	}
}

func TestGatewayTenantID_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	contextdata.SetTyped(env, "gateway.tenant_id", "tenant-acme")
	if v := state.GatewayTenantID(env); v != "tenant-acme" {
		t.Fatalf("got %q, want %q", v, "tenant-acme")
	}
}

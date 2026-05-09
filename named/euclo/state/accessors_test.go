package state

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
)

func TestGetStringHandlesNilAndTypeMismatch(t *testing.T) {
	if got := GetString(nil, KeyContextHint); got != "" {
		t.Fatalf("expected empty string for nil envelope, got %q", got)
	}

	env := contextdata.NewEnvelope("task", "session")
	env.SetWorkingValue(KeyContextHint, 123, contextdata.MemoryClassTask)
	if got := GetString(env, KeyContextHint); got != "" {
		t.Fatalf("expected empty string for non-string value, got %q", got)
	}
}

func TestPolicyDecisionRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")
	decision := &policy.PolicyDecision{
		MutationPermitted:    true,
		HITLRequired:         true,
		VerificationRequired: true,
		ReasonCodes:          []string{"mutating_family", "verification_required"},
	}

	SetPolicyDecision(env, decision)
	got, ok := GetPolicyDecision(env)
	if !ok {
		t.Fatal("expected policy decision to round-trip")
	}
	if got != decision {
		t.Fatal("expected to retrieve the same policy decision pointer")
	}
}

func TestDispatchRouteKindRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")

	SetDispatchRouteKind(env, "thoughtrecipe")
	got, ok := GetDispatchRouteKind(env)
	if !ok {
		t.Fatal("expected dispatch route kind to round-trip")
	}
	if got != "thoughtrecipe" {
		t.Fatalf("expected route kind thoughtrecipe, got %q", got)
	}
}

func TestAccessorsHandleNilEnvelope(t *testing.T) {
	if got, ok := GetPolicyDecision(nil); ok || got != nil {
		t.Fatalf("expected nil policy decision on nil envelope, got %v ok=%v", got, ok)
	}
	if got, ok := GetDispatchRouteKind(nil); ok || got != "" {
		t.Fatalf("expected empty dispatch route kind on nil envelope, got %q ok=%v", got, ok)
	}
	if got, ok := GetIntentEvidence(nil); ok || got != nil {
		t.Fatalf("expected nil intent evidence on nil envelope, got %v ok=%v", got, ok)
	}
	if got, ok := GetIntentInterpretation(nil); ok || got != nil {
		t.Fatalf("expected nil intent interpretation on nil envelope, got %v ok=%v", got, ok)
	}
	if got, ok := GetRouteResolution(nil); ok || got != nil {
		t.Fatalf("expected nil route resolution on nil envelope, got %v ok=%v", got, ok)
	}
}

func TestIntentEvidenceAccessorsRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")
	evidence := &intentcontext.IntentEvidence{
		ActionType:            "review",
		Target:                "named/euclo",
		ExplicitFiles:         []string{"named/euclo/state/keys.go"},
		MissingFields:         []string{"route"},
		RequiresClarification: true,
	}

	SetIntentEvidence(env, evidence)
	got, ok := GetIntentEvidence(env)
	if !ok {
		t.Fatal("expected intent evidence to round-trip")
	}
	if got.Target != evidence.Target {
		t.Fatalf("Target = %q, want %q", got.Target, evidence.Target)
	}
}

func TestRouteResolutionAccessorRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task", "session")
	resolution := &orchestrate.RouteResolution{
		RouteKind:                 orchestrate.RouteKindIntent,
		ThoughtRecipeID:           "euclo.thoughtrecipe.intent.clarify",
		ResolutionSource:          "registry",
		ClarificationStateVersion: 11,
	}

	SetRouteResolution(env, resolution)
	got, ok := GetRouteResolution(env)
	if !ok {
		t.Fatal("expected route resolution to round-trip")
	}
	if got.RouteID() != resolution.RouteID() {
		t.Fatalf("RouteID = %q, want %q", got.RouteID(), resolution.RouteID())
	}
}

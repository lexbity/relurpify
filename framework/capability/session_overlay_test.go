package capability

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// stubSessionHandler is a minimal InvocableCapabilityHandler for tests.
type stubSessionHandler struct {
	id     string
	called bool
}

func (h *stubSessionHandler) Descriptor(_ context.Context, _ *contextdata.Envelope) CapabilityDescriptor {
	return CapabilityDescriptor{ID: h.id}
}

func (h *stubSessionHandler) Invoke(_ context.Context, _ *contextdata.Envelope, _ map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	h.called = true
	return &contracts.CapabilityExecutionResult{Success: true, Data: map[string]any{"source": "session"}}, nil
}

func TestRegisterSessionCapability_NilEnvelope(t *testing.T) {
	handler := &stubSessionHandler{id: "test:cap.x"}
	if err := RegisterSessionCapability(nil, "test:cap.x", handler); err == nil {
		t.Fatal("expected error for nil envelope")
	}
}

func TestRegisterSessionCapability_EmptyID(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	handler := &stubSessionHandler{id: "test:cap.x"}
	if err := RegisterSessionCapability(env, "", handler); err == nil {
		t.Fatal("expected error for empty capability id")
	}
}

func TestRegisterSessionCapability_NilHandler(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	if err := RegisterSessionCapability(env, "test:cap.x", nil); err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestRegisterSessionCapability_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	want := &stubSessionHandler{id: "test:cap.x"}

	if err := RegisterSessionCapability(env, "test:cap.x", want); err != nil {
		t.Fatalf("RegisterSessionCapability: %v", err)
	}

	got, ok := LookupSessionCapability(env, "test:cap.x")
	if !ok {
		t.Fatal("expected handler to be found after registration")
	}
	if got != want {
		t.Fatal("expected the same handler instance back")
	}
}

func TestLookupSessionCapability_NilEnvelope(t *testing.T) {
	if _, ok := LookupSessionCapability(nil, "test:cap.x"); ok {
		t.Fatal("expected false for nil envelope")
	}
}

func TestLookupSessionCapability_EmptyID(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	if _, ok := LookupSessionCapability(env, ""); ok {
		t.Fatal("expected false for empty capability id")
	}
}

func TestLookupSessionCapability_Missing(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	if _, ok := LookupSessionCapability(env, "test:cap.not_registered"); ok {
		t.Fatal("expected false for unregistered capability")
	}
}

func TestInvokeCapability_SessionOverrideTakesPrecedence(t *testing.T) {
	reg := NewRegistry()
	env := contextdata.NewEnvelope("t", "s")

	handler := &stubSessionHandler{id: "test:cap.override"}
	if err := RegisterSessionCapability(env, "test:cap.override", handler); err != nil {
		t.Fatalf("RegisterSessionCapability: %v", err)
	}

	// The capability is NOT in the global registry — session overlay must be used.
	result, err := reg.InvokeCapability(context.Background(), env, "test:cap.override", nil)
	if err != nil {
		t.Fatalf("InvokeCapability: %v", err)
	}
	if !result.Success {
		t.Fatal("expected successful result from session handler")
	}
	if result.Data["source"] != "session" {
		t.Fatalf("expected source=session, got %v", result.Data["source"])
	}
	if !handler.called {
		t.Fatal("expected session handler to be called")
	}
}

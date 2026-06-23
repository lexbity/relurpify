package registry

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

type sessionOverlayHandler struct {
	desc descriptor.CapabilityDescriptor
}

func (h sessionOverlayHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return h.desc
}

func (h sessionOverlayHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}

var _ handler.InvocableCapabilityHandler = sessionOverlayHandler{}

func TestRegisterSessionCapabilityRejectsUnavailableHandler(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	h := sessionOverlayHandler{
		desc: descriptor.CapabilityDescriptor{
			ID: "session:blocked",
			Availability: descriptor.AvailabilitySpec{
				Available: false,
				Reason:    "tool dependency missing: file_read",
			},
		},
	}

	err := RegisterSessionCapability(env.State(), h.desc.ID, h)
	if err == nil {
		t.Fatal("expected unavailable session capability to be rejected")
	}
	if got := err.Error(); got != "capability session:blocked unavailable: tool dependency missing: file_read" {
		t.Fatalf("unexpected error: %q", got)
	}
	if _, ok := LookupSessionCapability(env.State(), h.desc.ID); ok {
		t.Fatal("unavailable session capability should not be stored")
	}
}

func TestRegisterSessionCapabilityStoresAvailableHandler(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	h := sessionOverlayHandler{
		desc: descriptor.CapabilityDescriptor{
			ID: "session:ok",
			Availability: descriptor.AvailabilitySpec{
				Available: true,
			},
		},
	}

	if err := RegisterSessionCapability(env.State(), h.desc.ID, h); err != nil {
		t.Fatalf("RegisterSessionCapability returned error: %v", err)
	}
	stored, ok := LookupSessionCapability(env.State(), h.desc.ID)
	if !ok {
		t.Fatal("expected session capability to be stored")
	}
	if stored == nil {
		t.Fatal("stored handler should not be nil")
	}
}

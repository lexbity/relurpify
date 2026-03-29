package relurpic

import "testing"

func TestDefaultRegistryLookup(t *testing.T) {
	reg := DefaultRegistry()
	desc, ok := reg.Lookup(CapabilityChatImplement)
	if !ok {
		t.Fatal("expected chat implement descriptor")
	}
	if !desc.PrimaryCapable {
		t.Fatalf("expected primary-capable descriptor: %#v", desc)
	}
	if desc.ModeFamily != "chat" {
		t.Fatalf("unexpected mode family: %#v", desc)
	}
}

func TestDefaultRegistryIDsForMode(t *testing.T) {
	reg := DefaultRegistry()
	ids := reg.IDsForMode("planning")
	if len(ids) < 3 {
		t.Fatalf("expected planning capability ids, got %#v", ids)
	}
}

func TestDefaultRegistrySupportingForPrimary(t *testing.T) {
	reg := DefaultRegistry()
	ids := reg.SupportingForPrimary(CapabilityDebugInvestigate)
	if len(ids) < 4 {
		t.Fatalf("expected debug supporting capability ids, got %#v", ids)
	}
}

package relurpicabilities_test

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

// ---------------------------------------------------------------------------
// NewRegistry
// ---------------------------------------------------------------------------

func TestNewRegistry_IsEmpty(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	if ids := r.IDsForMode("chat"); len(ids) != 0 {
		t.Fatalf("expected empty registry, got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func TestRegister_NilRegistryDoesNotPanic(t *testing.T) {
	var r *relurpicabilities.Registry
	if err := r.Register(relurpicabilities.Descriptor{ID: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegister_EmptyIDIsIgnored(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	_ = r.Register(relurpicabilities.Descriptor{ID: ""})
	if ids := r.IDsForMode(""); len(ids) != 0 {
		t.Fatalf("expected empty registry after empty-ID registration, got %v", ids)
	}
}

func TestRegister_AddsDescriptor(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	_ = r.Register(relurpicabilities.Descriptor{
		ID:         "euclo:chat.ask",
		ModeFamily: "chat",
	})
	desc, ok := r.Lookup("euclo:chat.ask")
	if !ok {
		t.Fatal("expected to find registered descriptor")
	}
	if desc.ModeFamily != "chat" {
		t.Fatalf("unexpected ModeFamily: %q", desc.ModeFamily)
	}
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

func TestLookup_NilRegistryReturnsFalse(t *testing.T) {
	var r *relurpicabilities.Registry
	_, ok := r.Lookup("anything")
	if ok {
		t.Fatal("expected false from nil registry")
	}
}

func TestLookup_UnknownIDReturnsFalse(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	_, ok := r.Lookup("missing")
	if ok {
		t.Fatal("expected false for unknown ID")
	}
}

// ---------------------------------------------------------------------------
// IDsForMode
// ---------------------------------------------------------------------------

func TestIDsForMode_NilRegistryReturnsNil(t *testing.T) {
	var r *relurpicabilities.Registry
	if ids := r.IDsForMode("chat"); ids != nil {
		t.Fatalf("expected nil, got %v", ids)
	}
}

func TestIDsForMode_ReturnsSortedIDs(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:chat.z", ModeFamily: "chat"})
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:chat.a", ModeFamily: "chat"})
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:debug.x", ModeFamily: "debug"})
	ids := r.IDsForMode("chat")
	if len(ids) != 2 {
		t.Fatalf("expected 2 chat capabilities, got %v", ids)
	}
	if ids[0] != "euclo:chat.a" || ids[1] != "euclo:chat.z" {
		t.Fatalf("expected sorted order, got %v", ids)
	}
}

func TestIDsForMode_FiltersCorrectly(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:chat.ask", ModeFamily: "chat"})
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:debug.investigate", ModeFamily: "debug"})
	if ids := r.IDsForMode("debug"); len(ids) != 1 || ids[0] != "euclo:debug.investigate" {
		t.Fatalf("expected only debug capability, got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// SupportingForPrimary
// ---------------------------------------------------------------------------

func TestSupportingForPrimary_NilRegistryReturnsNil(t *testing.T) {
	var r *relurpicabilities.Registry
	if ids := r.SupportingForPrimary("x"); ids != nil {
		t.Fatalf("expected nil, got %v", ids)
	}
}

func TestSupportingForPrimary_UnknownIDReturnsNil(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	if ids := r.SupportingForPrimary("missing"); ids != nil {
		t.Fatalf("expected nil, got %v", ids)
	}
}

func TestSupportingForPrimary_ReturnsSortedSupportingIDs(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	_ = r.Register(relurpicabilities.Descriptor{
		ID:                     "euclo:chat.implement",
		SupportingCapabilities: []string{"euclo:chat.local-review", "euclo:chat.inspect"},
	})
	ids := r.SupportingForPrimary("euclo:chat.implement")
	if len(ids) != 2 {
		t.Fatalf("expected 2 supporting IDs, got %v", ids)
	}
	if ids[0] != "euclo:chat.inspect" || ids[1] != "euclo:chat.local-review" {
		t.Fatalf("expected sorted order, got %v", ids)
	}
}

func TestSupportingForPrimary_EmptySupportingCapabilities(t *testing.T) {
	r := relurpicabilities.NewRegistry()
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:chat.ask"})
	ids := r.SupportingForPrimary("euclo:chat.ask")
	if len(ids) != 0 {
		t.Fatalf("expected empty, got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// DefaultRegistry
// ---------------------------------------------------------------------------

func TestDefaultRegistry_ContainsPrimaryCapabilities(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	primaries := []string{
		relurpicabilities.CapabilityChatAsk,
		relurpicabilities.CapabilityChatImplement,
		relurpicabilities.CapabilityChatInspect,
		relurpicabilities.CapabilityArchaeologyExplore,
		relurpicabilities.CapabilityArchaeologyCompilePlan,
		relurpicabilities.CapabilityArchaeologyImplement,
		relurpicabilities.CapabilityDebugInvestigate,
	}
	for _, id := range primaries {
		if _, ok := r.Lookup(id); !ok {
			t.Errorf("expected primary capability %q in default registry", id)
		}
	}
}

func TestDefaultRegistry_ContainsSupportingCapabilities(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	supporting := []string{
		relurpicabilities.CapabilityChatDirectEditExecution,
		relurpicabilities.CapabilityChatLocalReview,
		relurpicabilities.CapabilityDebugRootCause,
		relurpicabilities.CapabilityDebugLocalization,
		relurpicabilities.CapabilityArchaeologyPatternSurface,
		relurpicabilities.CapabilityArchaeologyScopeExpand,
	}
	for _, id := range supporting {
		if _, ok := r.Lookup(id); !ok {
			t.Errorf("expected supporting capability %q in default registry", id)
		}
	}
}

func TestDefaultRegistry_ChatCapabilitiesHaveChatModeFamily(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	desc, ok := r.Lookup(relurpicabilities.CapabilityChatAsk)
	if !ok {
		t.Fatal("expected chat.ask in registry")
	}
	if desc.ModeFamily != "chat" {
		t.Fatalf("expected ModeFamily=chat, got %q", desc.ModeFamily)
	}
}

func TestDefaultRegistry_ImplementHasSupportingCapabilities(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	supporting := r.SupportingForPrimary(relurpicabilities.CapabilityChatImplement)
	if len(supporting) == 0 {
		t.Fatal("expected chat.implement to have supporting capabilities")
	}
}

func TestDefaultRegistry_PrimaryCapabilitiesHavePrimaryCapableTrue(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	for _, id := range []string{
		relurpicabilities.CapabilityChatAsk,
		relurpicabilities.CapabilityDebugInvestigate,
		relurpicabilities.CapabilityArchaeologyExplore,
	} {
		desc, _ := r.Lookup(id)
		if !desc.PrimaryCapable {
			t.Errorf("expected %q to have PrimaryCapable=true", id)
		}
	}
}

func TestDefaultRegistry_SupportingCapabilitiesHaveSupportingOnlyTrue(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	for _, id := range []string{
		relurpicabilities.CapabilityChatDirectEditExecution,
		relurpicabilities.CapabilityDebugRootCause,
	} {
		desc, _ := r.Lookup(id)
		if !desc.SupportingOnly {
			t.Errorf("expected %q to have SupportingOnly=true", id)
		}
	}
}

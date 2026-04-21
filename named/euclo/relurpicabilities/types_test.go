package relurpicabilities_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
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
		ID:           "euclo:chat.ask",
		ModeFamilies: []string{"chat"},
	})
	desc, ok := r.Lookup("euclo:chat.ask")
	if !ok {
		t.Fatal("expected to find registered descriptor")
	}
	if desc.PrimaryMode() != "chat" {
		t.Fatalf("unexpected ModeFamily: %q", desc.PrimaryMode())
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
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:chat.z", ModeFamilies: []string{"chat"}})
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:chat.a", ModeFamilies: []string{"chat"}})
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:debug.x", ModeFamilies: []string{"debug"}})
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
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:chat.ask", ModeFamilies: []string{"chat"}})
	_ = r.Register(relurpicabilities.Descriptor{ID: "euclo:debug.investigate", ModeFamilies: []string{"debug"}})
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
		relurpicabilities.CapabilityDebugInvestigateRepair,
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
		relurpicabilities.CapabilityBKCCompile,
		relurpicabilities.CapabilityBKCStream,
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
	if desc.PrimaryMode() != "chat" {
		t.Fatalf("expected ModeFamily=chat, got %q", desc.PrimaryMode())
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
		relurpicabilities.CapabilityDebugInvestigateRepair,
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

func TestDefaultRegistry_DebugInvestigateRepairParadigmMix(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	desc, ok := r.Lookup(relurpicabilities.CapabilityDebugInvestigateRepair)
	if !ok {
		t.Fatal("expected debug.investigate-repair in registry")
	}
	want := []string{"blackboard", "react", "reflection"}
	if len(desc.ParadigmMix) != len(want) {
		t.Fatalf("expected ParadigmMix %v, got %v", want, desc.ParadigmMix)
	}
	for i, v := range want {
		if desc.ParadigmMix[i] != v {
			t.Fatalf("expected ParadigmMix %v, got %v", want, desc.ParadigmMix)
		}
	}
}

func TestDefaultRegistry_ChatImplementParadigmMix(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	desc, ok := r.Lookup(relurpicabilities.CapabilityChatImplement)
	if !ok {
		t.Fatal("expected chat.implement in registry")
	}
	want := []string{"react", "architect"}
	if len(desc.ParadigmMix) != len(want) {
		t.Fatalf("expected ParadigmMix %v, got %v", want, desc.ParadigmMix)
	}
	for i, v := range want {
		if desc.ParadigmMix[i] != v {
			t.Fatalf("expected ParadigmMix %v, got %v", want, desc.ParadigmMix)
		}
	}
}

func TestDefaultRegistry_DebugRepairSimplePresentAndPrimaryCapable(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	desc, ok := r.Lookup(relurpicabilities.CapabilityDebugRepairSimple)
	if !ok {
		t.Fatal("expected debug.repair.simple in registry")
	}
	if !desc.PrimaryCapable {
		t.Fatal("expected debug.repair.simple to have PrimaryCapable=true")
	}
	if desc.PrimaryMode() != "debug" {
		t.Fatalf("expected ModeFamily=debug, got %q", desc.PrimaryMode())
	}
}

func TestDefaultRegistry_DebugModeContainsRepairSimple(t *testing.T) {
	r := relurpicabilities.DefaultRegistry()
	ids := r.IDsForMode("debug")
	found := false
	for _, id := range ids {
		if id == relurpicabilities.CapabilityDebugRepairSimple {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected IDsForMode(debug) to contain %q, got %v", relurpicabilities.CapabilityDebugRepairSimple, ids)
	}
}

// ---------------------------------------------------------------------------
// Phase 5 — Descriptor Extensions and Registry Method Updates
// ---------------------------------------------------------------------------

func TestDescriptorModeFamiliesMultiMode(t *testing.T) {
	r := relurpicabilities.NewRegistry()

	// Register a capability that belongs to both debug and chat modes
	_ = r.Register(relurpicabilities.Descriptor{
		ID:             "euclo:multi.mode",
		ModeFamilies:   []string{"debug", "chat"},
		PrimaryCapable: true,
	})

	// Should appear in IDsForMode("debug")
	debugIDs := r.IDsForMode("debug")
	foundDebug := false
	for _, id := range debugIDs {
		if id == "euclo:multi.mode" {
			foundDebug = true
			break
		}
	}
	if !foundDebug {
		t.Errorf("expected capability in IDsForMode(debug), got %v", debugIDs)
	}

	// Should appear in IDsForMode("chat")
	chatIDs := r.IDsForMode("chat")
	foundChat := false
	for _, id := range chatIDs {
		if id == "euclo:multi.mode" {
			foundChat = true
			break
		}
	}
	if !foundChat {
		t.Errorf("expected capability in IDsForMode(chat), got %v", chatIDs)
	}
}

func TestMatchByKeywordsPriorityTieBreak(t *testing.T) {
	r := relurpicabilities.NewRegistry()

	// Register two capabilities with same keywords but different priorities
	_ = r.Register(relurpicabilities.Descriptor{
		ID:              "euclo:chat.low",
		ModeFamilies:    []string{"chat"},
		PrimaryCapable:  true,
		Keywords:        []string{"test", "keyword"},
		TriggerPriority: 10,
	})
	_ = r.Register(relurpicabilities.Descriptor{
		ID:              "euclo:chat.high",
		ModeFamilies:    []string{"chat"},
		PrimaryCapable:  true,
		Keywords:        []string{"test", "keyword"},
		TriggerPriority: 50,
	})

	// Both match the same instruction with same count
	matches := r.MatchByKeywords("test keyword", "chat", nil)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Higher priority should come first
	if matches[0].ID != "euclo:chat.high" {
		t.Errorf("expected high priority first, got %s (priority=%d)", matches[0].ID, matches[0].TriggerPriority)
	}
	if matches[1].ID != "euclo:chat.low" {
		t.Errorf("expected low priority second, got %s (priority=%d)", matches[1].ID, matches[1].TriggerPriority)
	}
}

func TestMatchByKeywordsPriorityIDTieBreak(t *testing.T) {
	r := relurpicabilities.NewRegistry()

	// Register two capabilities with same keywords and same priority
	_ = r.Register(relurpicabilities.Descriptor{
		ID:              "euclo:chat.b",
		ModeFamilies:    []string{"chat"},
		PrimaryCapable:  true,
		Keywords:        []string{"test"},
		TriggerPriority: 10,
	})
	_ = r.Register(relurpicabilities.Descriptor{
		ID:              "euclo:chat.a",
		ModeFamilies:    []string{"chat"},
		PrimaryCapable:  true,
		Keywords:        []string{"test"},
		TriggerPriority: 10,
	})

	matches := r.MatchByKeywords("test", "chat", nil)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Same priority, so ID sort (ascending) applies
	if matches[0].ID != "euclo:chat.a" {
		t.Errorf("expected 'euclo:chat.a' first (ID tie-break), got %s", matches[0].ID)
	}
	if matches[1].ID != "euclo:chat.b" {
		t.Errorf("expected 'euclo:chat.b' second (ID tie-break), got %s", matches[1].ID)
	}
}

func TestDescriptorTriggerPriorityDefault(t *testing.T) {
	r := relurpicabilities.NewRegistry()

	// Register capability with zero-value TriggerPriority (default)
	_ = r.Register(relurpicabilities.Descriptor{
		ID:              "euclo:chat.default",
		ModeFamilies:    []string{"chat"},
		PrimaryCapable:  true,
		Keywords:        []string{"test"},
		TriggerPriority: 0, // zero = default
	})

	// Should not cause panic during sort
	matches := r.MatchByKeywords("test", "chat", nil)
	if len(matches) != 1 {
		t.Errorf("expected 1 match, got %d", len(matches))
	}

	// Verify the match
	if matches[0].TriggerPriority != 0 {
		t.Errorf("expected priority 0, got %d", matches[0].TriggerPriority)
	}
}

func TestDescriptorThoughtRecipeFields(t *testing.T) {
	desc := relurpicabilities.Descriptor{
		ID:                     "euclo:thought.my-recipe",
		ModeFamilies:           []string{"chat"},
		PrimaryCapable:         true,
		AllowDynamicResolution: true,
		IsUserDefined:          true,
		RecipePath:             "/config/recipes/my-recipe.yaml",
	}

	if !desc.AllowDynamicResolution {
		t.Error("expected AllowDynamicResolution to be true")
	}
	if !desc.IsUserDefined {
		t.Error("expected IsUserDefined to be true")
	}
	if desc.RecipePath != "/config/recipes/my-recipe.yaml" {
		t.Errorf("unexpected RecipePath: %q", desc.RecipePath)
	}
}

package services

import (
	"sort"
	"testing"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	euclocapabilities "codeburg.org/lexbit/relurpify/named/euclo/capabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
)

// TestEucloCapabilitiesMatchBlueprints ensures the hardcoded eucloCapabilities
// slice stays in sync with the blueprint table in relurpicabilities. A mismatch
// means RegisterAll will error at runtime with "unknown relurpic capability
// declaration(s)" — this test surfaces that at compile-and-test time.
func TestEucloCapabilitiesMatchBlueprints(t *testing.T) {
	blueprintIDs := relurpicabilities.AllCapabilityIDs()
	sort.Strings(blueprintIDs)

	declared := make([]string, len(eucloCapabilities))
	copy(declared, eucloCapabilities)
	sort.Strings(declared)

	if len(declared) != len(blueprintIDs) {
		t.Fatalf("eucloCapabilities has %d entries, blueprint table has %d; lists must match exactly",
			len(declared), len(blueprintIDs))
	}

	bpSet := make(map[string]struct{}, len(blueprintIDs))
	for _, id := range blueprintIDs {
		bpSet[id] = struct{}{}
	}
	for _, id := range declared {
		if _, ok := bpSet[id]; !ok {
			t.Errorf("eucloCapabilities contains %q which has no blueprint entry", id)
		}
	}

	declSet := make(map[string]struct{}, len(declared))
	for _, id := range declared {
		declSet[id] = struct{}{}
	}
	for _, id := range blueprintIDs {
		if _, ok := declSet[id]; !ok {
			t.Errorf("blueprint %q is missing from eucloCapabilities", id)
		}
	}
}

// TestFamilyCapabilitiesExistInBlueprint guards against advertising capability
// families that reference capabilities with no handler. Every capability ID and
// fallback referenced by the builtin families must exist in the canonical
// blueprint table.
func TestFamilyCapabilitiesExistInBlueprint(t *testing.T) {
	blueprints := make(map[string]struct{})
	for _, id := range relurpicabilities.AllCapabilityIDs() {
		blueprints[id] = struct{}{}
	}

	for _, family := range euclocapabilities.GetBuiltinFamilies() {
		if fallback := family.FallbackCapability; fallback != "" {
			if _, ok := blueprints[fallback]; !ok {
				t.Errorf("family %s fallback %q has no blueprint entry", family.ID, fallback)
			}
		}
		for _, id := range family.CapabilityIDs {
			if _, ok := blueprints[id]; !ok {
				t.Errorf("family %s references capability %q with no blueprint entry", family.ID, id)
			}
		}
	}
}

// TestEucloCapabilityIDsReturnsCopy proves the accessor hands out a defensive
// copy: a caller mutating the slice must not corrupt the package vocabulary.
func TestEucloCapabilityIDsReturnsCopy(t *testing.T) {
	ids := EucloCapabilityIDs()
	if len(ids) == 0 {
		t.Fatal("expected capability IDs")
	}
	ids[0] = "mutated"
	if EucloCapabilityIDs()[0] == "mutated" {
		t.Fatal("EucloCapabilityIDs must return a defensive copy")
	}
}

// TestCapabilityLookupAdapter covers both adapter branches: a nil registry maps
// to a nil lookup, and the adapter reports unknown IDs as not found.
func TestCapabilityLookupAdapter(t *testing.T) {
	if got := CapabilityLookup(nil); got != nil {
		t.Fatalf("CapabilityLookup(nil) = %v, want nil", got)
	}
	lookup := CapabilityLookup(registry.NewRegistry())
	if lookup == nil {
		t.Fatal("expected non-nil lookup for a registry")
	}
	if _, ok := lookup.Select("euclo:cap.does_not_exist"); ok {
		t.Fatal("unknown capability must not be found")
	}
	adapter := capabilityLookupAdapter{reg: nil}
	if _, ok := adapter.Select("anything"); ok {
		t.Fatal("nil-inner adapter must report not found")
	}
}

// TestWithCapabilityDepsRegistration exercises the deps option through the
// default registrar path.
func TestWithCapabilityDepsRegistration(t *testing.T) {
	reg := NewRegistration(WithCapabilityDeps(CapabilityDeps{Workspace: t.TempDir()}))
	if err := reg.RegisterCapabilities(registry.NewRegistry()); err != nil {
		t.Fatalf("RegisterCapabilities with deps: %v", err)
	}
}

// TestDefaultPromptRegistrarNilRegistry covers the nil-registry no-op branch.
func TestDefaultPromptRegistrarNilRegistry(t *testing.T) {
	var reg defaultPromptRegistrar
	if err := reg.RegisterAll(nil); err != nil {
		t.Fatalf("nil prompt registry must be a no-op: %v", err)
	}
}

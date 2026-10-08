package services

import (
	"sort"
	"testing"

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

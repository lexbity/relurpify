package services

import (
	"sort"
	"testing"

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
